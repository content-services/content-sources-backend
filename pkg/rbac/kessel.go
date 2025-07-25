package rbac

import (
	"context"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/kessel_client"
	"github.com/project-kessel/inventory-client-go/common"
	kesselClientV2 "github.com/project-kessel/inventory-client-go/v1beta2"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog"
)

// KesselClientWrapper is a Kessel implementation of the ClientWrapper interface
type KesselClientWrapper struct {
	client      *kesselClientV2.InventoryClient
	timeout     time.Duration
	rbacBaseURL string
	tokenClient *common.TokenClient
}

// NewKesselClientWrapper creates a new Kessel RBAC client
func NewKesselClientWrapper(kesselURL string, timeout time.Duration, rbacBaseURL string, authEnabled bool, clientID, clientSecret, oidcIssuer string, insecure bool) (ClientWrapper, error) {
	if kesselURL == "" {
		return nil, fmt.Errorf("kessel URL cannot be empty")
	}
	if timeout < 0 {
		return nil, fmt.Errorf("timeout cannot be negative")
	}

	options := []func(*common.Config){
		common.WithgRPCUrl(kesselURL),
		common.WithTLSInsecure(insecure),
	}

	if authEnabled {
		options = append(options, common.WithAuthEnabled(clientID, clientSecret, oidcIssuer))
	}

	config := common.NewConfig(options...)

	client, err := kesselClientV2.New(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kessel client: %w", err)
	}

	var tokenClient *common.TokenClient
	if authEnabled {
		tokenClient = common.NewTokenClient(config)
	}

	return &KesselClientWrapper{
		client:      client,
		timeout:     timeout,
		rbacBaseURL: rbacBaseURL,
		tokenClient: tokenClient,
	}, nil
}

// Allowed checks if the user has permission to perform the given verb on the given resource
func (k *KesselClientWrapper) Allowed(ctx context.Context, resource Resource, verb Verb) (bool, error) {
	logger := zerolog.Ctx(ctx)

	// Skip RBAC check for org admins
	if skipRbacCheck(ctx) {
		return true, nil
	}

	// Get identity from context
	id := identity.GetIdentity(ctx)
	if id.Identity.User == nil && id.Identity.ServiceAccount == nil {
		logger.Error().Msg("no user or service account identity found in context")
		return false, fmt.Errorf("no user or service account identity found in context")
	}

	kesselClient, _ := kessel_client.NewKesselClient()
	workspaceID, _, err := kesselClient.GetRootWorkspaceID(ctx, id.Identity.OrgID)
	if err != nil {
		return false, fmt.Errorf("failed to get root workspace ID: %w", err)
	}

	kesselPermission := mapToKesselPermission(resource, verb)

	if verb == RbacVerbRead {
		return kesselClient.CheckRead(ctx, workspaceID, kesselPermission)
	} else {
		return kesselClient.CheckWrite(ctx, workspaceID, kesselPermission)
	}
}

// mapToKesselPermission maps rbac v1 verbs to kessel verbs
func mapToKesselPermission(resource Resource, verb Verb) string {
	// Skip invalid resources
	if resource == ResourceAny || resource == ResourceUndefined || string(resource) == "" {
		return ""
	}

	// Map verb to action suffix
	var kesselVerb string
	switch verb {
	case RbacVerbRead:
		kesselVerb = "view"
	case RbacVerbWrite:
		kesselVerb = "edit"
	case RbacVerbUpload:
		kesselVerb = "upload"
	default:
		return ""
	}

	var kesselResource string
	switch resource {
	case ResourceRepositories:
		kesselResource = "repository"
	case ResourceTemplates:
		kesselResource = "template"
	}

	return fmt.Sprintf("content_sources_%s_%s", kesselResource, kesselVerb)
}
