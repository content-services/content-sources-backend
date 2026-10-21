package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/labstack/echo/v4"
)

var errDistributionNotFound = errors.New("repository distribution not found")

var errRepositoryNotFound = errors.New("repository not found")

type PackageHandler struct {
	DaoRegistry dao.DaoRegistry
	TangClient  tangy.Tangy
	PulpClient  pulp_client.PulpClient
}

func RegisterPackageRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, tangClient tangy.Tangy, pulpClient pulp_client.PulpClient, middleware ...echo.MiddlewareFunc) {
	ph := PackageHandler{
		DaoRegistry: *daoReg,
		TangClient:  tangClient,
		PulpClient:  pulpClient,
	}
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/packages", ph.ListPackages, rbac.RbacVerbRead, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/maven_packages/:group/:name", ph.ListMavenPackageVersions, rbac.RbacVerbRead, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/maven_packages/:group/:name/:version", ph.GetMavenPackageDetail, rbac.RbacVerbRead, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name/:version", ph.GetPythonPackageDetail, rbac.RbacVerbRead, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name", ph.GetPythonPackageVersions, rbac.RbacVerbRead, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/npm_packages/:scope/:name", ph.GetNpmPackageVersions, rbac.RbacVerbRead, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/npm_packages/:scope/:name/:version", ph.GetNpmPackageDetail, rbac.RbacVerbRead, middleware...)
}

func (ph *PackageHandler) fetchLightwellRepo(c echo.Context, uuid string) (api.RepositoryResponse, error) {
	_, orgID := GetAccountIdOrgId(c)
	ctx := c.Request().Context()

	repos, _, err := ph.DaoRegistry.RepositoryConfig.List(ctx, orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: uuid})
	if err != nil {
		return api.RepositoryResponse{}, err
	}

	err = &ce.DaoError{
		NotFound: true,
		Message:  "Repository not found",
	}
	if len(repos.Data) == 0 {
		return api.RepositoryResponse{}, err
	}
	return repos.Data[0], nil
}

// ListPackages godoc
// @Summary      List Packages
// @ID           listPackages
// @Description  List packages for Maven (group and name), Python (name), or npm (scope and name) repositories. Returns empty results for other content types.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        offset query int false "Starting point for pagination. Default: 0"
// @Param        limit query int false "Number of items per page. Default: 100"
// @Param        search query string false "Term to filter and retrieve items that match the specified search criteria. For Maven, search term can include name or group. For Python and npm, search term can include name (including scoped names like @types/)."
// @Param        sort_by query string false "Sort field. Valid fields are name and last_updated. Append :asc or :desc. name defaults to asc. last_updated defaults to desc."
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PackageResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/packages [get]
func (ph *PackageHandler) ListPackages(c echo.Context) error {
	listPackagesRequest := api.ListPackagesRequest{}
	if err := c.Bind(&listPackagesRequest); err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error binding parameters", err.Error())
	}

	pageData := ParsePagination(c)
	sortField, sortDir, err := parsePackageSortBy(pageData.SortBy)
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid sort_by", err.Error())
	}

	uuid := c.Param("uuid")
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	switch repo.ContentType {
	case config.ContentTypeMaven, config.ContentTypePython, config.ContentTypeNpm:
		resp, err := ph.DaoRegistry.LightwellPackage.ListRepositoryPackages(ctx, repo.UUID, listPackagesRequest.Search, packageSortField(repo.ContentType, sortField), sortDir, pageData.Limit, pageData.Offset)
		if err != nil {
			return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error retrieving packages", err.Error())
		}
		return c.JSON(http.StatusOK, resp)
	default:
		return c.JSON(http.StatusOK, api.PackageResponse{
			Results: []api.PackageItem{},
			Total:   0,
			Limit:   pageData.Limit,
			Offset:  pageData.Offset,
		})
	}
}

func parsePackageSortBy(sortBy string) (string, string, error) {
	const message = "valid fields are name and last_updated, each optionally followed by :asc or :desc"
	sortBy = strings.TrimSpace(sortBy)
	if sortBy == "" {
		return "last_updated", "desc", nil
	}

	field := sortBy
	dir := ""
	if i := strings.LastIndex(sortBy, ":"); i >= 0 {
		field = strings.TrimSpace(sortBy[:i])
		dir = strings.TrimSpace(sortBy[i+1:])
	}
	switch field {
	case "name", "last_updated":
	default:
		return "", "", fmt.Errorf("invalid sort_by %q: %s", sortBy, message)
	}
	if dir == "" {
		dir = "asc"
		if field == "last_updated" {
			dir = "desc"
		}
	}
	switch dir {
	case "asc", "desc":
	default:
		return "", "", fmt.Errorf("invalid sort_by %q: %s", sortBy, message)
	}
	return field, dir, nil
}

