package user

import (
	"context"
	"fmt"
	v1 "github.com/openshift/api/config/v1"
	"github.com/pkg/errors"
	"regexp"
	"strings"

	keycloak "github.com/keycloak/keycloak-operator/pkg/apis/keycloak/v1alpha1"
	usersv1 "github.com/openshift/api/user/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Helper for User associated functions

const (
	updateProfileAction         = "UPDATE_PROFILE"
	invalidCharacterReplacement = "-"
	GeneratedNamePrefix         = "generated-"
)

var (
	exclusionGroups = []string{
		"layered-cs-sre-admins",
		"osd-sre-admins",
	}
)

func GetUserEmailFromIdentity(ctx context.Context, serverClient k8sclient.Client, user usersv1.User) (string, error) {
	email := ""

	// User can have multiple identities
	for _, identityName := range user.Identities {
		identity := &usersv1.Identity{}
		err := serverClient.Get(ctx, k8sclient.ObjectKey{Name: identityName}, identity)

		if err != nil {
			return "", fmt.Errorf("failed to get identity provider: %w", err)
		}

		// Get first identity with email and break loop
		if identity.Extra["email"] != "" {
			email = identity.Extra["email"]
			break
		}
	}

	return email, nil
}

func AppendUpdateProfileActionForUserWithoutEmail(keycloakUser *keycloak.KeycloakAPIUser) {
	if keycloakUser.Email == "" {
		keycloakUser.RequiredActions = []string{updateProfileAction}
	}
}

func GetValidGeneratedUserName(keycloakUser keycloak.KeycloakAPIUser) string {
	// Regex for only alphanumeric values
	reg, _ := regexp.Compile("[^a-zA-Z0-9]+")

	// Replace all non-alphanumeric values with the replacement character
	processedString := reg.ReplaceAllString(strings.ToLower(keycloakUser.UserName), invalidCharacterReplacement)

	// Remove occurrence of replacement character at end of string
	processedString = strings.TrimSuffix(processedString, invalidCharacterReplacement)

	for _, federatedIdentity := range keycloakUser.FederatedIdentities {
		userId := federatedIdentity.UserID
		// Append user id to name to ensure uniqueness
		if userId != "" {
			processedString = fmt.Sprintf("%s-%s", processedString, federatedIdentity.UserID)
			break
		}
	}

	return fmt.Sprintf("%v%v", GeneratedNamePrefix, processedString)
}

func UserInExclusionGroup(user usersv1.User, groups *usersv1.GroupList) bool {

	// Below is a slightly complex way to determine if the user exists in an exlcusion group
	// Ideally we would use the user.Groups field but this does not seem to get populated.
	for _, group := range groups.Items {
		for _, xGroup := range exclusionGroups {
			if group.Name == xGroup {
				for _, groupUser := range group.Users {
					if groupUser == user.Name {
						return true
					}
				}
			}
		}
	}
	return false
}

func GetUsersInActiveIDPs(ctx context.Context, serverClient k8sclient.Client) (*usersv1.UserList, error) {
	openshiftUsers := &usersv1.UserList{}
	err := serverClient.List(ctx, openshiftUsers)
	if err != nil {
		return nil, errors.Wrap(err, "could not list users")
	}
	idpNames := map[string]bool{}

	oAuths := &v1.OAuthList{}
	err = serverClient.List(ctx, oAuths)
	if err != nil {
		return nil, errors.Wrap(err, "could not list oAuths")
	}

	// get active idp names
	for _, oauth := range oAuths.Items {
		for _, idp := range oauth.Spec.IdentityProviders {
			idpNames[idp.Name] = true
		}
	}

	activeUsers := &usersv1.UserList{}

	//go over each user
	for _, user := range openshiftUsers.Items {
		// get  their identities - can be multiple?
		identities, err := GetIdentities(ctx, serverClient, user)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get identities for user %v", user.Name)
		}

		for _, identity := range identities.Items {
			//if any identity is provided by an active idp
			if _, ok := idpNames[identity.ProviderName]; ok {
				//add user to return set
				activeUsers.Items = append(activeUsers.Items, user)
				//move to next user - so we don't add a user twice
				break
			}
		}
	}
	return activeUsers, nil
}

func GetIdentities(ctx context.Context, serverClient k8sclient.Client, user usersv1.User) (*usersv1.IdentityList, error) {
	identities := &usersv1.IdentityList{}

	for _, identityName := range user.Identities {
		identity := &usersv1.Identity{}
		err := serverClient.Get(ctx, k8sclient.ObjectKey{Name: identityName}, identity)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get identity %v for user %v", identityName, user.Name)
		}
		identities.Items = append(identities.Items, *identity)
	}
	return identities, nil
}
