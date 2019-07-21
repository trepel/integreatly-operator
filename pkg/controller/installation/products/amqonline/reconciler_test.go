package amqonline

import (
	"context"
	chev1 "github.com/eclipse/che-operator/pkg/apis/org/v1"
	enmassev1 "github.com/enmasseproject/enmasse/pkg/apis/admin/v1beta1"
	aerogearv1 "github.com/integr8ly/integreatly-operator/pkg/apis/aerogear/v1alpha1"
	"github.com/integr8ly/integreatly-operator/pkg/apis/enmasse/v1beta1"
	enmassev1beta1 "github.com/integr8ly/integreatly-operator/pkg/apis/enmasse/v1beta1"
	"github.com/integr8ly/integreatly-operator/pkg/apis/enmasse/v1beta2"
	enmassev1beta2 "github.com/integr8ly/integreatly-operator/pkg/apis/enmasse/v1beta2"
	"github.com/integr8ly/integreatly-operator/pkg/apis/integreatly/v1alpha1"
	kafkav1 "github.com/integr8ly/integreatly-operator/pkg/apis/kafka.strimzi.io/v1alpha1"
	"github.com/integr8ly/integreatly-operator/pkg/controller/installation/marketplace"
	"github.com/integr8ly/integreatly-operator/pkg/controller/installation/products/config"
	operatorsv1alpha1 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	marketplacev1 "github.com/operator-framework/operator-marketplace/pkg/apis/operators/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func buildScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	chev1.SchemeBuilder.AddToScheme(scheme)
	aerogearv1.SchemeBuilder.AddToScheme(scheme)
	v1alpha1.SchemeBuilder.AddToScheme(scheme)
	operatorsv1alpha1.AddToScheme(scheme)
	marketplacev1.SchemeBuilder.AddToScheme(scheme)
	kafkav1.SchemeBuilder.AddToScheme(scheme)
	corev1.SchemeBuilder.AddToScheme(scheme)
	enmassev1.SchemeBuilder.AddToScheme(scheme)
	enmassev1beta1.SchemeBuilder.AddToScheme(scheme)
	enmassev1beta2.SchemeBuilder.AddToScheme(scheme)
	return scheme
}

const (
	defaultNamespace = "amq-online"
)

func basicConfigMock() *config.ConfigReadWriterMock {
	return &config.ConfigReadWriterMock{
		ReadAMQOnlineFunc: func() (ready *config.AMQOnline, e error) {
			return config.NewAMQOnline(config.ProductConfig{
				"NAMESPACE": defaultNamespace,
			}), nil
		},
		ReadRHSSOFunc: func() (*config.RHSSO, error) {
			return config.NewRHSSO(config.ProductConfig{
				"NAMESPACE": defaultNamespace,
				"URL":       "sso.openshift-cluster.com",
			}), nil
		},
	}
}

func TestReconcile_reconcileAuthServices(t *testing.T) {
	scenarios := []struct {
		Name           string
		Client         client.Client
		FakeConfig     *config.ConfigReadWriterMock
		Installation   *v1alpha1.Installation
		ExpectedStatus v1alpha1.StatusPhase
		AuthServices   []*enmassev1.AuthenticationService
		FakeMPM        *marketplace.MarketplaceInterfaceMock
	}{
		{
			Name:           "Test returns none phase if successfully creating new auth services",
			Client:         fake.NewFakeClientWithScheme(buildScheme()),
			FakeConfig:     basicConfigMock(),
			AuthServices:   GetDefaultAuthServices(defaultNamespace),
			ExpectedStatus: v1alpha1.PhaseNone,
		},
		{
			Name:           "Test returns none phase if trying to create existing auth services",
			Client:         fake.NewFakeClientWithScheme(buildScheme(), GetDefaultAuthServices(defaultSubscriptionName)[0]),
			AuthServices:   GetDefaultAuthServices(defaultNamespace),
			FakeConfig:     basicConfigMock(),
			ExpectedStatus: v1alpha1.PhaseNone,
		},
	}

	for _, s := range scenarios {
		t.Run(s.Name, func(t *testing.T) {
			r, err := NewReconciler(s.FakeConfig, s.Installation, s.FakeMPM)
			if err != nil {
				t.Fatalf("could not create reconciler %v", err)
			}
			phase, err := r.reconcileAuthServices(context.TODO(), s.Client, s.AuthServices)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if phase != s.ExpectedStatus {
				t.Fatalf("expected status %s but got %s", s.ExpectedStatus, phase)
			}
		})
	}
}

