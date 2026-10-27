package lightwell

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/labstack/echo/v4"
)

// These imports are used by swag for OpenAPI generation
// TODO remove when these handlers are implemented here
var _ api.LightwellPackageCollectionResponse
var _ ce.ErrorResponse

type LightwellPackageHandler struct {
	handler.LightwellPackagesHandler
	PackageHandler handler.PackageHandler
}

func RegisterLightwellPackageRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, tangClient tangy.Tangy, pulpClient pulp_client.PulpClient, fsClient *feature_service_client.FeatureServiceClient, middleware ...echo.MiddlewareFunc) {
	h := LightwellPackageHandler{
		LightwellPackagesHandler: handler.LightwellPackagesHandler{
			DaoRegistry:          *daoReg,
			TangClient:           tangClient,
			PulpClient:           pulpClient,
			FeatureServiceClient: *fsClient,
		},
		PackageHandler: handler.PackageHandler{
			DaoRegistry: *daoReg,
			TangClient:  tangClient,
			PulpClient:  pulpClient,
		},
	}
	addLightwellRoute(engine, http.MethodGet, "/packages", h.listPackages, rbac.RbacVerbRead, middleware...)
	addLightwellRoute(engine, http.MethodGet, "/package_versions", h.listPackageVersions, rbac.RbacVerbRead, middleware...)
	addLightwellRoute(engine, http.MethodGet, "/repositories/:uuid/packages", h.listRepoPackages, rbac.RbacVerbRead, middleware...)
	addLightwellRoute(engine, http.MethodGet, "/repositories/:uuid/java_packages/:group/:name", h.listJavaPackageVersions, rbac.RbacVerbRead, middleware...)
	addLightwellRoute(engine, http.MethodGet, "/repositories/:uuid/java_packages/:group/:name/:version", h.getJavaPackageDetail, rbac.RbacVerbRead, middleware...)
	addLightwellRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name/:version", h.getPythonPackageDetail, rbac.RbacVerbRead, middleware...)
	addLightwellRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name", h.getPythonPackageVersions, rbac.RbacVerbRead, middleware...)
	addLightwellRoute(engine, http.MethodGet, "/repositories/:uuid/js_packages/:scope/:name/:version", h.getJsPackageDetail, rbac.RbacVerbRead, middleware...)
	addLightwellRoute(engine, http.MethodGet, "/repositories/:uuid/js_packages/:scope/:name", h.getJsPackageVersions, rbac.RbacVerbRead, middleware...)
}

// listPackages godoc
// @Summary      List Lightwell Packages
// @ID           listLightwellNetworkPackages
// @Description  List packages aggregated across all Lightwell repositories.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        ecosystem       query  string  false  "Filter by ecosystem (maven, python, npm)"
// @Param        name            query  string  false  "Filter by package name (substring match)"
// @Param        search          query  string  false  "Filter by package name or group/scope (substring match)"
// @Param        sort_by         query  string  false  "Sort the results. Valid fields: group/name, name, scope/name, last_publication. Each may be suffixed with :asc or :desc (default asc)."
// @Param        repository      query  string  false  "Filter by repository name"
// @Param        security_level  query  string  false  "Filter by security level (validated, remediated)"
// @Param        demo            query  bool    false  "Return demo repositories instead of production ones (default false)"
// @Param        limit           query  int     false  "Limit of results to return"
// @Param        offset          query  int     false  "Offset into results"
// @Success      200 {object} api.LightwellPackageCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /packages [get]
func (h *LightwellPackageHandler) listPackages(c echo.Context) error {
	return h.ListPackages(c)
}

