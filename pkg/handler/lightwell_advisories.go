package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

type LightwellAdvisoryHandler struct {
	DaoRegistry dao.DaoRegistry
}

func RegisterLightwellAdvisoryRoutes(engine *echo.Group, daoReg *dao.DaoRegistry) {
	h := LightwellAdvisoryHandler{DaoRegistry: *daoReg}
	addRepoRoute(engine, http.MethodGet, "/lightwell/advisories", h.list, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/lightwell/repositories/:repository_name/advisories", h.listRepoAdvisories, rbac.RbacVerbRead)
}

// listLightwellAdvisories godoc
// @Summary      List Lightwell Advisories
// @ID           listLightwellAdvisories
// @Description  List security advisories for Lightwell remediated packages with optional filtering.
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
// @Router       /lightwell/advisories [get]
func (h *LightwellAdvisoryHandler) list(c echo.Context) error {
	page := ParsePagination(c)
	filters := parseLightwellAdvisoryFilters(c)

	opts := dao.ListLightwellAdvisoriesOptions{
		SeverityMin: filters.SeverityMin,
		Limit:       int32(page.Limit),  //nolint:gosec // bounded by MaxLimit (200)
		Offset:      int32(page.Offset), //nolint:gosec // bounded by ParsePagination
	}
	if filters.Repository != "" {
		opts.RepoName = &filters.Repository
	}
	if filters.PackageName != "" {
		opts.PackageName = &filters.PackageName
	}
	if filters.CveID != "" {
		opts.CveID = &filters.CveID
	}

	data, totalCount, err := h.DaoRegistry.LightwellAdvisory.ListAdvisories(c.Request().Context(), opts)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error listing advisories", err.Error())
	}

	resp := api.LightwellAdvisoryCollectionResponse{Data: data}
	collResp := setCollectionResponseMetadata(&resp, c, totalCount)
	return c.JSON(http.StatusOK, collResp)
}

func parseLightwellAdvisoryFilters(c echo.Context) api.LightwellAdvisoryFilterData {
	var filters api.LightwellAdvisoryFilterData
	_ = echo.QueryParamsBinder(c).
		String("repository", &filters.Repository).
		String("package_name", &filters.PackageName).
		String("severity_min", &filters.SeverityMin).
		String("cve_id", &filters.CveID).
		BindError()
	return filters
}

func (h *LightwellAdvisoryHandler) listRepoAdvisories(c echo.Context) error {
	repoName := c.Param("repository_name")
	c.QueryParams().Set("repository", repoName)
	return h.list(c)
}
