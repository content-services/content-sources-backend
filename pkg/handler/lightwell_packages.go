package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/lightwell/coords"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type LightwellPackagesHandler struct {
	DaoRegistry          dao.DaoRegistry
	TangClient           tangy.Tangy
	PulpClient           pulp_client.PulpClient
	FeatureServiceClient feature_service_client.FeatureServiceClient
}

func RegisterLightwellPackageRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, tangClient tangy.Tangy, pulpClient pulp_client.PulpClient, fsClient *feature_service_client.FeatureServiceClient) {
	h := LightwellPackagesHandler{
		DaoRegistry:          *daoReg,
		TangClient:           tangClient,
		PulpClient:           pulpClient,
		FeatureServiceClient: *fsClient,
	}
	addRepoRoute(engine, http.MethodGet, "/lightwell/packages", h.ListPackages, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/lightwell/package_versions", h.ListPackageVersions, rbac.RbacVerbRead)
}

// listLightwellPackages godoc
// @Summary      List Lightwell Packages (cross-repo)
// @ID           listLightwellPackages
// @Description  List packages aggregated across all Lightwell repositories, with optional filtering by ecosystem, name, and security level.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        ecosystem       query  string  false  "Filter by ecosystem (maven, python, npm)"
// @Param        name            query  string  false  "Filter by package name (substring match)"
// @Param        search          query  string  false  "Filter by package name or group/scope (substring match)"
// @Param        sort_by         query  string  false  "Sort the results. Valid fields: group/name, name, scope/name, last_updated. Each may be suffixed with :asc or :desc (default asc)."
// @Param        repository      query  string  false  "Filter by repository name"
// @Param        security_level  query  string  false  "Filter by security level (validated, remediated)"
// @Param        demo            query  bool    false  "Return demo repositories instead of production ones (default false)"
// @Param        limit           query  int     false  "Limit of results to return"
// @Param        offset          query  int     false  "Offset into results"
// @Success      200 {object} api.LightwellPackageCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/packages [get]
func (h *LightwellPackagesHandler) ListPackages(c echo.Context) error {
	page := ParsePagination(c)
	filters := parseLightwellPackageFilters(c)

	if err := validateContentType(filters.Ecosystem); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid ecosystem filter", err.Error())
	}

	sortColumn, sortDirection, err := parseLightwellSortBy(filters.SortBy)
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid sort_by parameter", err.Error())
	}

	_, orgID := GetAccountIdOrgId(c)
	features, err := h.FeatureServiceClient.GetEntitledFeatures(c.Request().Context(), orgID)
	if err != nil {
		log.Error().Err(err).Msg("error checking entitled features")
		resp := api.LightwellPackageCollectionResponse{Data: []api.LightwellPackageResponse{}}
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
		resp := api.LightwellPackageCollectionResponse{Data: []api.LightwellPackageResponse{}}
		collResp := SetCollectionResponseMetadata(&resp, c, 0)
		return c.JSON(http.StatusOK, collResp)
	}

	opts := dao.ListLightwellPackagesOptions{
		Demo:             filters.Demo,
		EntitledFeatures: lightwellFeatures,
		SortColumn:       sortColumn,
		SortDirection:    sortDirection,
		Limit:            int32(page.Limit),  //nolint:gosec // bounded by MaxLimit (200)
		Offset:           int32(page.Offset), //nolint:gosec // bounded by ParsePagination
	}
	if filters.Ecosystem != "" {
		opts.Ecosystem = &filters.Ecosystem
	}
	if filters.Name != "" {
		opts.Name = &filters.Name
	}
	if filters.Search != "" {
		opts.Search = &filters.Search
	}
	if filters.Repository != "" {
		opts.Repository = &filters.Repository
	}
	if filters.SecurityLevel != "" {
		opts.SecurityLevel = &filters.SecurityLevel
	}

	rows, total, err := h.DaoRegistry.LightwellPackage.ListPackages(c.Request().Context(), opts)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving packages", err.Error())
	}

	data := make([]api.LightwellPackageResponse, 0, len(rows))
	for _, row := range rows {
		data = append(data, mapRowToLightwellPackage(row))
	}

	resp := api.LightwellPackageCollectionResponse{Data: data}
	collResp := SetCollectionResponseMetadata(&resp, c, total)
	return c.JSON(http.StatusOK, collResp)
}

