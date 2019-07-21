package rhsso

import (
	"context"
	"errors"
	"fmt"
	aerogearv1 "github.com/integr8ly/integreatly-operator/pkg/apis/aerogear/v1alpha1"
	"github.com/integr8ly/integreatly-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/integr8ly/integreatly-operator/pkg/controller/installation/marketplace"
	"github.com/integr8ly/integreatly-operator/pkg/controller/installation/products/config"
	"github.com/integr8ly/integreatly-operator/pkg/resources"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/ownerutil"
	pkgerr "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	pkgclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	defaultInstallationNamespace = "rhsso"
	keycloakName                 = "rhsso"
	keycloakRealmName            = "openshift"
	defaultSubscriptionName      = "rhsso"
)

func NewReconciler(coreClient *kubernetes.Clientset, configManager config.ConfigReadWriter, instance *v1alpha1.Installation, mpm marketplace.MarketplaceInterface) (*Reconciler, error) {
	config, err := configManager.ReadRHSSO()
	if err != nil {
		return nil, err
	}
	if config.GetNamespace() == "" {
		config.SetNamespace(instance.Spec.NamespacePrefix + defaultInstallationNamespace)
	}
	return &Reconciler{
		coreClient:    coreClient,
		ConfigManager: configManager,
		Config:        config,
		mpm:           mpm,
		Reconciler:    resources.NewReconciler(mpm),
	}, nil
}

type Reconciler struct {
	coreClient    *kubernetes.Clientset
	Config        *config.RHSSO
	ConfigManager config.ConfigReadWriter
	mpm           marketplace.MarketplaceInterface
	*resources.Reconciler
}

func (r *Reconciler) Reconcile(ctx context.Context, inst *v1alpha1.Installation, serverClient pkgclient.Client) (v1alpha1.StatusPhase, error) {
	phase := inst.Status.ProductStatus[r.Config.GetProductName()]
	switch v1alpha1.StatusPhase(phase) {
	case v1alpha1.PhaseNone:
		return r.reconcileNamespace(ctx, r.Config.GetNamespace(), inst, serverClient)
	case v1alpha1.PhaseCreatingSubscription, v1alpha1.PhaseAwaitingOperator:
		return r.handleCreatingSubscription(ctx, r.Config.GetNamespace(), serverClient)
	case v1alpha1.PhaseCreatingComponents:
		return r.handleCreatingComponents(ctx, serverClient, inst)
	case v1alpha1.PhaseInProgress:
		return r.handleProgressPhase(ctx, serverClient)
	case v1alpha1.PhaseCompleted:
		return v1alpha1.PhaseCompleted, nil
	case v1alpha1.PhaseFailed:
		//potentially do a dump and recover in the future
		return v1alpha1.PhaseFailed, errors.New("installation of RHSSO failed")
	default:
		return v1alpha1.PhaseFailed, errors.New("Unknown phase for RHSSO: " + string(phase))
	}
}

func (r *Reconciler) reconcileNamespace(ctx context.Context, ns string, inst *v1alpha1.Installation, client pkgclient.Client) (v1alpha1.StatusPhase, error) {
	phase, err := r.ReconcileNamespace(ctx, ns, inst, client)
	if err != nil {
		return v1alpha1.PhaseFailed, pkgerr.Wrap(err, "failed to reconcile namespace for amq streams")
	}
	if phase == v1alpha1.PhaseNone {
		return v1alpha1.PhaseCreatingSubscription, nil
	}
	return phase, err
}

func (r *Reconciler) handleCreatingSubscription(ctx context.Context, ns string, client pkgclient.Client) (v1alpha1.StatusPhase, error) {
	phase, err := r.ReconcileSubscription(ctx, defaultSubscriptionName, ns, client)
	if err != nil {
		return v1alpha1.PhaseFailed, pkgerr.Wrap(err, "failed to reconcile subscription for amq streams")
	}
	if phase == v1alpha1.PhaseNone {
		return v1alpha1.PhaseCreatingComponents, nil
	}
	return phase, nil
}

