package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/content-sources-backend/pkg/terms"
	"github.com/labstack/echo/v4"
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

// GetTermsRequired godoc
// @Summary      Check if Lightwell terms acceptance is required
// @ID           getLightwellTermsRequired
// @Description  Check whether the current user must accept Lightwell terms before access.
// @Tags         lightwell
// @Produce      json
// @Success      200 {object} api.TermsRequiredResponse
// @Router       /lightwell/terms/required [get]
func (h *LightwellTermsHandler) GetTermsRequired(c echo.Context) error {
	events := terms.RequiredEvents(c.Request().Context(), h.TermsServiceClient, h.FeatureServiceClient)
	resp := api.TermsRequiredResponse{Required: len(events) > 0}
	if resp.Required {
		ts := config.Get().Clients.TermsService
		resp.Site = ts.Site
		resp.Events = events
	}

	return c.JSON(http.StatusOK, resp)
}