func mapRowToLightwellPackage(row dao.LightwellPackageRow) api.LightwellPackageResponse {
	versions := make([]string, len(row.Versions))
	releases := make([]api.ReleaseInfo, 0, len(row.Versions))
	for i, stored := range row.Versions {
		upstream := ""
		if i < len(row.UpstreamVersions) {
			upstream = row.UpstreamVersions[i]
		}
		// v0.1 clients read version as the upstream part. The mirror stores the full version string.
		version := lightwellAPIVersion(stored, upstream)
		versions[i] = version
		ri := api.ReleaseInfo{Version: version}
		if i < len(row.Releases) {
			ri.Release = row.Releases[i]
		}
		if i < len(row.PublishedAts) {
			ri.CreatedAt = row.PublishedAts[i]
		}
		releases = append(releases, ri)
	}
	return api.LightwellPackageResponse{
		Name:           row.Name,
		Group:          row.Group,
		Ecosystem:      row.Ecosystem,
		Repository:     row.RepositoryName,
		RepositoryUUID: row.RepositoryConfigurationUUID,
		Versions:       versions,
		LatestReleases: releases,
		LastUpdated:    row.LastUpdated,
	}
}

// lightwellSortColumns maps the UI-facing sort_by tokens to normalized sort columns.
// The maven-flavored "group/name" and npm-flavored "scope/name" tokens both resolve to
// the physical package_group ordering.
var lightwellSortColumns = map[string]string{
	"group/name":   "group_name",
	"scope/name":   "group_name",
	"name":         "name",
	"last_updated": "last_updated",
}

// parseLightwellSortBy parses a sort_by value like "group/name:desc" into a normalized
// column and direction. An empty value yields the default ("group_name", "asc"). An
// unknown field or direction returns an error listing the valid tokens.
func parseLightwellSortBy(sortBy string) (column string, direction string, err error) {
	if strings.TrimSpace(sortBy) == "" {
		return "group_name", "asc", nil
	}

	field := strings.TrimSpace(sortBy)
	direction = "asc"
	if parts := strings.SplitN(sortBy, ":", 2); len(parts) == 2 {
		field = strings.TrimSpace(parts[0])
		direction = strings.ToLower(strings.TrimSpace(parts[1]))
	}

	column, ok := lightwellSortColumns[field]
	if !ok {
		return "", "", fmt.Errorf("unknown sort field %q; valid sort_by fields are: group/name, name, scope/name, last_updated (each optionally suffixed with :asc or :desc)", field)
	}
	if direction != "asc" && direction != "desc" {
		return "", "", fmt.Errorf("unknown sort direction %q; valid directions are: asc, desc", direction)
	}
	return column, direction, nil
}

// lightwellAPIVersion is the version field on the v0.1 list responses.
// Rows imported before the split have an empty upstream version and already store the v0.1 version.
func lightwellAPIVersion(stored, upstream string) string {
	if upstream != "" {
		return upstream
	}
	return stored
}