func (r *Reconciler) handleCreatingComponents(ctx context.Context, serverClient pkgclient.Client, inst *v1alpha1.Installation) (v1alpha1.StatusPhase, error) {
	kc := &aerogearv1.Keycloak{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fmt.Sprintf(
				"%s/%s",
				aerogearv1.SchemeGroupVersion.Group,
				aerogearv1.SchemeGroupVersion.Version),
			Kind: aerogearv1.KeycloakKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      keycloakName,
			Namespace: r.Config.GetNamespace(),
		},
		Spec: aerogearv1.KeycloakSpec{
			AdminCredentials: "",
			Plugins: []string{
				"keycloak-metrics-spi",
			},
			Backups:   []aerogearv1.KeycloakBackup{},
			Provision: true,
		},
	}
	ownerutil.EnsureOwner(kc, inst)
	err := serverClient.Create(ctx, kc)
	if err != nil {
		return v1alpha1.PhaseFailed, err
	}

	logrus.Infof("Creating Keycloakrealm")
	kcr := &aerogearv1.KeycloakRealm{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fmt.Sprintf(
				"%s/%s",
				aerogearv1.SchemeGroupVersion.Group,
				aerogearv1.SchemeGroupVersion.Version),
			Kind: aerogearv1.KeycloakRealmKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      keycloakRealmName,
			Namespace: r.Config.GetNamespace(),
		},
		Spec: aerogearv1.KeycloakRealmSpec{
			CreateOnly: true,
			KeycloakApiRealm: &aerogearv1.KeycloakApiRealm{
				ID:          keycloakRealmName,
				Realm:       keycloakRealmName,
				DisplayName: keycloakRealmName,
				Enabled:     true,
				EventsListeners: []string{
					"metrics-listener",
				},
				Users: []*aerogearv1.KeycloakUser{
					{
						KeycloakApiUser: &aerogearv1.KeycloakApiUser{
							Enabled:       true,
							Attributes:    aerogearv1.KeycloakAttributes{},
							UserName:      "customer-admin",
							EmailVerified: true,
							Email:         "customer-admin@example.com",
							Password:      "Password1",
							RealmRoles: []string{
								"offline_access",
								"uma_authorization",
							},
							ClientRoles: map[string][]string{
								"account": {
									"manage-account",
									"view-profile",
								},
								"realm-management": {
									"manage-users",
									"manage-identity-providers",
									"view-realm",
								},
							},
						},
						OutputSecret: "customer-admin-user-credentials",
					},
				},
			},
		},
	}
	ownerutil.EnsureOwner(kcr, inst)
	err = serverClient.Create(ctx, kcr)
	if err != nil {
		return v1alpha1.PhaseFailed, err
	}

	return v1alpha1.PhaseInProgress, nil
}

func (r *Reconciler) handleProgressPhase(ctx context.Context, serverClient pkgclient.Client) (v1alpha1.StatusPhase, error) {
	logrus.Infof("checking ready status for rhsso")
	kcr := &aerogearv1.KeycloakRealm{}

	err := serverClient.Get(ctx, pkgclient.ObjectKey{Name: keycloakRealmName, Namespace: r.Config.GetNamespace()}, kcr)
	if err != nil {
		return v1alpha1.PhaseFailed, err
	}

	if kcr.Status.Phase == aerogearv1.PhaseReconcile {
		err = r.exportConfig(ctx, serverClient)
		if err != nil {
			logrus.Errorf("Failed to write RH-SSO config %v", err)
			return v1alpha1.PhaseFailed, err
		}

		logrus.Infof("Keycloak has successfully processed the keycloakRealm")
		return v1alpha1.PhaseCompleted, nil
	}

	logrus.Infof("KeycloakRealm status phase is: %s", kcr.Status.Phase)
	return v1alpha1.PhaseInProgress, nil
}

func (r *Reconciler) exportConfig(ctx context.Context, serverClient pkgclient.Client) error {
	kc := &aerogearv1.Keycloak{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keycloakName,
			Namespace: r.Config.GetNamespace(),
		},
	}
	err := serverClient.Get(ctx, pkgclient.ObjectKey{Name: keycloakName, Namespace: r.Config.GetNamespace()}, kc)
	if err != nil {
		return pkgerr.Wrap(err, "could not retrieve keycloak custom resource for keycloak config")
	}
	kcAdminCredSecretName := kc.Spec.AdminCredentials

	kcAdminCredSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kcAdminCredSecretName,
			Namespace: r.Config.GetNamespace(),
		},
	}
	err = serverClient.Get(ctx, pkgclient.ObjectKey{Name: kcAdminCredSecretName, Namespace: r.Config.GetNamespace()}, kcAdminCredSecret)
	if err != nil {
		return pkgerr.Wrap(err, "could not retrieve keycloak admin credential secret for keycloak config")
	}
	kcURLBytes := kcAdminCredSecret.Data["SSO_ADMIN_URL"]
	r.Config.SetRealm(keycloakRealmName)
	r.Config.SetURL(string(kcURLBytes))
	err = r.ConfigManager.WriteConfig(r.Config)
	if err != nil {
		return pkgerr.Wrap(err, "could not update keycloak config")
	}
	return nil
}