// listPackageVersions godoc
// @Summary      List Lightwell Package Versions
// @ID           listLightwellNetworkPackageVersions
// @Description  List package versions aggregated across all Lightwell repositories.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        ecosystem       query  string  false  "Filter by ecosystem (maven, python, npm)"
// @Param        name            query  string  false  "Filter by package name (substring match)"
// @Param        security_level  query  string  false  "Filter by security level (validated, remediated)"
// @Param        resolves_cve    query  string  false  "Filter versions that resolve specific CVE ID"
// @Param        vulnerable_to_cve query string false "Filter versions vulnerable to specific CVE ID"
// @Param        demo            query  bool    false  "Return demo repositories instead of production ones (default false)"
// @Param        limit           query  int     false  "Limit of results to return"
// @Param        offset          query  int     false  "Offset into results"
// @Success      200 {object} api.LightwellPackageVersionCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /package_versions [get]
func (h *LightwellPackageHandler) listPackageVersions(c echo.Context) error {
	return h.ListPackageVersions(c)
}

// listRepoPackages godoc
// @Summary      List Repository Packages
// @ID           listLightwellRepoPackages
// @Description  List packages for a specific Lightwell repository.
// @Tags         lightwell
// @Param        uuid path string true "Repository UUID"
// @Param        offset query int false "Starting point for pagination. Default: 0"
// @Param        limit query int false "Number of items per page. Default: 100"
// @Param        search query string false "Search term to filter packages."
// @Param        sort_by query string false "Sort field. Valid fields are name and last_updated. Append :asc or :desc. name defaults to asc. last_updated defaults to desc."
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PackageResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/packages [get]
func (h *LightwellPackageHandler) listRepoPackages(c echo.Context) error {
	return h.PackageHandler.ListPackages(c)
}

// listJavaPackageVersions godoc
// @Summary      List Java Package Versions
// @ID           listLightwellJavaPackageVersions
// @Description  List all versions for a specific Java package by group and name.
// @Tags         lightwell
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
func (h *LightwellPackageHandler) listJavaPackageVersions(c echo.Context) error {
	return h.PackageHandler.ListJavaPackageVersions(c)
}

// getJavaPackageDetail godoc
// @Summary      Get Java Package Detail
// @ID           getLightwellJavaPackageDetail
// @Description  Get builds for a specific Java package by group, name, and version.
// @Tags         lightwell
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
func (h *LightwellPackageHandler) getJavaPackageDetail(c echo.Context) error {
	return h.PackageHandler.GetJavaPackageDetail(c)
}

// getPythonPackageDetail godoc
// @Summary      Get Python Package Detail
// @ID           getLightwellPythonPackageDetail
// @Description  Get metadata and builds for a specific Python package by name and version.
// @Tags         lightwell
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
func (h *LightwellPackageHandler) getPythonPackageDetail(c echo.Context) error {
	return h.PackageHandler.GetPythonPackageDetail(c)
}

// getPythonPackageVersions godoc
// @Summary      Get Python Package Versions
// @ID           getLightwellPythonPackageVersions
// @Description  List versions for a Python package by name. The version is the upstream version, and builds lists each rebuild.
// @Tags         lightwell
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
func (h *LightwellPackageHandler) getPythonPackageVersions(c echo.Context) error {
	return h.PackageHandler.GetPythonPackageVersions(c)
}

// getJsPackageDetail godoc
// @Summary      Get JS Package Detail
// @ID           getLightwellJsPackageDetail
// @Description  Get a JavaScript package version by scope, name, and upstream version. builds lists each rebuild. Use "-" as the scope for unscoped packages.
// @Tags         lightwell
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
func (h *LightwellPackageHandler) getJsPackageDetail(c echo.Context) error {
	return h.PackageHandler.GetNpmPackageDetail(c)
}

// getJsPackageVersions godoc
// @Summary      Get JS Package Versions
// @ID           getLightwellJsPackageVersions
// @Description  List versions for a JavaScript package by scope and name. The version is the upstream version, and builds lists each rebuild. Use "-" as the scope for unscoped packages.
// @Tags         lightwell
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
func (h *LightwellPackageHandler) getJsPackageVersions(c echo.Context) error {
	return h.PackageHandler.GetNpmPackageVersions(c)
}