// listLightwellPackageVersions godoc
// @Summary      List Lightwell Package Versions (cross-repo)
// @ID           listLightwellPackageVersions
// @Description  List individual package versions aggregated across all Lightwell repositories, with optional CVE-based filtering.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        ecosystem            query  string  false  "Filter by ecosystem (maven, python, npm)"
// @Param        name                 query  string  false  "Filter by package name (substring match)"
// @Param        security_level       query  string  false  "Filter by security level (validated, remediated)"
// @Param        repository           query  string  false  "Filter by repository name"
// @Param        resolves_cve_id      query  string  false  "Show only packages that resolve this CVE"
// @Param        vulnerable_to_cve_id query  string  false  "Show only packages vulnerable to this CVE"
// @Param        demo                 query  bool    false  "Return demo repositories instead of production ones (default false)"
// @Param        limit                query  int     false  "Limit of results to return"
// @Param        offset               query  int     false  "Offset into results"
// @Success      200 {object} api.LightwellPackageVersionCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/package_versions [get]
func (h *LightwellPackagesHandler) ListPackageVersions(c echo.Context) error {
	page := ParsePagination(c)
	filters := parseLightwellPackageVersionFilters(c)

	if err := validateContentType(filters.Ecosystem); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid ecosystem filter", err.Error())
	}

	_, orgID := GetAccountIdOrgId(c)
	features, err := h.FeatureServiceClient.GetEntitledFeatures(c.Request().Context(), orgID)
	if err != nil {
		log.Error().Err(err).Msg("error checking entitled features")
		resp := api.LightwellPackageVersionCollectionResponse{Data: []api.LightwellPackageVersionResponse{}}
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
		resp := api.LightwellPackageVersionCollectionResponse{Data: []api.LightwellPackageVersionResponse{}}
		collResp := SetCollectionResponseMetadata(&resp, c, 0)
		return c.JSON(http.StatusOK, collResp)
	}

	opts := dao.ListLightwellPackageVersionsOptions{
		Demo:             filters.Demo,
		EntitledFeatures: lightwellFeatures,
		Limit:            int32(page.Limit),  //nolint:gosec // bounded by MaxLimit (200)
		Offset:           int32(page.Offset), //nolint:gosec // bounded by ParsePagination
	}
	if filters.Ecosystem != "" {
		opts.Ecosystem = &filters.Ecosystem
	}
	if filters.Name != "" {
		opts.Name = &filters.Name
	}
	if filters.Repository != "" {
		opts.Repository = &filters.Repository
	}
	if filters.SecurityLevel != "" {
		opts.SecurityLevel = &filters.SecurityLevel
	}
	if filters.ResolvesCveID != "" {
		opts.ResolvesCveID = &filters.ResolvesCveID
	}
	if filters.VulnerableToCveID != "" {
		opts.VulnerableToCveID = &filters.VulnerableToCveID
	}

	rows, total, err := h.DaoRegistry.LightwellPackage.ListPackageVersions(c.Request().Context(), opts)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package versions", err.Error())
	}

	data := make([]api.LightwellPackageVersionResponse, 0, len(rows))
	for _, row := range rows {
		data = append(data, api.LightwellPackageVersionResponse{
			Name:           row.Name,
			Group:          row.Group,
			Version:        lightwellAPIVersion(row.Version, row.UpstreamVersion),
			Ecosystem:      row.Ecosystem,
			Repository:     row.RepositoryName,
			RepositoryUUID: row.RepositoryConfigurationUUID,
			Release:        row.Release,
			CreatedAt:      row.PublishedAt,
			Purl:           row.Purl,
			Coordinates:    coords.BuildCoordinates(row.Ecosystem, row.Group, row.Name),
		})
	}

	resp := api.LightwellPackageVersionCollectionResponse{Data: data}
	collResp := SetCollectionResponseMetadata(&resp, c, total)
	return c.JSON(http.StatusOK, collResp)
}

// --- filter helpers ---

func parseLightwellPackageFilters(c echo.Context) api.LightwellPackageFilterData {
	var f api.LightwellPackageFilterData
	_ = echo.QueryParamsBinder(c).
		String("ecosystem", &f.Ecosystem).
		String("name", &f.Name).
		String("search", &f.Search).
		String("sort_by", &f.SortBy).
		String("repository", &f.Repository).
		String("security_level", &f.SecurityLevel).
		Bool("demo", &f.Demo).
		BindError()
	return f
}

func parseLightwellPackageVersionFilters(c echo.Context) api.LightwellPackageVersionFilterData {
	var f api.LightwellPackageVersionFilterData
	_ = echo.QueryParamsBinder(c).
		String("ecosystem", &f.Ecosystem).
		String("name", &f.Name).
		String("security_level", &f.SecurityLevel).
		String("repository", &f.Repository).
		String("resolves_cve_id", &f.ResolvesCveID).
		String("vulnerable_to_cve_id", &f.VulnerableToCveID).
		Bool("demo", &f.Demo).
		BindError()
	return f
}

var validContentTypes = map[string]bool{
	config.ContentTypeMaven:  true,
	config.ContentTypePython: true,
	config.ContentTypeNpm:    true,
}

func validateContentType(ct string) error {
	if ct == "" {
		return nil
	}
	if !validContentTypes[ct] {
		return ce.NewErrorResponse(http.StatusBadRequest, "unsupported ecosystem", "must be maven, python, or npm")
	}
	return nil
}