func packageSortField(contentType, field string) string {
	if field == "name" && (contentType == config.ContentTypeMaven || contentType == config.ContentTypeNpm) {
		return "group/name"
	}
	return field
}

func (ph *PackageHandler) resolveRepository(ctx context.Context, orgID, basePath, repoUUID string) (pulp_client.PulpClient, string, error) {
	domainName, err := ph.DaoRegistry.Domain.FetchOrCreateDomain(ctx, orgID)
	if err != nil {
		return nil, "", err
	}

	pulpClient := ph.PulpClient.WithDomain(domainName)
	href, err := pulpClient.ResolveRepositoryFromBasePath(ctx, basePath)
	if err != nil {
		return nil, "", fmt.Errorf("repository for UUID %v: %w", repoUUID, err)
	}
	if href == nil {
		return nil, "", fmt.Errorf("repository for UUID %v: %w", repoUUID, errRepositoryNotFound)
	}

	return pulpClient, *href, nil
}

func (ph *PackageHandler) resolveRepositoryHref(ctx context.Context, orgID, basePath, repoUUID string) (string, error) {
	_, href, err := ph.resolveRepository(ctx, orgID, basePath, repoUUID)
	return href, err
}

func (ph *PackageHandler) repositoryHrefErrorResponse(err error) error {
	if errors.Is(err, errDistributionNotFound) {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution not found")
	}
	if errors.Is(err, errRepositoryNotFound) {
		return ce.NewErrorResponse(http.StatusNotFound, "Repository not found", err.Error())
	}
	var daoError *ce.DaoError
	if errors.As(err, &daoError) {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching or creating domain", err.Error())
	}
	return ce.NewErrorResponse(http.StatusInternalServerError, "Error finding repository distribution", err.Error())
}

// ListMavenPackageVersions godoc
// @Summary      List Maven Package Versions
// @ID           listMavenPackageVersions
// @Description  List all versions (builds) for a specific Maven package by group and name.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        group path string true "Maven package group ID"
// @Param        name path string true "Maven package artifact ID"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.MavenPackageVersionsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/maven_packages/{group}/{name} [get]
func (ph *PackageHandler) ListMavenPackageVersions(c echo.Context) error {
	uuid := c.Param("uuid")
	groupID := c.Param("group")
	name := c.Param("name")
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeMaven {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Maven repository")
	}

	versions, err := ph.DaoRegistry.LightwellPackage.ListMavenPackageVersions(ctx, repo.UUID, groupID, name)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error retrieving package versions", err.Error())
	}

	return c.JSON(http.StatusOK, versions)
}

// GetMavenPackageDetail godoc
// @Summary      Get Maven Package Detail
// @ID           getPackageDetail
// @Description  Get builds for a specific Maven package by group, name, and version.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        group path string true "Maven package group ID"
// @Param        name path string true "Maven package artifact ID"
// @Param        version path string true "Maven package version"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.MavenPackageDetailResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/maven_packages/{group}/{name}/{version} [get]
func (ph *PackageHandler) GetMavenPackageDetail(c echo.Context) error {
	uuid := c.Param("uuid")
	groupID := c.Param("group")
	name := c.Param("name")
	version := c.Param("version")
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeMaven {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Maven repository")
	}

	detail, err := ph.DaoRegistry.LightwellPackage.GetMavenPackageDetail(ctx, repo.UUID, groupID, name, version)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error retrieving package builds", err.Error())
	}

	return c.JSON(http.StatusOK, detail)
}

// GetPythonPackageVersions godoc
// @Summary      Get Python Package Versions
// @ID           getPythonPackageVersions
// @Description  Get metadata and distributions for all versions of a Python package by name.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        name path string true "Python package normalized name"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PythonPackageVersionsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/python_packages/{name} [get]
func (ph *PackageHandler) GetPythonPackageVersions(c echo.Context) error {
	uuid := c.Param("uuid")
	name := c.Param("name")
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypePython {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Python repository")
	}

	versions, err := ph.DaoRegistry.LightwellPackage.ListPythonPackageVersions(ctx, repo.UUID, name)
	if err != nil {
		title := "Error retrieving package versions"
		var daoErr *ce.DaoError
		if errors.As(err, &daoErr) && daoErr.NotFound {
			title = "Package not found"
		}
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), title, err.Error())
	}

	return c.JSON(http.StatusOK, versions)
}

