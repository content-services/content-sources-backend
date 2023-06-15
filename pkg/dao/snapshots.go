package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"gorm.io/gorm"
)

type snapshotDaoImpl struct {
	db *gorm.DB
}

// Create records a snapshot of a repository
func (sDao snapshotDaoImpl) Create(s *Snapshot) error {
	trans := sDao.db.Create(s)
	if trans.Error != nil {
		return trans.Error
	}
	return nil
}

// List the snapshots for a given repository config
func (sDao snapshotDaoImpl) List(repoConfigUuid string, paginationData api.PaginationData, _ api.FilterData) (api.SnapshotCollectionResponse, int64, error) {
	var snaps []Snapshot
	var totalSnaps int64

	filteredDB := sDao.db
	result := filteredDB.
		Joins("inner join repository_configurations on repository_configurations.repository_uuid = snapshots.repository_uuid").
		Where("repository_configurations.uuid = ?", repoConfigUuid).
		Where("snapshots.org_id = repository_configurations.org_id").
		Limit(paginationData.Limit).
		Offset(paginationData.Offset).
		Find(&snaps).Count(&totalSnaps)
	resp := snapshotConvertToResponses(snaps)
	if result.Error != nil {
		return api.SnapshotCollectionResponse{}, 0, result.Error
	}
	return api.SnapshotCollectionResponse{Data: resp}, totalSnaps, nil
}

// Converts the database models to our response objects
func snapshotConvertToResponses(snapshots []Snapshot) []api.SnapshotResponse {
	repos := make([]api.SnapshotResponse, len(snapshots))
	for i := 0; i < len(snapshots); i++ {
		snapshotModelToApi(snapshots[i], &repos[i])
	}
	return repos
}

func snapshotModelToApi(model Snapshot, resp *api.SnapshotResponse) {
	resp.CreatedAt = model.CreatedAt
	resp.DistributionPath = model.DistributionPath
	resp.ContentCounts = model.ContentCounts
}
