package handler

import (
	"fmt"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
)

type LightwellTermsHandler struct {
	TermsServiceClient   terms_service_client.TermsServiceClient
	FeatureServiceClient feature_service_client.FeatureServiceClient
}

func RegisterLightwellTermsRoutes(engine *echo.Group, tsClient *terms_service_client.TermsServiceClient, fsClient *feature_service_client.FeatureServiceClient) {
	if tsClient == nil || fsClient == nil {
		log.Error().Msg("terms routes not registered: tsClient or fsClient is nil")
		return
	}
	h := LightwellTermsHandler{
		TermsServiceClient:   *tsClient,
		FeatureServiceClient: *fsClient,
	}
	addRepoRoute(engine, http.MethodGet, "/lightwell/terms/required", h.GetTermsRequired, rbac.RbacVerbRead)
}

// getLogin extracts the username from the request identity, guarding against
// nil User (e.g. service accounts). Returns empty string if unavailable.
func getLogin(c echo.Context) string {
	id := identity.GetIdentity(c.Request().Context())
	if id.Identity.User != nil && id.Identity.User.Username != "" {
		return id.Identity.User.Username
	}
	// TODO: Confirm Identity.User.Username reliability for service accounts
	if id.Identity.ServiceAccount != nil {
		return id.Identity.ServiceAccount.Username
	}
	return ""
}

// GetTermsRequired godoc
// @Summary      Check if Lightwell terms acceptance is required
// @ID           getLightwellTermsRequired
// @Description  Check whether the current user must accept Lightwell terms before access.
// @Tags         lightwell
// @Produce      json
// @Success      200 {object} api.TermsRequiredResponse
// @Router       /lightwell/terms/required [get]
func (h *LightwellTermsHandler) GetTermsRequired(c echo.Context) error {
	ctx := c.Request().Context()

	if err := CheckLightwellTermsEnabled(ctx); err != nil {
		log.Ctx(ctx).Info().Err(err).Msg("lightwell terms feature not enabled, returning required=false")
		return c.JSON(http.StatusOK, api.TermsRequiredResponse{Required: false})
	}

	id := identity.GetIdentity(c.Request().Context())
	if id.Identity.User != nil && id.Identity.User.Internal {
		log.Ctx(ctx).Debug().Msg("skipping terms check for internal Red Hat user")
		return c.JSON(http.StatusOK, api.TermsRequiredResponse{Required: false})
	}

	login := getLogin(c)
	if login == "" {
		log.Ctx(ctx).Warn().Msg("no login found in request identity, returning required=false")
		return c.JSON(http.StatusOK, api.TermsRequiredResponse{Required: false})
	}

	events, err := h.TermsServiceClient.GetRequiredEvents(ctx, login)
	if err != nil {
		// if the terms service is unreachable, block access
		log.Ctx(ctx).Error().Err(err).Msg("terms service unavailable")
		return ce.NewErrorResponse(http.StatusServiceUnavailable,
			"Terms service unavailable",
			"Unable to verify terms acceptance status. Please try again later.")
	}

	events, err = h.filterEventsByEntitlement(c, events)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("feature service unavailable during terms filtering, failing closed")
		return ce.NewErrorResponse(http.StatusServiceUnavailable,
			"Terms service unavailable",
			"Unable to verify feature entitlements for terms check. Please try again later.")
	}

	resp := api.TermsRequiredResponse{Required: len(events) > 0}
	if resp.Required {
		ts := config.Get().Clients.TermsService
		resp.Site = ts.Site
		resp.Events = events
	}

	return c.JSON(http.StatusOK, resp)
}

// filterEventsByEntitlement narrows the required events to only those the
// user's org is entitled to, based on the event_feature_map config. If no
// mapping is configured, all events pass through unchanged.
func (h *LightwellTermsHandler) filterEventsByEntitlement(c echo.Context, events []string) ([]string, error) {
	eventFeatureMap := config.Get().Clients.TermsService.EventFeatures()
	if len(eventFeatureMap) == 0 || len(events) == 0 {
		log.Ctx(c.Request().Context()).Debug().Msg("event feature map or events is empty")
		return events, nil
	}

	_, orgID := GetAccountIdOrgId(c)
	if orgID == "" {
		return events, nil
	}

	entitledFeatures, err := h.FeatureServiceClient.GetEntitledFeatures(c.Request().Context(), orgID)
	if err != nil {
		// if the feature service is unreachable, we cannot
		// determine which terms apply, so block access.
		return nil, fmt.Errorf("could not check entitled features for terms filtering: %w", err)
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
	return filtered, nil
}