// GetPythonPackageDetail godoc
// @Summary      Get Python Package Detail
// @ID           getPythonPackageDetail
// @Description  Get metadata and builds for a specific Python package by name and version.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        name path string true "Python package normalized name"
// @Param        version path string true "Python package version"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PythonPackageDetailResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/python_packages/{name}/{version} [get]
func (ph *PackageHandler) GetPythonPackageDetail(c echo.Context) error {
	uuid := c.Param("uuid")
	name := c.Param("name")
	version := c.Param("version")
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypePython {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Python repository")
	}

	detail, err := ph.DaoRegistry.LightwellPackage.GetPythonPackageDetail(ctx, repo.UUID, name, version)
	if err != nil {
		title := "Error retrieving package detail"
		var daoErr *ce.DaoError
		if errors.As(err, &daoErr) && daoErr.NotFound {
			title = "Package not found"
		}
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), title, err.Error())
	}

	return c.JSON(http.StatusOK, detail)
}

func npmPackageName(scope, name string) string {
	if scope == "" || scope == "-" {
		return name
	}
	return scope + "/" + name
}

func ParseNpmPackageName(fullName string) (scope, name string) {
	if strings.HasPrefix(fullName, "@") {
		parts := strings.SplitN(fullName, "/", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	}
	return "-", fullName
}

// GetNpmPackageVersions godoc
// @Summary      Get NPM Package Versions
// @ID           getNpmPackageVersions
// @Description  Get tarball info for all versions of an npm package by scope and name. Use "-" as the scope for unscoped packages.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        scope path string true "NPM package scope (e.g. @types). Use - for unscoped packages."
// @Param        name path string true "NPM package name"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.NpmPackageVersionsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/npm_packages/{scope}/{name} [get]
func (ph *PackageHandler) GetNpmPackageVersions(c echo.Context) error {
	uuid := c.Param("uuid")
	scope := c.Param("scope")
	name := c.Param("name")
	packageName := npmPackageName(scope, name)
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeNpm {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not an npm repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, repo.OrgID, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	tangResp, err := ph.TangClient.NpmPackageVersionsGet(ctx, repositoryHref, packageName)
	if err != nil {
		if errors.Is(err, tangy.ErrNpmPackageNotFound) {
			return ce.NewErrorResponse(http.StatusNotFound, "Package not found", err.Error())
		}
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package versions", err.Error())
	}

	versions := make([]api.NpmPackageDetailResponse, len(tangResp))
	for i, detail := range tangResp {
		versions[i] = mapNpmPackageDetailToAPI(detail, scope, name)
	}

	return c.JSON(http.StatusOK, api.NpmPackageVersionsResponse{
		Scope:    scope,
		Name:     name,
		Versions: versions,
	})
}

// GetNpmPackageDetail godoc
// @Summary      Get NPM Package Detail
// @ID           getNpmPackageDetail
// @Description  Get tarball info for a specific npm package by scope, name, and version. Use "-" as the scope for unscoped packages.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        scope path string true "NPM package scope (e.g. @types). Use - for unscoped packages."
// @Param        name path string true "NPM package name"
// @Param        version path string true "NPM package version"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.NpmPackageDetailResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/npm_packages/{scope}/{name}/{version} [get]
func (ph *PackageHandler) GetNpmPackageDetail(c echo.Context) error {
	uuid := c.Param("uuid")
	scope := c.Param("scope")
	name := c.Param("name")
	version := c.Param("version")
	packageName := npmPackageName(scope, name)
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeNpm {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not an npm repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, repo.OrgID, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	tangResp, err := ph.TangClient.NpmPackageGet(ctx, repositoryHref, packageName, version)
	if err != nil {
		if errors.Is(err, tangy.ErrNpmPackageNotFound) {
			return ce.NewErrorResponse(http.StatusNotFound, "Package not found", err.Error())
		}
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package detail", err.Error())
	}

	return c.JSON(http.StatusOK, mapNpmPackageDetailToAPI(tangResp, scope, name))
}

func mapNpmPackageDetailToAPI(tangDetail tangy.NpmPackageDetail, scope, name string) api.NpmPackageDetailResponse {
	latestVersions := make([]api.ReleaseInfo, len(tangDetail.LatestVersions))
	for i, ver := range tangDetail.LatestVersions {
		latestVersions[i] = api.ReleaseInfo{
			Version:   ver.Version,
			CreatedAt: ver.CreatedAt,
		}
	}

	return api.NpmPackageDetailResponse{
		Scope:     scope,
		Name:      name,
		Version:   tangDetail.Version,
		CreatedAt: tangDetail.CreatedAt,
		Tarball: api.NpmTarball{
			RelativePath: tangDetail.Tarball.RelativePath,
			Filename:     tangDetail.Tarball.Filename,
			Sha256:       tangDetail.Tarball.Sha256,
			Size:         tangDetail.Tarball.Size,
		},
		UpstreamVersions: tangDetail.Versions,
		LatestVersions:   latestVersions,
	}
}
