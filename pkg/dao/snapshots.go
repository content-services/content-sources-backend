package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

type snapshotDaoImpl struct {
	db *gorm.DB
}

func GetSnapshotDao(db *gorm.DB) SnapshotDao {
	return snapshotDaoImpl{
		db: db,
	}
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
	var repoConfig models.RepositoryConfiguration

	// First check if repo config exists
	result := sDao.db.Where("text(uuid) = ?", repoConfigUuid).First(&repoConfig)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return api.SnapshotCollectionResponse{}, totalSnaps, &ce.DaoError{
				Message:  "Could not find repository with UUID " + repoConfigUuid,
				NotFound: true,
			}
		}
		return api.SnapshotCollectionResponse{}, totalSnaps, DBErrorToApi(result.Error)
	}

	filteredDB := sDao.db
	result = filteredDB.
		Where("text(snapshots.repository_configuration_uuid) = ?", repoConfigUuid).
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

func (sDao snapshotDaoImpl) FetchForRepoConfigUUID(repoConfigUUID string) ([]models.Snapshot, error) {
	var snaps []models.Snapshot
	result := sDao.db.Model(&models.Snapshot{}).
		Where("repository_configuration_uuid = ?", repoConfigUUID).
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

func (sDao snapshotDaoImpl) FetchLatestSnapshot(repoConfigUUID string) (api.SnapshotResponse, error) {
	var snap models.Snapshot
	result := sDao.db.
		Joins("inner join repository_configurations on repository_configurations.repository_uuid = snapshots.repository_uuid").
		Where("repository_configurations.uuid = ?", repoConfigUUID).
		Where("snapshots.org_id = repository_configurations.org_id").
		Order("created_at DESC").
		First(&snap)
	if result.Error != nil {
		return api.SnapshotResponse{}, result.Error
	}
	var apiSnap api.SnapshotResponse
	snapshotModelToApi(snap, &apiSnap)
	return apiSnap, nil
}
