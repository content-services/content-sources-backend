package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

type snapshotDaoImpl struct {
	db *gorm.DB
}

// Create records a snapshot of a repository
func (sDao snapshotDaoImpl) Create(s *models.Snapshot) error {
	trans := sDao.db.Create(s)
	if trans.Error != nil {
		return trans.Error
	}
	return nil
}

// List the snapshots for a given repository config
func (sDao snapshotDaoImpl) List(repoConfigUuid string, paginationData api.PaginationData, _ api.FilterData) (api.SnapshotCollectionResponse, int64, error) {
	var snaps []models.Snapshot
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
func snapshotConvertToResponses(snapshots []models.Snapshot) []api.SnapshotResponse {
	repos := make([]api.SnapshotResponse, len(snapshots))
	for i := 0; i < len(snapshots); i++ {
		snapshotModelToApi(snapshots[i], &repos[i])
	}
	return repos
}

func snapshotModelToApi(model models.Snapshot, resp *api.SnapshotResponse) {
	resp.CreatedAt = model.CreatedAt
	resp.RepositoryPath = model.RepositoryPath
	resp.ContentCounts = model.ContentCounts
}

func (sDao snapshotDaoImpl) FetchForRepoUUID(orgID string, repoUUID string) ([]models.Snapshot, error) {
	var snaps []models.Snapshot
	result := sDao.db.Model(&models.Snapshot{}).
		Where("repository_uuid = ?", repoUUID).
		Where("org_id = ?", orgID).
		Find(&snaps)
	if result.Error != nil {
		return snaps, result.Error
	}
	return snaps, nil
}

func (sDao snapshotDaoImpl) Delete(snapUUID string) error {
	var snap models.Snapshot
	result := sDao.db.Where("uuid = ?", snapUUID).First(&snap)
	if result.Error != nil {
		return result.Error
	}
	result = sDao.db.Delete(snap)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
