package dao

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/cache"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type snapshotDaoImpl struct {
	db         *gorm.DB
	pulpClient pulp_client.PulpClient
	cache      cache.Cache
}

func GetSnapshotDao(db *gorm.DB) SnapshotDao {
	return &snapshotDaoImpl{
		db: db,
	}
}

// Create records a snapshot of a repository
func (sDao *snapshotDaoImpl) Create(s *models.Snapshot) error {
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
func (sDao *snapshotDaoImpl) List(ctx context.Context, repoConfigUuid string, paginationData api.PaginationData, _ api.FilterData) (api.SnapshotCollectionResponse, int64, error) {
	var snaps []models.Snapshot
	var totalSnaps int64
	var repoConfig models.RepositoryConfiguration

	// First check if repo config exists
	result := sDao.db.Where("uuid = ?", UuidifyString(repoConfigUuid)).First(&repoConfig)
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
		Where("snapshots.repository_configuration_uuid = ?", UuidifyString(repoConfigUuid))

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

	if len(snaps) == 0 {
		return api.SnapshotCollectionResponse{Data: []api.SnapshotResponse{}}, totalSnaps, nil
	}

	pulpContentPath, err := sDao.getPulpData(ctx)
	if err != nil {
		return api.SnapshotCollectionResponse{}, 0, err
	}

	resp := snapshotConvertToResponses(snaps, pulpContentPath)

	return api.SnapshotCollectionResponse{Data: resp}, totalSnaps, nil
}

func (sDao *snapshotDaoImpl) Fetch(uuid string) (models.Snapshot, error) {
	var snapshot models.Snapshot
	result := sDao.db.Where("uuid = ?", UuidifyString(uuid)).First(&snapshot)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return models.Snapshot{}, &ce.DaoError{
				Message:  "Could not find snapshot with UUID " + uuid,
				NotFound: true,
			}
		}
		return models.Snapshot{}, result.Error
	}
	return snapshot, nil
}

func (sDao *snapshotDaoImpl) GetRepositoryConfigurationFile(ctx context.Context, orgID, snapshotUUID, repoConfigUUID string) (string, error) {
	rcDao := GetRepositoryConfigDao(sDao.db)
	repoConfig, err := rcDao.Fetch(orgID, repoConfigUUID)
	if err != nil {
		return "", err
	}

	snapshot, err := sDao.Fetch(snapshotUUID)
	if err != nil {
		return "", err
	}

	contentURL, err := sDao.getContentURL(ctx, snapshot.RepositoryPath)
	if err != nil {
		return "", err
	}

	repoID := strings.Replace(repoConfig.Name, " ", "_", len(repoConfig.Name))

	fileConfig := fmt.Sprintf(""+
		"[%v]\n"+
		"name=%v\n"+
		"baseurl=%v\n"+
		"gpgcheck=0\n"+
		"repo_gpgcheck=0\n"+
		"enabled=1\n",
		repoID, repoConfig.Name, contentURL)

	return fileConfig, nil
}

func (sDao *snapshotDaoImpl) InitializePulpClient(ctx context.Context, orgID string) error {
	dDao := GetDomainDao(sDao.db)
	domainName, err := dDao.Fetch(orgID)
	if err != nil {
		return err
	}

	pulpClient := pulp_client.GetPulpClientWithDomain(ctx, domainName)
	sDao.pulpClient = pulpClient
	return nil
}

func (sDao *snapshotDaoImpl) FetchForRepoConfigUUID(repoConfigUUID string) ([]models.Snapshot, error) {
	var snaps []models.Snapshot
	result := sDao.db.Model(&models.Snapshot{}).
		Where("repository_configuration_uuid = ?", repoConfigUUID).
		Find(&snaps)
	if result.Error != nil {
		return snaps, result.Error
	}
	return snaps, nil
}

func (sDao *snapshotDaoImpl) Delete(snapUUID string) error {
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

func (sDao *snapshotDaoImpl) FetchLatestSnapshot(repoConfigUUID string) (api.SnapshotResponse, error) {
	var snap models.Snapshot
	result := sDao.db.
		Where("snapshots.repository_configuration_uuid = ?", repoConfigUUID).
		Order("created_at DESC").
		First(&snap)
	if result.Error != nil {
		return api.SnapshotResponse{}, result.Error
	}
	var apiSnap api.SnapshotResponse
	snapshotModelToApi(snap, &apiSnap)
	return apiSnap, nil
}

func (sDao *snapshotDaoImpl) getPulpData(ctx context.Context) (string, error) {
	pulpContentPath, err := sDao.cache.GetPulpContentPath(ctx)
	if err != nil && !errors.Is(err, cache.NotFound) {
		log.Logger.Error().Err(err).Msg("Error reading from cache")
	}

	cacheHit := err == nil
	if cacheHit {
		return pulpContentPath, nil
	}

	if sDao.pulpClient == nil {
		return "", fmt.Errorf("pulpClient cannot be nil")
	}

	status, err := sDao.pulpClient.Status()
	if err != nil {
		return "", err
	}

	pulpContentPath = status.ContentSettings.ContentOrigin + status.ContentSettings.ContentPathPrefix
	err = sDao.cache.SetPulpContentPath(ctx, pulpContentPath)
	if err != nil {
		log.Logger.Error().Err(err).Msg("Error writing to cache")
	}

	return pulpContentPath, nil
}

func (sDao *snapshotDaoImpl) getContentURL(ctx context.Context, repositoryPath string) (string, error) {
	pulpContentPath, err := sDao.getPulpData(ctx)
	if err != nil {
		return "", err
	}

	url := pulpContentPath + repositoryPath
	return url, nil
}

// Converts the database models to our response objects
func snapshotConvertToResponses(snapshots []models.Snapshot, pulpContentPath string) []api.SnapshotResponse {
	snapsAPI := make([]api.SnapshotResponse, len(snapshots))
	for i := 0; i < len(snapshots); i++ {
		snapshotModelToApi(snapshots[i], &snapsAPI[i])
		snapsAPI[i].URL = pulpContentPath + snapshots[i].RepositoryPath
	}
	return snapsAPI
}

func snapshotModelToApi(model models.Snapshot, resp *api.SnapshotResponse) {
	resp.UUID = model.UUID
	resp.CreatedAt = model.CreatedAt
	resp.RepositoryPath = model.RepositoryPath
	resp.ContentCounts = model.ContentCounts
	resp.AddedCounts = model.AddedCounts
	resp.RemovedCounts = model.RemovedCounts
}
