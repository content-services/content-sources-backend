package lightwell

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
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

func RegisterLightwellPackageRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, tangClient tangy.Tangy, pulpClient pulp_client.PulpClient) {
	h := LightwellPackageHandler{
		LightwellPackagesHandler: handler.LightwellPackagesHandler{
			DaoRegistry: *daoReg,
			TangClient:  tangClient,
			PulpClient:  pulpClient,
		},
		PackageHandler: handler.PackageHandler{
			DaoRegistry: *daoReg,
			TangClient:  tangClient,
			PulpClient:  pulpClient,
		},
	}
	addLightwellRoute(engine, http.MethodGet, "/packages", h.listPackages, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodGet, "/package_versions", h.listPackageVersions, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodGet, "/repositories/:uuid/packages", h.listRepoPackages, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodGet, "/repositories/:uuid/maven_packages/:group/:name", h.listMavenPackageVersions, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodGet, "/repositories/:uuid/maven_packages/:group/:name/:version", h.getMavenPackageDetail, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name/:version", h.getPythonPackageDetail, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name", h.getPythonPackageVersions, rbac.RbacVerbRead)
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
// @Param        security_level  query  string  false  "Filter by security level (validated, remediated)"
// @Param        limit           query  int     false  "Limit of results to return"
// @Param        offset          query  int     false  "Offset into results"
// @Success      200 {object} api.LightwellPackageCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
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
// @Param        limit           query  int     false  "Limit of results to return"
// @Param        offset          query  int     false  "Offset into results"
// @Success      200 {object} api.LightwellPackageVersionCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
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
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PackageResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/packages [get]
func (h *LightwellPackageHandler) listRepoPackages(c echo.Context) error {
	return h.PackageHandler.ListPackages(c)
}

// listMavenPackageVersions godoc
// @Summary      List Maven Package Versions
// @ID           listLightwellMavenPackageVersions
// @Description  List all versions for a specific Maven package by group and name.
// @Tags         lightwell
// @Param        uuid path string true "Repository UUID"
// @Param        group path string true "Maven package group ID"
// @Param        name path string true "Maven package artifact ID"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.MavenPackageVersionsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/maven_packages/{group}/{name} [get]
func (h *LightwellPackageHandler) listMavenPackageVersions(c echo.Context) error {
	return h.PackageHandler.ListMavenPackageVersions(c)
}

// getMavenPackageDetail godoc
// @Summary      Get Maven Package Detail
// @ID           getLightwellMavenPackageDetail
// @Description  Get builds for a specific Maven package by group, name, and version.
// @Tags         lightwell
// @Param        uuid path string true "Repository UUID"
// @Param        group path string true "Maven package group ID"
// @Param        name path string true "Maven package artifact ID"
// @Param        version path string true "Maven package version"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.MavenPackageDetailResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/maven_packages/{group}/{name}/{version} [get]
func (h *LightwellPackageHandler) getMavenPackageDetail(c echo.Context) error {
	return h.PackageHandler.GetMavenPackageDetail(c)
}

// getPythonPackageDetail godoc
// @Summary      Get Python Package Detail
// @ID           getLightwellPythonPackageDetail
// @Description  Get metadata and distributions for a specific Python package by name and version.
// @Tags         lightwell
// @Param        uuid path string true "Repository UUID"
// @Param        name path string true "Python package normalized name"
// @Param        version path string true "Python package version"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PythonPackageDetailResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/python_packages/{name}/{version} [get]
func (h *LightwellPackageHandler) getPythonPackageDetail(c echo.Context) error {
	return h.PackageHandler.GetPythonPackageDetail(c)
}

// getPythonPackageVersions godoc
// @Summary      Get Python Package Versions
// @ID           getLightwellPythonPackageVersions
// @Description  Get metadata and distributions for all versions of a Python package by name.
// @Tags         lightwell
// @Param        uuid path string true "Repository UUID"
// @Param        name path string true "Python package normalized name"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PythonPackageVersionsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/python_packages/{name} [get]
func (h *LightwellPackageHandler) getPythonPackageVersions(c echo.Context) error {
	return h.PackageHandler.GetPythonPackageVersions(c)
}
