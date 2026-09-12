package dao

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
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

// readableSnapshotOrgFilterSQL selects snapshots the viewer may read:
// owned / RH / community repos, or published snapshots of foreign partner repos.
// Args: orgIDs []string (viewer + shared orgs), viewerOrgID string.
// Requires the snapshots table to be aliased as "s".
const readableSnapshotOrgFilterSQL = `
	(
		repository_configurations.org_id IN ?
		OR (
			repository_configurations.partner = true
			AND repository_configurations.org_id != ?
			AND s.published = true
		)
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

// snapshotsPublishingSQL selects repository_configuration UUIDs that have at least one
// snapshot with published=true and a pending/running task (publish in progress).
const snapshotsPublishingSQL = `
	SELECT DISTINCT s.repository_configuration_uuid AS uuid
	FROM snapshots s
	JOIN tasks t ON s.publish_task_uuid = t.id
	WHERE s.repository_configuration_uuid IN ?
	  AND s.deleted_at IS NULL
	  AND s.published = true
	  AND t.status IN ('pending', 'running')`

// snapshotsUnpublishingSQL selects repository_configuration UUIDs that have at least one
// snapshot with published=false and a pending/running task (unpublish in progress).
const snapshotsUnpublishingSQL = `
	SELECT DISTINCT s.repository_configuration_uuid AS uuid
	FROM snapshots s
	JOIN tasks t ON s.publish_task_uuid = t.id
	WHERE s.repository_configuration_uuid IN ?
	  AND s.deleted_at IS NULL
	  AND s.published = false
	  AND t.status IN ('pending', 'running')`

// snapshotsPublishedSQL selects repository_configuration UUIDs that have at least one
// published snapshot with a completed publish task.
const snapshotsPublishedSQL = `
	SELECT DISTINCT s.repository_configuration_uuid AS uuid
	FROM snapshots s
	JOIN tasks t ON s.publish_task_uuid = t.id
	WHERE s.repository_configuration_uuid IN ?
	  AND s.deleted_at IS NULL
	  AND s.published = true
	  AND t.status = 'completed'`

// snapshotsPublishStoppedSQL selects repository_configuration UUIDs that have at least one
// snapshot with a failed or canceled publish task (covers both publish and unpublish failures).
const snapshotsPublishStoppedSQL = `
	SELECT DISTINCT s.repository_configuration_uuid AS uuid
	FROM snapshots s
	JOIN tasks t ON s.publish_task_uuid = t.id
	WHERE s.repository_configuration_uuid IN ?
	  AND s.deleted_at IS NULL
	  AND t.status IN ('failed', 'canceled')`

// computeSnapshotPublishStates returns the aggregate publish state for each partner repo config UUID.
// Each state flag is independent — a repo can be simultaneously published and publishing.
func computeSnapshotPublishStates(ctx context.Context, db *gorm.DB, repoConfigUUIDs []string) (map[string]*api.SnapshotPublishState, error) {
	if len(repoConfigUUIDs) == 0 {
		return map[string]*api.SnapshotPublishState{}, nil
	}

	result := make(map[string]*api.SnapshotPublishState, len(repoConfigUUIDs))
	for _, uuid := range repoConfigUUIDs {
		result[uuid] = &api.SnapshotPublishState{}
	}

	type uuidRow struct{ UUID string }

	var publishingUUIDs []uuidRow
	err := db.WithContext(ctx).
		Raw(snapshotsPublishingSQL, repoConfigUUIDs).
		Scan(&publishingUUIDs).Error
	if err != nil {
		return nil, fmt.Errorf("error querying publishing states: %w", err)
	}
	for _, row := range publishingUUIDs {
		if ps, ok := result[row.UUID]; ok {
			ps.Publishing = true
		}
	}

	var unpublishingUUIDs []uuidRow
	err = db.WithContext(ctx).
		Raw(snapshotsUnpublishingSQL, repoConfigUUIDs).
		Scan(&unpublishingUUIDs).Error
	if err != nil {
		return nil, fmt.Errorf("error querying unpublishing states: %w", err)
	}
	for _, row := range unpublishingUUIDs {
		if ps, ok := result[row.UUID]; ok {
			ps.Unpublishing = true
		}
	}

	var publishedUUIDs []uuidRow
	err = db.WithContext(ctx).
		Raw(snapshotsPublishedSQL, repoConfigUUIDs).
		Scan(&publishedUUIDs).Error
	if err != nil {
		return nil, fmt.Errorf("error querying published states: %w", err)
	}
	for _, row := range publishedUUIDs {
		if ps, ok := result[row.UUID]; ok {
			ps.Published = true
		}
	}

	var stoppedUUIDs []uuidRow
	err = db.WithContext(ctx).
		Raw(snapshotsPublishStoppedSQL, repoConfigUUIDs).
		Scan(&stoppedUUIDs).Error
	if err != nil {
		return nil, fmt.Errorf("error querying stopped publish states: %w", err)
	}
	for _, row := range stoppedUUIDs {
		if ps, ok := result[row.UUID]; ok {
			ps.Stopped = true
		}
	}

	return result, nil
}
