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
	JOIN tasks t ON s.last_publish_task_uuid = t.id
	WHERE s.repository_configuration_uuid IN ?
	  AND s.deleted_at IS NULL
	  AND s.published = true
	  AND t.status IN ('pending', 'running')`

// snapshotsUnpublishingSQL selects repository_configuration UUIDs that have at least one
// snapshot with published=false and a pending/running task (unpublish in progress).
const snapshotsUnpublishingSQL = `
	SELECT DISTINCT s.repository_configuration_uuid AS uuid
	FROM snapshots s
	JOIN tasks t ON s.last_publish_task_uuid = t.id
	WHERE s.repository_configuration_uuid IN ?
	  AND s.deleted_at IS NULL
	  AND s.published = false
	  AND t.status IN ('pending', 'running')`

// snapshotsPublishedSQL selects repository_configuration UUIDs that have at least one
// published snapshot with a completed publish task.
const snapshotsPublishedSQL = `
	SELECT DISTINCT s.repository_configuration_uuid AS uuid
	FROM snapshots s
	JOIN tasks t ON s.last_publish_task_uuid = t.id
	WHERE s.repository_configuration_uuid IN ?
	  AND s.deleted_at IS NULL
	  AND s.published = true
	  AND t.status = 'completed'`

// snapshotsPublishStoppedSQL selects repository_configuration UUIDs that have at least one
// snapshot with a failed or canceled publish task (covers both publish and unpublish failures).
const snapshotsPublishStoppedSQL = `
	SELECT DISTINCT s.repository_configuration_uuid AS uuid
	FROM snapshots s
	JOIN tasks t ON s.last_publish_task_uuid = t.id
	WHERE s.repository_configuration_uuid IN ?
	  AND s.deleted_at IS NULL
	  AND t.status IN ('failed', 'canceled')`

// snapshotPublishStatesSQL unions the four independent state queries above into a
// single round trip. Each branch stays self-contained — deleting a state means
// removing its UNION ALL branch here, its const above, and its case in the switch
// in computeSnapshotPublishStates.
const snapshotPublishStatesSQL = `
	SELECT uuid, 'publishing' AS state FROM (` + snapshotsPublishingSQL + `) p
	UNION ALL
	SELECT uuid, 'unpublishing' AS state FROM (` + snapshotsUnpublishingSQL + `) u
	UNION ALL
	SELECT uuid, 'published' AS state FROM (` + snapshotsPublishedSQL + `) pb
	UNION ALL
	SELECT uuid, 'stopped' AS state FROM (` + snapshotsPublishStoppedSQL + `) st`

// publishStateRow is a single (repoConfigUUID, state) pair returned by snapshotPublishStatesSQL.
type publishStateRow struct {
	UUID  string
	State string
}

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

	var rows []publishStateRow
	err := db.WithContext(ctx).
		Raw(snapshotPublishStatesSQL, repoConfigUUIDs, repoConfigUUIDs, repoConfigUUIDs, repoConfigUUIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("error querying snapshot publish states: %w", err)
	}

	for _, row := range rows {
		ps, ok := result[row.UUID]
		if !ok {
			continue
		}
		switch row.State {
		case "publishing":
			ps.Publishing = true
		case "unpublishing":
			ps.Unpublishing = true
		case "published":
			ps.Published = true
		case "stopped":
			ps.Stopped = true
		}
	}

	return result, nil
}
