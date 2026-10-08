package lightwell

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

// These imports are used by swag for OpenAPI generation
var _ api.TermsRequiredResponse

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
}

// getTermsRequired godoc
// @Summary      Check if Lightwell terms acceptance is required
// @ID           getLightwellNetworkTermsRequired
// @Description  Check whether the current user must accept Lightwell terms.
// @Tags         lightwell
// @Produce      json
// @Success      200 {object} api.TermsRequiredResponse
// @Router       /terms/required [get]
func (h *LightwellTermsHandler) getTermsRequired(c echo.Context) error {
	return h.GetTermsRequired(c)
}
