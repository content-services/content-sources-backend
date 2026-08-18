package dao

import (
	"context"
	"errors"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

type CreateCoverageReportParams struct {
	OrgID     string
	AccountID *string
}

type CreateCoverageUploadParams struct {
	UUID       string
	StorageKey string
	Sha256     string
	SizeBytes  int64
}

type coverageReportDaoImpl struct {
	db *gorm.DB
}

func (d coverageReportDaoImpl) Create(ctx context.Context, reportParams CreateCoverageReportParams, uploadParams CreateCoverageUploadParams) (api.CoverageReportResponse, error) {
	var report models.CoverageReport
	var upload models.CoverageUpload

	d.coverageReportCreateParamsToModels(reportParams, uploadParams, &report, &upload)
	report.Status = config.TaskStatusPending

	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&upload).Error; err != nil {
			return err
		}
		return tx.Create(&report).Error
	})
	if err != nil {
		return api.CoverageReportResponse{}, d.toApiError(err)
	}

	return d.modelToResponse(report), nil
}

func (d coverageReportDaoImpl) Fetch(ctx context.Context, orgID string, uuid string) (api.CoverageReportResponse, error) {
	var report models.CoverageReport
	result := d.db.WithContext(ctx).Where("uuid = ? AND org_id = ?", uuid, orgID).First(&report)
	if result.Error != nil {
		return api.CoverageReportResponse{}, d.toApiError(result.Error)
	}
	return d.modelToResponse(report), nil
}

func (d coverageReportDaoImpl) ListPackages(ctx context.Context, orgID string, reportUUID string, pageData api.PaginationData, filterData api.ListCoverageReportPackagesRequest) (api.CoverageReportPackageCollectionResponse, int64, error) {
	var count int64
	result := d.db.WithContext(ctx).Model(&models.CoverageReport{}).Where("uuid = ? AND org_id = ?", reportUUID, orgID).Count(&count)
	if result.Error != nil {
		return api.CoverageReportPackageCollectionResponse{}, 0, d.toApiError(result.Error)
	}
	if count == 0 {
		return api.CoverageReportPackageCollectionResponse{}, 0, &ce.DaoError{Message: "Coverage report not found", NotFound: true}
	}

	query := d.db.WithContext(ctx).Model(&models.CoverageReportPackage{}).Where("coverage_report_uuid = ?", reportUUID)

	if filterData.Search != "" {
		query = query.Where("name ILIKE ?", "%"+filterData.Search+"%")
	}
	if filterData.Ecosystem != "" {
		query = query.Where("ecosystem = ?", filterData.Ecosystem)
	}
	if filterData.Covered != nil {
		if *filterData.Covered {
			query = query.Where("match_status IN ?", []string{models.CoverageMatchStatusExact, models.CoverageMatchStatusPartial})
		} else {
			query = query.Where("match_status = ?", models.CoverageMatchStatusNone)
		}
	}

	var totalPackages int64
	if err := query.Count(&totalPackages).Error; err != nil {
		return api.CoverageReportPackageCollectionResponse{}, 0, d.toApiError(err)
	}

	var packages []models.CoverageReportPackage
	if err := query.Order("name ASC").Offset(pageData.Offset).Limit(pageData.Limit).Find(&packages).Error; err != nil {
		return api.CoverageReportPackageCollectionResponse{}, 0, d.toApiError(err)
	}

	items := make([]api.CoverageReportPackageResponse, len(packages))
	for i, pkg := range packages {
		items[i] = api.CoverageReportPackageResponse{
			Name:      pkg.Name,
			Version:   pkg.Version,
			Ecosystem: pkg.Ecosystem,
			Covered:   pkg.MatchStatus != models.CoverageMatchStatusNone,
		}
	}

	return api.CoverageReportPackageCollectionResponse{Data: items}, totalPackages, nil
}

func (d coverageReportDaoImpl) coverageReportCreateParamsToModels(report CreateCoverageReportParams, upload CreateCoverageUploadParams, modelReport *models.CoverageReport, modelUpload *models.CoverageUpload) {
	modelReport.OrgID = report.OrgID
	modelReport.AccountID = report.AccountID

	modelUpload.UUID = upload.UUID
	modelUpload.StorageKey = upload.StorageKey
	modelUpload.Sha256 = upload.Sha256
	modelUpload.SizeBytes = upload.SizeBytes
}

func (d coverageReportDaoImpl) modelToResponse(r models.CoverageReport) api.CoverageReportResponse {
	resp := api.CoverageReportResponse{
		UUID:        r.UUID,
		Status:      r.Status,
		CreatedAt:   r.CreatedAt,
		CompletedAt: r.CompletedAt,
	}
	if r.InputFormat != nil {
		resp.InputFormat = *r.InputFormat
	}
	if r.Total != nil {
		resp.Total = *r.Total
	}
	if r.ExactMatches != nil {
		resp.ExactMatches = *r.ExactMatches
	}
	if r.PartialMatches != nil {
		resp.PartialMatches = *r.PartialMatches
	}
	if r.Unmatched != nil {
		resp.Unmatched = *r.Unmatched
	}
	if r.AnalysisTaskError != nil {
		resp.AnalysisTaskError = *r.AnalysisTaskError
	}
	if r.AnalysisTaskUUID != nil {
		resp.AnalysisTaskUUID = *r.AnalysisTaskUUID
	}
	if r.EcosystemCoverageSummary != nil {
		summary := make([]api.EcosystemCoverageSummary, len(*r.EcosystemCoverageSummary))
		for i, s := range *r.EcosystemCoverageSummary {
			summary[i] = api.EcosystemCoverageSummary{
				Ecosystem:      s.Ecosystem,
				Total:          s.Total,
				ExactMatches:   s.ExactMatches,
				PartialMatches: s.PartialMatches,
				Unmatched:      s.Unmatched,
			}
		}
		resp.EcosystemCoverageSummary = summary
	}
	return resp
}

func (d coverageReportDaoImpl) toApiError(e error) *ce.DaoError {
	if e == nil {
		return nil
	}

	dbError, ok := e.(models.Error)
	if ok {
		daoError := ce.DaoError{BadValidation: dbError.Validation, Message: dbError.Message}
		daoError.Wrap(e)
		return &daoError
	}

	daoError := ce.DaoError{}
	if errors.Is(e, gorm.ErrRecordNotFound) {
		daoError = ce.DaoError{
			Message:  "Coverage report not found",
			NotFound: true,
		}
	} else {
		daoError = ce.DaoError{
			Message:  e.Error(),
			NotFound: ce.HttpCodeForDaoError(e) == 404,
		}
	}
	daoError.Wrap(e)
	return &daoError
}
