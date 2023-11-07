package dao

import (
	"context"
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"gorm.io/gorm"
)

type snapshotDaoImpl struct {
	db         *gorm.DB
	pulpClient pulp_client.PulpClient
	ctx        context.Context
}

func GetSnapshotDao(db *gorm.DB) SnapshotDao {
	return &snapshotDaoImpl{
		db:  db,
		ctx: context.Background(),
	}
}

func (sDao *snapshotDaoImpl) WithContext(ctx context.Context) SnapshotDao {
	cpy := *sDao
	cpy.ctx = ctx
	return &cpy
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
func (sDao *snapshotDaoImpl) List(
	orgID string,
	repoConfigUUID string,
	paginationData api.PaginationData,
	_ api.FilterData,
) (api.SnapshotCollectionResponse, int64, error) {
	var snaps []models.Snapshot
	var totalSnaps int64
	var repoConfig models.RepositoryConfiguration

	// First check if repo config exists
	result := sDao.db.Where(
		"repository_configurations.org_id IN (?,?) AND uuid = ?",
		orgID,
		config.RedHatOrg,
		UuidifyString(repoConfigUUID)).
		First(&repoConfig)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return api.SnapshotCollectionResponse{}, totalSnaps, &ce.DaoError{
				Message:  "Could not find repository with UUID " + repoConfigUUID,
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
		Model(&models.Snapshot{}).
		Joins("JOIN repository_configurations ON repository_configuration_uuid = repository_configurations.uuid").
		Where("repository_configuration_uuid = ?", UuidifyString(repoConfigUUID)).
		Where("repository_configurations.org_id IN (?,?)", orgID, config.RedHatOrg)

	// Get count
	filteredDB.Count(&totalSnaps)

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

	pulpContentPath, err := sDao.pulpClient.WithContext(sDao.ctx).GetContentPath()
	if err != nil {
		return api.SnapshotCollectionResponse{}, 0, err
	}

	resp := snapshotConvertToResponses(snaps, pulpContentPath)

	return api.SnapshotCollectionResponse{Data: resp}, totalSnaps, nil
}

func (sDao *snapshotDaoImpl) Fetch(uuid string) (api.SnapshotResponse, error) {
	var snapAPI api.SnapshotResponse
	snapModel, err := sDao.fetch(uuid)
	if err != nil {
		return api.SnapshotResponse{}, err
	}
	snapshotModelToApi(snapModel, &snapAPI)
	return snapAPI, nil
}

func (sDao *snapshotDaoImpl) fetch(uuid string) (models.Snapshot, error) {
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

func (sDao *snapshotDaoImpl) GetRepositoryConfigurationFile(orgID, snapshotUUID, repoConfigUUID, host string) (string, error) {
	rcDao := repositoryConfigDaoImpl{db: sDao.db}
	repoConfig, err := rcDao.fetchRepoConfig(orgID, repoConfigUUID, true)
	if err != nil {
		return "", err
	}

	snapshot, err := sDao.fetch(snapshotUUID)
	if err != nil {
		return "", err
	}

	pc := sDao.pulpClient.WithContext(sDao.ctx)
	contentPath, err := pc.GetContentPath()
	if err != nil {
		return "", err
	}

	contentURL := pulpContentURL(contentPath, snapshot.RepositoryPath)

	repoID := strings.Replace(repoConfig.Name, " ", "_", len(repoConfig.Name))

	var gpgCheck, repoGpgCheck int
	var gpgKeyField string
	if repoConfig.GpgKey != "" {
		gpgCheck = 1
		gpgKeyField = fmt.Sprintf("http://%v%v/repositories/%v/gpg_key/", host, api.FullRootPath(), repoConfigUUID) // host includes trailing slash
	}
	if repoConfig.MetadataVerification {
		repoGpgCheck = 1
	}

	fileConfig := fmt.Sprintf(""+
		"[%v]\n"+
		"name=%v\n"+
		"baseurl=%v\n"+
		"gpgcheck=%v\n"+ // set to verify packages
		"repo_gpgcheck=%v\n"+ // set to verify metadata
		"enabled=1\n"+
		"gpgkey=%v\n",
		repoID, repoConfig.Name, contentURL, gpgCheck, repoGpgCheck, gpgKeyField)

	return fileConfig, nil
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

// Converts the database models to our response objects
func snapshotConvertToResponses(snapshots []models.Snapshot, pulpContentPath string) []api.SnapshotResponse {
	snapsAPI := make([]api.SnapshotResponse, len(snapshots))
	for i := 0; i < len(snapshots); i++ {
		snapshotModelToApi(snapshots[i], &snapsAPI[i])
		snapsAPI[i].URL = pulpContentURL(pulpContentPath, snapshots[i].RepositoryPath)
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

// pulpContentURL combines content path and repository path to get content URL
func pulpContentURL(pulpContentPath string, repositoryPath string) string {
	return pulpContentPath + repositoryPath + "/"
}
