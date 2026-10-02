package lightwell

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

// These imports are used by swag for OpenAPI generation
var _ api.TermsRequiredResponse
var _ api.TermsDetailsResponse
var _ api.TermsAcceptRequest
var _ api.TermsAcceptResponse
var _ ce.ErrorResponse

type LightwellTermsHandler struct {
	handler.LightwellTermsHandler
}

func RegisterLightwellTermsRoutes(engine *echo.Group, tsClient *terms_service_client.TermsServiceClient) {
	if tsClient == nil {
		panic("tsClient is nil")
	}
	h := LightwellTermsHandler{
		LightwellTermsHandler: handler.LightwellTermsHandler{
			TermsServiceClient: *tsClient,
		},
	}
	addLightwellRoute(engine, http.MethodGet, "/terms/required", h.getTermsRequired, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodGet, "/terms/details", h.getTermsDetails, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodPut, "/terms/accept", h.acceptTerms, rbac.RbacVerbWrite)
}

// getTermsRequired godoc
// @Summary      Check if Lightwell terms acceptance is required
// @ID           getLightwellNetworkTermsRequired
// @Description  Check whether the current user must accept Lightwell terms.
// @Tags         lightwell
// @Produce      json
// @Success      200 {object} api.TermsRequiredResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /terms/required [get]
func (h *LightwellTermsHandler) getTermsRequired(c echo.Context) error {
	return h.GetTermsRequired(c)
}

// getTermsDetails godoc
// @Summary      Get required Lightwell terms details
// @ID           getLightwellNetworkTermsDetails
// @Description  Retrieve term names, PDF links, and IDs for terms the user must accept.
// @Tags         lightwell
// @Produce      json
// @Success      200 {object} api.TermsDetailsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /terms/details [get]
func (h *LightwellTermsHandler) getTermsDetails(c echo.Context) error {
	return h.GetTermsDetails(c)
}

// acceptTerms godoc
// @Summary      Accept Lightwell terms
// @ID           acceptLightwellNetworkTerms
// @Description  Record the user's acceptance of a specific Lightwell term.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        body  body  api.TermsAcceptRequest  true  "Term acceptance request"
// @Success      200 {object} api.TermsAcceptResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /terms/accept [put]
func (h *LightwellTermsHandler) acceptTerms(c echo.Context) error {
	return h.AcceptTerms(c)
}
