package handler

import (
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type LightwellAdvisoryHandler struct {
	DaoRegistry          dao.DaoRegistry
	FeatureServiceClient feature_service_client.FeatureServiceClient
}

func RegisterLightwellAdvisoryRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, fsClient *feature_service_client.FeatureServiceClient) {
	h := LightwellAdvisoryHandler{
		DaoRegistry:          *daoReg,
		FeatureServiceClient: *fsClient,
	}
	addRepoRoute(engine, http.MethodGet, "/lightwell/advisories", h.ListAdvisories, rbac.RbacVerbRead)
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
// @Param        package_version  query  string  false  "Filter by package version (substring match on advisory_id)"
// @Param        name             query  string  false  "Filter by advisory id or alias (substring match)"
// @Param        severity_min     query  string  false  "Minimum severity level (low, moderate, important, critical)"
// @Param        cve_id           query  string  false  "Filter by CVE ID (exact match)"
// @Param        latest_release   query  bool    false  "When true, return only advisories from the highest Lightwell rebuild (baseline, then novel, then hotfix) of the package/version. Requires package_name and package_version."
// @Param        limit            query  int     false  "Limit of results to return"
// @Param        offset           query  int     false  "Offset into results"
// @Success      200 {object} api.LightwellAdvisoryCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/advisories [get]
func (h *LightwellAdvisoryHandler) ListAdvisories(c echo.Context) error {
	_, orgID := GetAccountIdOrgId(c)

	features, err := h.FeatureServiceClient.GetEntitledFeatures(c.Request().Context(), orgID)
	if err != nil {
		log.Error().Err(err).Msg("error checking entitled features")
		resp := api.LightwellAdvisoryCollectionResponse{Data: []api.LightwellAdvisoryResponse{}}
		collResp := SetCollectionResponseMetadata(&resp, c, 0)
		return c.JSON(http.StatusOK, collResp)
	}

	var lightwellFeatures []string
	for _, f := range features {
		if strings.HasPrefix(f, "lightwell-") {
			lightwellFeatures = append(lightwellFeatures, f)
		}
	}
	if len(lightwellFeatures) == 0 {
		resp := api.LightwellAdvisoryCollectionResponse{Data: []api.LightwellAdvisoryResponse{}}
		collResp := SetCollectionResponseMetadata(&resp, c, 0)
		return c.JSON(http.StatusOK, collResp)
	}

	page := ParsePagination(c)
	filters := parseLightwellAdvisoryFilters(c)
	if filters.LatestRelease && (filters.PackageName == "" || filters.PackageVersion == "") {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error listing advisories", "latest_release requires package_name and package_version")
	}

	opts := dao.ListLightwellAdvisoriesOptions{
		SeverityMin:      filters.SeverityMin,
		LatestRelease:    filters.LatestRelease,
		EntitledFeatures: lightwellFeatures,
		Limit:            int32(page.Limit),  //nolint:gosec // bounded by MaxLimit (200)
		Offset:           int32(page.Offset), //nolint:gosec // bounded by ParsePagination
	}
	if filters.Repository != "" {
		opts.RepoName = &filters.Repository
	}
	if filters.PackageName != "" {
		opts.PackageName = &filters.PackageName
	}
	if filters.PackageVersion != "" {
		opts.PackageVersion = &filters.PackageVersion
	}
	if filters.CveID != "" {
		opts.CveID = &filters.CveID
	}
	if filters.Name != "" {
		opts.Name = &filters.Name
	}

	data, totalCount, err := h.DaoRegistry.LightwellAdvisory.ListAdvisories(c.Request().Context(), opts)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error listing advisories", err.Error())
	}

	resp := api.LightwellAdvisoryCollectionResponse{Data: data}
	collResp := SetCollectionResponseMetadata(&resp, c, totalCount)
	return c.JSON(http.StatusOK, collResp)
}

func parseLightwellAdvisoryFilters(c echo.Context) api.LightwellAdvisoryFilterData {
	var filters api.LightwellAdvisoryFilterData
	_ = echo.QueryParamsBinder(c).
		String("repository", &filters.Repository).
		String("package_name", &filters.PackageName).
		String("package_version", &filters.PackageVersion).
		String("severity_min", &filters.SeverityMin).
		String("cve_id", &filters.CveID).
		String("name", &filters.Name).
		Bool("latest_release", &filters.LatestRelease).
		BindError()
	return filters
}
