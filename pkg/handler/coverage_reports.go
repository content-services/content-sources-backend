package handler

import (
	"errors"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

type CoverageReportHandler struct{}

func checkLightwellBeaconAndLensAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := CheckLightwellBeaconAndLensAccessible(c.Request().Context()); err != nil {
			return err
		}
		return next(c)
	}
}

func RegisterCoverageReportRoutes(engine *echo.Group) {
	ch := CoverageReportHandler{}
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
	report, err := stubGetCoverageReport(c.Param("uuid"))
	if errors.Is(err, errStubCoverageReportNotFound) {
		return ce.NewErrorResponse(http.StatusNotFound, "Coverage report not found", "Report is not available or analysis is incomplete")
	}
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error loading fixture", err.Error())
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
// @Param        status query string false "Filter by package match status (possible values: in_network, not_in_network)"
// @Param        offset query int false "Starting point for pagination. Default: 0"
// @Param        limit query int false "Number of items per page. Default: 100"
// @Success      200 {object} api.CoverageReportPackagesResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /coverage_reports/{uuid}/packages [get]
func (ch *CoverageReportHandler) listCoverageReportPackages(c echo.Context) error {
	req := api.ListCoverageReportPackagesRequest{}
	if err := c.Bind(&req); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	response, err := stubListCoverageReportPackages(c.Param("uuid"), req, ParsePagination(c))
	if errors.Is(err, errStubCoverageReportNotFound) {
		return ce.NewErrorResponse(http.StatusNotFound, "Coverage report not found", "Report is not available or analysis is incomplete")
	}
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error loading fixture", err.Error())
	}

	return c.JSON(http.StatusOK, response)
}