func TestReconcile_reconcileBrokerConfigs(t *testing.T) {
	scenarios := []struct {
		Name                 string
		Client               client.Client
		FakeConfig           *config.ConfigReadWriterMock
		Installation         *v1alpha1.Installation
		ExpectedStatus       v1alpha1.StatusPhase
		BrokeredInfraConfigs []*v1beta1.BrokeredInfraConfig
		StandardInfraConfigs []*v1beta1.StandardInfraConfig
		FakeMPM              *marketplace.MarketplaceInterfaceMock
	}{
		{
			Name:                 "Test returns none phase if successfully creating new address space plans",
			Client:               fake.NewFakeClientWithScheme(buildScheme()),
			FakeConfig:           basicConfigMock(),
			BrokeredInfraConfigs: GetDefaultBrokeredInfraConfigs(defaultNamespace),
			StandardInfraConfigs: GetDefaultStandardInfraConfigs(defaultNamespace),
			ExpectedStatus:       v1alpha1.PhaseNone,
		},
		{
			Name:                 "Test returns none phase if trying to create existing address space plans",
			Client:               fake.NewFakeClientWithScheme(buildScheme(), GetDefaultAuthServices(defaultSubscriptionName)[0]),
			BrokeredInfraConfigs: GetDefaultBrokeredInfraConfigs(defaultNamespace),
			StandardInfraConfigs: GetDefaultStandardInfraConfigs(defaultNamespace),
			FakeConfig:           basicConfigMock(),
			ExpectedStatus:       v1alpha1.PhaseNone,
		},
	}

	for _, s := range scenarios {
		t.Run(s.Name, func(t *testing.T) {
			r, err := NewReconciler(s.FakeConfig, s.Installation, s.FakeMPM)
			if err != nil {
				t.Fatalf("could not create reconciler %v", err)
			}
			phase, err := r.reconcileBrokerConfigs(context.TODO(), s.Client, s.BrokeredInfraConfigs, s.StandardInfraConfigs)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if phase != s.ExpectedStatus {
				t.Fatalf("expected status %s but got %s", s.ExpectedStatus, phase)
			}
		})
	}
}

func TestReconcile_reconcileAddressPlans(t *testing.T) {
	scenarios := []struct {
		Name           string
		Client         client.Client
		FakeConfig     *config.ConfigReadWriterMock
		Installation   *v1alpha1.Installation
		ExpectedStatus v1alpha1.StatusPhase
		AddressPlans   []*v1beta2.AddressPlan
		FakeMPM        *marketplace.MarketplaceInterfaceMock
	}{
		{
			Name:           "Test returns none phase if successfully creating new address space plans",
			Client:         fake.NewFakeClientWithScheme(buildScheme()),
			FakeConfig:     basicConfigMock(),
			AddressPlans:   GetDefaultAddressPlans(defaultNamespace),
			ExpectedStatus: v1alpha1.PhaseNone,
		},
		{
			Name:           "Test returns none phase if trying to create existing address space plans",
			Client:         fake.NewFakeClientWithScheme(buildScheme(), GetDefaultAuthServices(defaultSubscriptionName)[0]),
			AddressPlans:   GetDefaultAddressPlans(defaultNamespace),
			FakeConfig:     basicConfigMock(),
			ExpectedStatus: v1alpha1.PhaseNone,
		},
	}

	for _, s := range scenarios {
		t.Run(s.Name, func(t *testing.T) {
			r, err := NewReconciler(s.FakeConfig, s.Installation, s.FakeMPM)
			if err != nil {
				t.Fatalf("could not create reconciler %v", err)
			}
			phase, err := r.reconcileAddressPlans(context.TODO(), s.Client, s.AddressPlans)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if phase != s.ExpectedStatus {
				t.Fatalf("expected status %s but got %s", s.ExpectedStatus, phase)
			}
		})
	}
}

func TestReconcile_reconcileAddressSpacePlans(t *testing.T) {
	scenarios := []struct {
		Name              string
		Client            client.Client
		FakeConfig        *config.ConfigReadWriterMock
		Installation      *v1alpha1.Installation
		ExpectedStatus    v1alpha1.StatusPhase
		AddressSpacePlans []*v1beta2.AddressSpacePlan
		FakeMPM           *marketplace.MarketplaceInterfaceMock
	}{
		{
			Name:              "Test returns none phase if successfully creating new address space plans",
			Client:            fake.NewFakeClientWithScheme(buildScheme()),
			FakeConfig:        basicConfigMock(),
			AddressSpacePlans: GetDefaultAddressSpacePlans(defaultNamespace),
			ExpectedStatus:    v1alpha1.PhaseNone,
		},
		{
			Name:              "Test returns none phase if trying to create existing address space plans",
			Client:            fake.NewFakeClientWithScheme(buildScheme(), GetDefaultAuthServices(defaultSubscriptionName)[0]),
			AddressSpacePlans: GetDefaultAddressSpacePlans(defaultNamespace),
			FakeConfig:        basicConfigMock(),
			ExpectedStatus:    v1alpha1.PhaseNone,
		},
	}

	for _, s := range scenarios {
		t.Run(s.Name, func(t *testing.T) {
			r, err := NewReconciler(s.FakeConfig, s.Installation, s.FakeMPM)
			if err != nil {
				t.Fatalf("could not create reconciler %v", err)
			}
			phase, err := r.reconcileAddressSpacePlans(context.TODO(), s.Client, s.AddressSpacePlans)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if phase != s.ExpectedStatus {
				t.Fatalf("expected status %s but got %s", s.ExpectedStatus, phase)
			}
		})
	}
}
