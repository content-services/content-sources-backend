package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

type CoverageReportHandler struct {
	DaoRegistry dao.DaoRegistry
}

func checkLightwellBeaconAndLensAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := CheckLightwellBeaconAndLensAccessible(c.Request().Context()); err != nil {
			return err
		}
		return next(c)
	}
}

func RegisterCoverageReportRoutes(engine *echo.Group, daoReg *dao.DaoRegistry) {
	ch := CoverageReportHandler{DaoRegistry: *daoReg}
	addRepoRoute(engine, http.MethodPost, "/coverage_reports/", ch.createCoverageReport, rbac.RbacVerbWrite, checkLightwellBeaconAndLensAccessible)
	addRepoRoute(engine, http.MethodGet, "/coverage_reports/:uuid", ch.getCoverageReport, rbac.RbacVerbRead, checkLightwellBeaconAndLensAccessible)
	addRepoRoute(engine, http.MethodGet, "/coverage_reports/:uuid/packages", ch.listCoverageReportPackages, rbac.RbacVerbRead, checkLightwellBeaconAndLensAccessible)
}

// CreateCoverageReport godoc
// @Summary      Create coverage report
// @ID           createCoverageReport
// @Description  Upload a manifest file and start coverage analysis.
// @Tags         coverage_reports
// @Accept       multipart/form-data
// @Produce      json
// @Param        file formData file true "Manifest file (CycloneDX, SPDX, etc.)"
// @Success      201 {object} api.CoverageReportResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /coverage_reports/ [post]
func (ch *CoverageReportHandler) createCoverageReport(c echo.Context) error {
	if _, err := c.FormFile("file"); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", "file is required")
	}

	report, err := stubCreateCoverageReport()
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error loading fixture", err.Error())
	}

	return c.JSON(http.StatusCreated, report)
}

// GetCoverageReport godoc
// @Summary      Get coverage report
// @ID           getCoverageReport
// @Description  Return a coverage report by UUID.
// @Tags         coverage_reports
// @Produce      json
// @Param        uuid path string true "Coverage report UUID"
// @Success      200 {object} api.CoverageReportResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /coverage_reports/{uuid} [get]
func (ch *CoverageReportHandler) getCoverageReport(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)

	report, err := ch.DaoRegistry.CoverageReport.Fetch(c.Request().Context(), orgID, c.Param("uuid"))
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching coverage report", err.Error())
	}

	return c.JSON(http.StatusOK, report)
}

// ListCoverageReportPackages godoc
// @Summary      List coverage report packages
// @ID           listCoverageReportPackages
// @Description  Return paginated packages for a completed coverage report.
// @Tags         coverage_reports
// @Produce      json
// @Param        uuid path string true "Coverage report UUID"
// @Param        search query string false "Filter by package name"
// @Param        ecosystem query string false "Filter by ecosystem"
// @Param        covered query bool false "Filter by coverage status (true = covered, false = not covered)"
// @Param        offset query int false "Starting point for pagination. Default: 0"
// @Param        limit query int false "Number of items per page. Default: 100"
// @Success      200 {object} api.CoverageReportPackageCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /coverage_reports/{uuid}/packages [get]
func (ch *CoverageReportHandler) listCoverageReportPackages(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)

	req := api.ListCoverageReportPackagesRequest{}
	if err := c.Bind(&req); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	pageData := ParsePagination(c)

	response, totalCount, err := ch.DaoRegistry.CoverageReport.ListPackages(c.Request().Context(), orgID, c.Param("uuid"), pageData, req)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing coverage report packages", err.Error())
	}

	return c.JSON(http.StatusOK, setCollectionResponseMetadata(&response, c, totalCount))
}
