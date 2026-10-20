package terms

import (
	"context"
	"slices"

	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
)

// getLogin extracts the username from the request identity, guarding against
// nil User (e.g. service accounts). Returns empty string if unavailable.
func getLogin(ctx context.Context) string {
	id := identity.GetIdentity(ctx)
	if id.Identity.User != nil && id.Identity.User.Username != "" {
		return id.Identity.User.Username
	}
	// TODO: Confirm terms-service support for service-account usernames.
	if id.Identity.ServiceAccount != nil {
		return id.Identity.ServiceAccount.Username
	}
	return ""
}

// RequiredEvents returns outstanding terms for the caller’s entitled features,
// preserving the shared bypasses and fail-open policy for both APIs.
func RequiredEvents(ctx context.Context, tsClient terms_service_client.TermsServiceClient, fsClient feature_service_client.FeatureServiceClient) []string {
	if !config.FeatureAccessible(ctx, config.Get().Features.LightwellTerms) {
		log.Ctx(ctx).Info().Msg("lightwell terms feature not enabled, returning required=false")
		return nil
	}

	id := identity.GetIdentity(ctx)
	if id.Identity.User != nil && id.Identity.User.Internal {
		log.Ctx(ctx).Debug().Msg("skipping terms check for internal Red Hat user")
		return nil
	}

	login := getLogin(ctx)
	if login == "" {
		log.Ctx(ctx).Warn().Msg("no login found in request identity, returning required=false")
		return nil
	}

	orgID := id.Identity.Internal.OrgID
	entitledFeatures, err := fsClient.GetEntitledFeatures(ctx, orgID)
	if err != nil {
		// Fail open: if the feature service is unreachable, don't block
		// users with terms for features they may not have access to.
		log.Ctx(ctx).Warn().Err(err).Msg("could not check entitled features for terms check, failing open (required=false)")
		return nil
	}

	if slices.Contains(entitledFeatures, config.LightwellClearinghouseFeature) {
		log.Ctx(ctx).Debug().Msg("skipping terms check for clearinghouse-entitled org")
		return nil
	}

	events, err := tsClient.GetRequiredEvents(ctx, login)
	if err != nil {
		// Fail open: if the terms service is unreachable, allow the user
		// through rather than blocking access to content.
		log.Ctx(ctx).Warn().Err(err).Msg("terms service unavailable, failing open (required=false)")
		return nil
	}

	return filterEventsByEntitlement(ctx, events, entitledFeatures)
}

// filterEventsByEntitlement narrows the required events to only those the
// user's org is entitled to, based on the event_feature_map config. If no
// mapping is configured, all events pass through unchanged.
func filterEventsByEntitlement(ctx context.Context, events []string, entitledFeatures []string) []string {
	eventFeatureMap := config.Get().Clients.TermsService.EventFeatures()
	if len(eventFeatureMap) == 0 || len(events) == 0 {
		log.Ctx(ctx).Debug().Msg("event feature map or events is empty")
		return events
	}

	featureSet := make(map[string]bool, len(entitledFeatures))
	for _, f := range entitledFeatures {
		featureSet[f] = true
	}

	var filtered []string
	for _, event := range events {
		requiredFeature, mapped := eventFeatureMap[event]
		if !mapped {
			filtered = append(filtered, event)
			continue
		}
		if featureSet[requiredFeature] {
			filtered = append(filtered, event)
		}
	}
	return filtered
}
