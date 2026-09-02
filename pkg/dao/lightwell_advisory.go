package dao

import (
	"context"
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LightwellAdvisoryInput struct {
	RepoName      string
	AdvisoryID    string
	Severity      string
	Details       string
	ReferenceURLs []string
	PackageName   string
	FixedVersions []string
	Checksum      string
}

type LightwellNotificationData struct {
	PackageName   string
	AdvisoryID    string
	Severity      string
	FixedVersions []string
	ReferenceURLs []string
}

type ListLightwellAdvisoriesOptions struct {
	RepoName    *string
	PackageName *string
	SeverityMin string
	CveID       *string
	Limit       int32
	Offset      int32
}

type LightwellAdvisoryCveMatch struct {
	PackageName   string
	FixedVersions []string
	RepoName      string
	Severity      string
}

type lightwellAdvisoryDaoImpl struct {
	db      *gorm.DB
	querier store.Querier
}

func GetLightwellAdvisoryDao(db *gorm.DB) LightwellAdvisoryDao {
	return lightwellAdvisoryDaoImpl{db: db}
}

func (d lightwellAdvisoryDaoImpl) ListByRepository(ctx context.Context, repoConfigUUID string) ([]LightwellAdvisoryInput, error) {
	var advisories []models.LightwellAdvisory
	result := d.db.WithContext(ctx).Where("repository_configuration_uuid = ?", repoConfigUUID).Find(&advisories)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list advisories for repository configuration %s: %w", repoConfigUUID, result.Error)
	}
	return advisoryInputs(advisories), nil
}

func (d lightwellAdvisoryDaoImpl) List(ctx context.Context, offset int, limit int) ([]LightwellAdvisoryInput, int64, error) {
	var total int64
	query := d.db.WithContext(ctx).Model(&models.LightwellAdvisory{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count advisories: %w", err)
	}
	var advisories []models.LightwellAdvisory
	result := query.Order("uuid ASC").Offset(offset).Limit(limit).Find(&advisories)
	if result.Error != nil {
		return nil, 0, fmt.Errorf("failed to list advisories: %w", result.Error)
	}
	return advisoryInputs(advisories), total, nil
}

func advisoryInputs(advisories []models.LightwellAdvisory) []LightwellAdvisoryInput {
	inputs := make([]LightwellAdvisoryInput, len(advisories))
	for i, a := range advisories {
		inputs[i] = LightwellAdvisoryInput{
			RepoName:      a.RepoName,
			AdvisoryID:    a.AdvisoryID,
			Severity:      a.Severity,
			Details:       a.Details,
			ReferenceURLs: a.ReferenceURLs,
			PackageName:   a.PackageName,
			FixedVersions: a.FixedVersions,
			Checksum:      a.Checksum,
		}
	}
	return inputs
}

func (d lightwellAdvisoryDaoImpl) SyncForRepository(ctx context.Context, repoConfigUUID string, repoName string, advisories []LightwellAdvisoryInput) error {
	if len(advisories) == 0 {
		return nil
	}

	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		modelAdvisories := make([]models.LightwellAdvisory, len(advisories))
		advisoryPackagePairs := make([]string, len(advisories))
		args := []interface{}{repoConfigUUID}
		for i, a := range advisories {
			advisoryPackagePairs[i] = "(?, ?)"
			args = append(args, a.AdvisoryID, a.PackageName)
			modelAdvisories[i] = models.LightwellAdvisory{
				RepoName:                    repoName,
				AdvisoryID:                  a.AdvisoryID,
				Severity:                    a.Severity,
				Details:                     a.Details,
				ReferenceURLs:               a.ReferenceURLs,
				PackageName:                 a.PackageName,
				FixedVersions:               a.FixedVersions,
				RepositoryConfigurationUUID: repoConfigUUID,
				Checksum:                    a.Checksum,
			}
		}

		result := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "repository_configuration_uuid"},
				{Name: "advisory_id"},
				{Name: "package_name"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"repo_name", "severity", "details", "reference_urls",
				"fixed_versions", "checksum", "updated_at",
			}),
		}).CreateInBatches(&modelAdvisories, 100)
		if result.Error != nil {
			return fmt.Errorf("failed to upsert advisories: %w", result.Error)
		}

		query := fmt.Sprintf(
			"repository_configuration_uuid = ? AND (advisory_id, package_name) NOT IN (%s)",
			strings.Join(advisoryPackagePairs, ", "),
		)
		result = tx.Where(query, args...).Delete(&models.LightwellAdvisory{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete stale advisories: %w", result.Error)
		}

		return nil
	})
}

