package rbac

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/clients/kessel_client"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog"
)

// KesselClientWrapper is a Kessel implementation of the ClientWrapper interface
type KesselClientWrapper struct {
	kesselClient kessel_client.KesselClient
}

// NewKesselClientWrapper creates a new Kessel RBAC client
func NewKesselClientWrapper() (ClientWrapper, error) {
	kesselClient, err := kessel_client.NewKesselClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create kessel client: %w", err)
	}

	return &KesselClientWrapper{kesselClient: kesselClient}, nil
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

	workspaceID, _, err := k.kesselClient.GetRootWorkspaceID(ctx, id.Identity.OrgID)
	if err != nil {
		return false, fmt.Errorf("failed to get root workspace ID: %w", err)
	}

	kesselPermission := mapToKesselPermission(resource, verb)

	if verb == RbacVerbRead {
		return k.kesselClient.CheckRead(ctx, workspaceID, kesselPermission)
	} else {
		return k.kesselClient.CheckWrite(ctx, workspaceID, kesselPermission)
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
