package lightwell

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

// These imports are used by swag for OpenAPI generation
// TODO remove when these handlers are implemented here
var _ api.LightwellAdvisoryCollectionResponse
var _ ce.ErrorResponse

type LightwellAdvisoryHandler struct {
	handler.LightwellAdvisoryHandler
}

func RegisterLightwellAdvisoryRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, fsClient *feature_service_client.FeatureServiceClient) {
	h := LightwellAdvisoryHandler{
		LightwellAdvisoryHandler: handler.LightwellAdvisoryHandler{
			DaoRegistry:          *daoReg,
			FeatureServiceClient: *fsClient,
		},
	}
	addLightwellRoute(engine, http.MethodGet, "/advisories", h.listAdvisories, rbac.RbacVerbRead)
}

// listAdvisories godoc
// @Summary      List Lightwell Advisories
// @ID           listLightwellNetworkAdvisories
// @Description  List security advisories for Lightwell remediated packages.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        repository       query  string  false  "Filter by repository name"
// @Param        package_name     query  string  false  "Filter by package name (substring match)"
// @Param        severity_min     query  string  false  "Minimum severity level (low, moderate, important, critical)"
// @Param        cve_id           query  string  false  "Filter by CVE ID (exact match)"
// @Param        limit            query  int     false  "Limit of results to return"
// @Param        offset           query  int     false  "Offset into results"
// @Success      200 {object} api.LightwellAdvisoryCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /advisories [get]
func (h *LightwellAdvisoryHandler) listAdvisories(c echo.Context) error {
	return h.ListAdvisories(c)
}
