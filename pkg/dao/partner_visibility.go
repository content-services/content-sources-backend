package dao

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

const foreignPartnerVisibleSQL = `
	repository_configurations.partner = true
	AND repository_configurations.org_id != ?
	AND EXISTS (
		SELECT 1 FROM snapshots
		WHERE snapshots.repository_configuration_uuid = repository_configurations.uuid
		AND snapshots.published = true
		AND snapshots.deleted_at IS NULL
	)`

// IsForeignPartnerView reports whether viewerOrgID is accessing a partner repository it does not own.
func IsForeignPartnerView(repoConfig models.RepositoryConfiguration, viewerOrgID string) bool {
	return repoConfig.Partner && repoConfig.OrgID != viewerOrgID
}

// ForeignPartnerVisibleExpr returns a GORM scope for foreign partner repositories with at least one published snapshot.
func ForeignPartnerVisibleExpr(viewerOrgID string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(foreignPartnerVisibleSQL, viewerOrgID)
	}
}

// HasPublishedSnapshot reports whether a repository configuration has at least one published, non-deleted snapshot.
func HasPublishedSnapshot(ctx context.Context, db *gorm.DB, repoConfigUUID string) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&models.Snapshot{}).
		Where("repository_configuration_uuid = ?", repoConfigUUID).
		Where("published = ?", true).
		Where("deleted_at IS NULL").
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
