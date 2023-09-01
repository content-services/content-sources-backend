package dao

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
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

	updateResult := trans.
		Exec(`
			UPDATE repository_configurations 
			SET last_snapshot_uuid = ? 
			WHERE repository_configurations.uuid = ?`,
			s.UUID,
			s.RepositoryConfigurationUUID,
		)

	if updateResult.Error != nil {
		fmt.Printf("%v", updateResult.Error.Error())
		return updateResult.Error
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
	sortMap := map[string]string{
		"created_at": "created_at",
	}

	order := convertSortByToSQL(paginationData.SortBy, sortMap, "created_at asc")

	filteredDB := sDao.db.
		Where("text(snapshots.repository_configuration_uuid) = ?", repoConfigUuid)

	// Get count
	filteredDB.
		Model(&snaps).
		Count(&totalSnaps)

	if filteredDB.Error != nil {
		return api.SnapshotCollectionResponse{}, 0, filteredDB.Error
	}

	// Get Data
	filteredDB.Order(order).
		Limit(paginationData.Limit).
		Offset(paginationData.Offset).
		Find(&snaps)

	if filteredDB.Error != nil {
		return api.SnapshotCollectionResponse{}, 0, filteredDB.Error
	}

	resp := snapshotConvertToResponses(snaps)

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
	resp.AddedCounts = model.AddedCounts
	resp.RemovedCounts = model.RemovedCounts
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
