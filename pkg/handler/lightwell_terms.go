package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
)

type LightwellTermsHandler struct {
	TermsServiceClient terms_service_client.TermsServiceClient
}

func RegisterLightwellTermsRoutes(engine *echo.Group, tsClient *terms_service_client.TermsServiceClient) {
	if tsClient == nil {
		panic("tsClient is nil")
	}
	h := LightwellTermsHandler{TermsServiceClient: *tsClient}
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

	login := getLogin(c)
	if login == "" {
		log.Ctx(ctx).Warn().Msg("no login found in request identity, returning required=false")
		return c.JSON(http.StatusOK, api.TermsRequiredResponse{Required: false})
	}

	events, err := h.TermsServiceClient.GetRequiredEvents(ctx, login)
	if err != nil {
		// Fail open: if the terms service is unreachable, allow the user
		// through rather than blocking access to content.
		log.Ctx(ctx).Warn().Err(err).Msg("terms service unavailable, failing open (required=false)")
		return c.JSON(http.StatusOK, api.TermsRequiredResponse{Required: false})
	}

	resp := api.TermsRequiredResponse{Required: len(events) > 0}
	if resp.Required {
		ts := config.Get().Clients.TermsService
		resp.Site = ts.Site
		resp.Events = events
	}

	return c.JSON(http.StatusOK, resp)
}