func (d lightwellAdvisoryDaoImpl) ListUnnotifiedAdvisories(ctx context.Context, repoConfigUUID string, orgID string) ([]LightwellNotificationData, error) {
	var advisories []models.LightwellAdvisory
	result := d.db.WithContext(ctx).
		Table("lightwell_advisories AS la").
		Select("la.*").
		Joins("LEFT JOIN lightwell_advisory_notifications AS lan ON la.repository_configuration_uuid = lan.repository_configuration_uuid AND la.advisory_id = lan.advisory_id AND la.package_name = lan.package_name AND lan.org_id = ?", orgID).
		Where("la.repository_configuration_uuid = ? AND lan.uuid IS NULL", repoConfigUUID).
		Find(&advisories)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list unnotified advisories for repository configuration %s: %w", repoConfigUUID, result.Error)
	}

	data := make([]LightwellNotificationData, len(advisories))
	for i, a := range advisories {
		data[i] = LightwellNotificationData{
			PackageName:   a.PackageName,
			AdvisoryID:    a.AdvisoryID,
			Severity:      a.Severity,
			FixedVersions: a.FixedVersions,
			ReferenceURLs: a.ReferenceURLs,
		}
	}
	return data, nil
}

func (d lightwellAdvisoryDaoImpl) MarkAsNotified(ctx context.Context, repoConfigUUID string, orgID string, data []LightwellNotificationData) error {
	if len(data) == 0 {
		return nil
	}

	notifications := make([]models.LightwellAdvisoryNotification, 0, len(data))
	for _, d := range data {
		notifications = append(notifications, models.LightwellAdvisoryNotification{
			RepositoryConfigurationUUID: repoConfigUUID,
			AdvisoryID:                  d.AdvisoryID,
			PackageName:                 d.PackageName,
			OrgID:                       orgID,
		})
	}

	result := d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "repository_configuration_uuid"},
			{Name: "advisory_id"},
			{Name: "package_name"},
			{Name: "org_id"},
		},
		DoNothing: true,
	}).CreateInBatches(&notifications, 100)
	if result.Error != nil {
		return fmt.Errorf("failed to mark advisories as notified: %w", result.Error)
	}
	return nil
}

var severityMap = map[string]int16{
	"low":       1,
	"moderate":  2,
	"important": 3,
	"critical":  4,
}

func parseSeverityMin(s string) (pgtype.Int2, error) {
	if s == "" {
		return pgtype.Int2{}, nil
	}
	val, ok := severityMap[s]
	if !ok {
		return pgtype.Int2{}, fmt.Errorf("invalid severity: %s (must be one of: low, moderate, important, critical)", s)
	}
	return pgtype.Int2{Int16: val, Valid: true}, nil
}

func (d lightwellAdvisoryDaoImpl) ListAdvisories(ctx context.Context, opts ListLightwellAdvisoriesOptions) ([]api.LightwellAdvisoryResponse, int64, error) {
	severityMin, err := parseSeverityMin(opts.SeverityMin)
	if err != nil {
		return nil, 0, err
	}

	rows, err := d.querier.ListAdvisories(ctx, store.ListAdvisoriesParams{
		RepoName:    opts.RepoName,
		PackageName: opts.PackageName,
		SeverityMin: severityMin,
		CveID:       opts.CveID,
		PageOffset:  opts.Offset,
		PageLimit:   opts.Limit,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list advisories: %w", err)
	}

	var totalCount int64
	if len(rows) > 0 {
		totalCount = rows[0].TotalCount
	}

	data := make([]api.LightwellAdvisoryResponse, 0, len(rows))
	for _, row := range rows {
		refURLs := row.ReferenceUrls
		if refURLs == nil {
			refURLs = []string{}
		}
		fixedVersions := row.FixedVersions
		if fixedVersions == nil {
			fixedVersions = []string{}
		}
		data = append(data, api.LightwellAdvisoryResponse{
			AdvisoryID:    row.AdvisoryID,
			Severity:      row.Severity,
			Details:       row.Details,
			ReferenceURLs: refURLs,
			PackageName:   row.PackageName,
			FixedVersions: fixedVersions,
			Repository:    row.RepoName,
		})
	}
	return data, totalCount, nil
}

func (d lightwellAdvisoryDaoImpl) ListAdvisoriesByCveID(ctx context.Context, cveID string) ([]LightwellAdvisoryCveMatch, error) {
	rows, err := d.querier.ListAdvisoriesByCveID(ctx, cveID)
	if err != nil {
		return nil, fmt.Errorf("failed to list advisories by CVE ID: %w", err)
	}
	matches := make([]LightwellAdvisoryCveMatch, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, LightwellAdvisoryCveMatch{
			PackageName:   row.PackageName,
			FixedVersions: row.FixedVersions,
			RepoName:      row.RepoName,
			Severity:      row.Severity,
		})
	}
	return matches, nil
}

func (d lightwellAdvisoryDaoImpl) CountAdvisoriesByRepo(ctx context.Context, repoConfigUUID uuid.UUID) (int64, error) {
	count, err := d.querier.CountAdvisoriesByRepo(ctx, repoConfigUUID)
	if err != nil {
		return 0, fmt.Errorf("failed to count advisories for repo %s: %w", repoConfigUUID, err)
	}
	return count, nil
}
