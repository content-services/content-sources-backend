package handler

import (
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
	mavenMiddleware := append([]echo.MiddlewareFunc{DeprecateMavenPackagePath}, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/packages", ph.ListPackages, rbac.RbacVerbRead, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/java_packages/:group/:name", ph.ListJavaPackageVersions, rbac.RbacVerbRead, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/java_packages/:group/:name/:version", ph.GetJavaPackageDetail, rbac.RbacVerbRead, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/maven_packages/:group/:name", ph.ListMavenPackageVersions, rbac.RbacVerbRead, mavenMiddleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/maven_packages/:group/:name/:version", ph.GetMavenPackageDetail, rbac.RbacVerbRead, mavenMiddleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name/:version", ph.GetPythonPackageDetail, rbac.RbacVerbRead, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name", ph.GetPythonPackageVersions, rbac.RbacVerbRead, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/js_packages/:scope/:name/:version", ph.GetNpmPackageDetail, rbac.RbacVerbRead, middleware...)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/js_packages/:scope/:name", ph.GetNpmPackageVersions, rbac.RbacVerbRead, middleware...)
}

// DeprecateMavenPackagePath marks maven_packages responses as deprecated and points clients at java_packages.
func DeprecateMavenPackagePath(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		successor := strings.Replace(c.Request().URL.RequestURI(), "/maven_packages/", "/java_packages/", 1)
		c.Response().Header().Set("Deprecation", "true")
		c.Response().Header().Set("Link", "<"+successor+">; rel=\"successor-version\"")
		return next(c)
	}
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

// ListMavenPackageVersions godoc
// @Summary      List Maven Package Versions
// @ID           listMavenPackageVersions
// @Deprecated
// @Description  Deprecated. Use /repositories/{uuid}/java_packages/{group}/{name}. List all versions (builds) for a specific Maven package by group and name.
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
// @Deprecated
// @Description  Deprecated. Use /repositories/{uuid}/java_packages/{group}/{name}/{version}. Get builds for a specific Maven package by group, name, and version.
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

// ListJavaPackageVersions godoc
// @Summary      List Java Package Versions
// @ID           listJavaPackageVersions
// @Description  List all versions (builds) for a specific Java package by group and name.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        group path string true "Java package group ID"
// @Param        name path string true "Java package artifact ID"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.MavenPackageVersionsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/java_packages/{group}/{name} [get]
func (ph *PackageHandler) ListJavaPackageVersions(c echo.Context) error {
	return ph.ListMavenPackageVersions(c)
}

// GetJavaPackageDetail godoc
// @Summary      Get Java Package Detail
// @ID           getJavaPackageDetail
// @Description  Get builds for a specific Java package by group, name, and version.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        group path string true "Java package group ID"
// @Param        name path string true "Java package artifact ID"
// @Param        version path string true "Java package version"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.MavenPackageDetailResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/java_packages/{group}/{name}/{version} [get]
func (ph *PackageHandler) GetJavaPackageDetail(c echo.Context) error {
	return ph.GetMavenPackageDetail(c)
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
// @Summary      Get JS Package Versions
// @ID           getJsPackageVersions
// @Description  List versions for a JavaScript package by scope and name. The version is the upstream version, and builds lists each rebuild. Use "-" as the scope for unscoped packages.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        scope path string true "JS package scope (e.g. @types). Use - for unscoped packages."
// @Param        name path string true "JS package name"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.NpmPackageVersionsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/js_packages/{scope}/{name} [get]
func (ph *PackageHandler) GetNpmPackageVersions(c echo.Context) error {
	uuid := c.Param("uuid")
	scope := c.Param("scope")
	name := c.Param("name")
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeNpm {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not an npm repository")
	}

	versions, err := ph.DaoRegistry.LightwellPackage.ListNpmPackageVersions(ctx, repo.UUID, scope, name)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error retrieving package versions", err.Error())
	}

	return c.JSON(http.StatusOK, versions)
}

// GetNpmPackageDetail godoc
// @Summary      Get JS Package Detail
// @ID           getJsPackageDetail
// @Description  Get a JavaScript package version by scope, name, and upstream version. builds lists each rebuild. Use "-" as the scope for unscoped packages.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        scope path string true "JS package scope (e.g. @types). Use - for unscoped packages."
// @Param        name path string true "JS package name"
// @Param        version path string true "JS package upstream version"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.NpmPackageDetailResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/js_packages/{scope}/{name}/{version} [get]
func (ph *PackageHandler) GetNpmPackageDetail(c echo.Context) error {
	uuid := c.Param("uuid")
	scope := c.Param("scope")
	name := c.Param("name")
	version := c.Param("version")
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeNpm {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not an npm repository")
	}

	detail, err := ph.DaoRegistry.LightwellPackage.GetNpmPackageDetail(ctx, repo.UUID, scope, name, version)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error retrieving package builds", err.Error())
	}

	return c.JSON(http.StatusOK, detail)
}
