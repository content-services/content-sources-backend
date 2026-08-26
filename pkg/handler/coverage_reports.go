package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/s3_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

const maxCoverageUploadSizeBytes = 15 * 1024 * 1024 // 15 MiB

type CoverageReportHandler struct {
	DaoRegistry dao.DaoRegistry
	S3          s3_client.S3Client
}

func checkLightwellLensAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := CheckLightwellLensAccessible(c.Request().Context()); err != nil {
			return err
		}
		return next(c)
	}
}

func RegisterCoverageReportRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, s3Client s3_client.S3Client) {
	ch := CoverageReportHandler{
		DaoRegistry: *daoReg,
		S3:          s3Client,
	}
	addRepoRoute(engine, http.MethodPost, "/coverage_reports/", ch.createCoverageReport, rbac.RbacVerbWrite, checkLightwellLensAccessible)
	addRepoRoute(engine, http.MethodGet, "/coverage_reports/:uuid", ch.getCoverageReport, rbac.RbacVerbRead, checkLightwellLensAccessible)
	addRepoRoute(engine, http.MethodGet, "/coverage_reports/:uuid/packages", ch.listCoverageReportPackages, rbac.RbacVerbRead, checkLightwellLensAccessible)
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
	accountID, orgID := getAccountIdOrgId(c)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", "File is required")
	}
	if fileHeader.Size <= 0 {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error reading upload", "Size must be greater than 0")
	}
	if fileHeader.Size > maxCoverageUploadSizeBytes {
		return ce.NewErrorResponse(http.StatusRequestEntityTooLarge, "Error reading upload", "File exceeds maximum upload size")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error opening upload", err.Error())
	}
	defer file.Close()

	uploadUUID := uuid.NewString()
	storageKey := "coverage-uploads/" + uploadUUID

	hash := sha256.New()
	fileBytes, err := io.ReadAll(io.TeeReader(file, hash))
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error reading upload", err.Error())
	}

	if config.FeatureAccessible(c.Request().Context(), config.Get().Features.LightwellStoreUploads) {
		if ch.S3 == nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error uploading coverage report", "s3 not configured")
		}
		if err := ch.S3.Put(c.Request().Context(), storageKey, bytes.NewReader(fileBytes)); err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error uploading coverage report", err.Error())
		}
	}

	sha256Hex := hex.EncodeToString(hash.Sum(nil))
	sizeBytes := int64(len(fileBytes))

	reportParams := dao.CreateCoverageReportParams{OrgID: orgID}
	if accountID != "" {
		reportParams.AccountID = utils.Ptr(accountID)
	}
	uploadParams := dao.CreateCoverageUploadParams{
		UUID:       uploadUUID,
		StorageKey: storageKey,
		Sha256:     sha256Hex,
		SizeBytes:  sizeBytes,
	}

	report, err := ch.DaoRegistry.CoverageReport.Create(c.Request().Context(), reportParams, uploadParams)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error creating coverage report", err.Error())
	}

	// TODO: enqueue task, set task UUID

	// TODO: remove once we don't need seeded data
	if config.Get().Options.SeedLightwellCoverageReports {
		go func(reportUUID string) {
			time.Sleep(3 * time.Second)
			if _, err := seeds.SeedCoverageReport(db.DB, seeds.CoverageReportSeedOptions{UUID: reportUUID}); err != nil {
				log.Error().Err(err).Str("uuid", reportUUID).Msg("failed to seed coverage report")
			}
		}(report.UUID)
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
