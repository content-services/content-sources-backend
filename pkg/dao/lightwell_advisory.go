package dao

import (
	"context"
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LightwellAdvisoryInput struct {
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

type lightwellAdvisoryDaoImpl struct {
	db *gorm.DB
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
	inputs := make([]LightwellAdvisoryInput, len(advisories))
	for i, a := range advisories {
		inputs[i] = LightwellAdvisoryInput{
			AdvisoryID:    a.AdvisoryID,
			Severity:      a.Severity,
			Details:       a.Details,
			ReferenceURLs: a.ReferenceURLs,
			PackageName:   a.PackageName,
			FixedVersions: a.FixedVersions,
			Checksum:      a.Checksum,
		}
	}
	return inputs, nil
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

func (d lightwellAdvisoryDaoImpl) ListUnnotifiedAdvisories(ctx context.Context, repoConfigUUID string) ([]LightwellNotificationData, error) {
	var advisories []models.LightwellAdvisory
	result := d.db.WithContext(ctx).
		Table("lightwell_advisories AS la").
		Select("la.*").
		Joins("LEFT JOIN lightwell_advisory_notifications AS lan ON la.repository_configuration_uuid = lan.repository_configuration_uuid AND la.advisory_id = lan.advisory_id AND la.package_name = lan.package_name").
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

func (d lightwellAdvisoryDaoImpl) MarkAsNotified(ctx context.Context, repoConfigUUID string, data []LightwellNotificationData) error {
	if len(data) == 0 {
		return nil
	}

	notifications := make([]models.LightwellAdvisoryNotification, 0, len(data))
	for _, d := range data {
		notifications = append(notifications, models.LightwellAdvisoryNotification{
			RepositoryConfigurationUUID: repoConfigUUID,
			AdvisoryID:                  d.AdvisoryID,
			PackageName:                 d.PackageName,
		})
	}

	result := d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "repository_configuration_uuid"},
			{Name: "advisory_id"},
			{Name: "package_name"},
		},
		DoNothing: true,
	}).CreateInBatches(&notifications, 100)
	if result.Error != nil {
		return fmt.Errorf("failed to mark advisories as notified: %w", result.Error)
	}
	return nil
}
