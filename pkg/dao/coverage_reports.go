package dao

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/coverage/matcher"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/utils"
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

type SaveCoverageAnalysisParams struct {
	InputFormat string
	Results     []matcher.MatchResult
	Summary     matcher.MatchSummary
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
		ecosystems := strings.Split(filterData.Ecosystem, ",")
		query = query.Where("ecosystem IN ?", ecosystems)
	}
	if filterData.MatchStatus != "" {
		matchStatuses := strings.Split(filterData.MatchStatus, ",")
		query = query.Where("match_status IN ?", matchStatuses)
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
			Name:        pkg.Name,
			Version:     pkg.Version,
			Ecosystem:   pkg.Ecosystem,
			Covered:     pkg.MatchStatus != models.CoverageMatchStatusNone,
			MatchStatus: pkg.MatchStatus,
		}
	}

	return api.CoverageReportPackageCollectionResponse{Data: items}, totalPackages, nil
}

func (d coverageReportDaoImpl) InternalOnlyFetchCoverageUpload(ctx context.Context, uploadUUID string) (models.CoverageUpload, error) {
	var upload models.CoverageUpload
	result := d.db.WithContext(ctx).Where("uuid = ?", uploadUUID).First(&upload)
	if result.Error != nil {
		return models.CoverageUpload{}, d.toApiError(result.Error)
	}
	return upload, nil
}

func (d coverageReportDaoImpl) SetAnalysisTaskUUID(ctx context.Context, reportUUID string, taskUUID string) error {
	result := d.db.WithContext(ctx).
		Model(&models.CoverageReport{}).
		Where("uuid = ?", reportUUID).
		Update("analysis_task_uuid", taskUUID)
	if result.Error != nil {
		return d.toApiError(result.Error)
	}
	if result.RowsAffected == 0 {
		return d.toApiError(gorm.ErrRecordNotFound)
	}
	return nil
}

func (d coverageReportDaoImpl) UpdateCoverageReportStatus(ctx context.Context, reportUUID string, status string, errMsg *string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if status == config.TaskStatusFailed && errMsg != nil {
		updates["analysis_task_error"] = *errMsg
		updates["completed_at"] = time.Now().UTC()
	}

	result := d.db.WithContext(ctx).Model(&models.CoverageReport{}).Where("uuid = ?", reportUUID).Updates(updates)
	if result.Error != nil {
		return d.toApiError(result.Error)
	}
	if result.RowsAffected == 0 {
		return d.toApiError(gorm.ErrRecordNotFound)
	}
	return nil
}

func (d coverageReportDaoImpl) SaveCoverageAnalysis(ctx context.Context, reportUUID string, params SaveCoverageAnalysisParams) error {
	packages := make([]models.CoverageReportPackage, 0, len(params.Results))
	demandSignals := make([]models.CoverageDemandSignal, 0)
	for _, result := range params.Results {
		var namespace *string
		if result.Namespace != "" {
			namespace = utils.Ptr(result.Namespace)
		}
		packages = append(packages, models.CoverageReportPackage{
			CoverageReportUUID: reportUUID,
			Ecosystem:          result.Ecosystem,
			Name:               result.Name,
			Version:            result.Version,
			Namespace:          namespace,
			MatchStatus:        result.MatchStatus,
		})
		if result.MatchStatus == matcher.MatchStatusNone || result.MatchStatus == matcher.MatchStatusPartial {
			demandSignals = append(demandSignals, models.CoverageDemandSignal{
				Ecosystem:   result.Ecosystem,
				Name:        result.Name,
				Version:     result.Version,
				Namespace:   namespace,
				MatchStatus: result.MatchStatus,
				Source:      models.CoverageDemandSourceProspectDriven,
			})
		}
	}

	ecosystemSummary := make(models.EcosystemCoverageSummary, 0, len(params.Summary.EcosystemCoverageSummary))
	for _, entry := range params.Summary.EcosystemCoverageSummary {
		ecosystemSummary = append(ecosystemSummary, models.EcosystemCoverageSummaryEntry{
			Ecosystem:      entry.Ecosystem,
			Total:          entry.Total,
			ExactMatches:   entry.ExactMatches,
			PartialMatches: entry.PartialMatches,
			Unmatched:      entry.Unmatched,
		})
	}

	now := time.Now().UTC()
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var report models.CoverageReport
		if err := tx.Where("uuid = ?", reportUUID).First(&report).Error; err != nil {
			return err
		}

		report.Status = config.TaskStatusCompleted
		report.InputFormat = utils.Ptr(params.InputFormat)
		report.Total = utils.Ptr(params.Summary.Total)
		report.ExactMatches = utils.Ptr(params.Summary.ExactMatches)
		report.PartialMatches = utils.Ptr(params.Summary.PartialMatches)
		report.Unmatched = utils.Ptr(params.Summary.Unmatched)
		report.EcosystemCoverageSummary = &ecosystemSummary
		report.CatalogSnapshotAt = utils.Ptr(params.Summary.CatalogSnapshotAt)
		report.CompletedAt = &now

		if err := tx.Model(&report).Updates(report.MapForUpdate()).Error; err != nil {
			return err
		}
		if len(packages) > 0 {
			if err := tx.Create(&packages).Error; err != nil {
				return err
			}
		}
		if len(demandSignals) > 0 {
			if err := tx.Create(&demandSignals).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return d.toApiError(err)
	}
	return nil
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
