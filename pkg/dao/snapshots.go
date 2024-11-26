package dao

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
)

type snapshotDaoImpl struct {
	db         *gorm.DB
	pulpClient pulp_client.PulpClient
}

func GetSnapshotDao(db *gorm.DB) SnapshotDao {
	return &snapshotDaoImpl{
		db: db,
	}
}

// Create records a snapshot of a repository
func (sDao *snapshotDaoImpl) Create(ctx context.Context, s *models.Snapshot) error {
	trans := sDao.db.WithContext(ctx).Create(s)
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
	ctx context.Context,
	orgID string,
	repoConfigUUID string,
	paginationData api.PaginationData,
	_ api.FilterData,
) (api.SnapshotCollectionResponse, int64, error) {
	var snaps []models.Snapshot
	var totalSnaps int64
	var repoConfig models.RepositoryConfiguration

	// First check if repo config exists
	result := sDao.db.WithContext(ctx).Where(
		"repository_configurations.org_id IN (?,?) AND uuid = ?",
		orgID,
		config.RedHatOrg,
		UuidifyString(repoConfigUUID)).
		First(&repoConfig)

	if result.Error != nil {
		return api.SnapshotCollectionResponse{}, totalSnaps, RepositoryDBErrorToApi(result.Error, &repoConfigUUID)
	}
	sortMap := map[string]string{
		"created_at": "created_at",
	}

	order := convertSortByToSQL(paginationData.SortBy, sortMap, "created_at desc")

	filteredDB := readableSnapshots(sDao.db.WithContext(ctx), orgID).
		Where("repository_configuration_uuid = ?", UuidifyString(repoConfigUUID))

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

	pulpContentPath, err := sDao.pulpClient.GetContentPath(ctx)
	if err != nil {
		return api.SnapshotCollectionResponse{}, 0, err
	}

	resp := snapshotConvertToResponses(snaps, pulpContentPath)

	return api.SnapshotCollectionResponse{Data: resp}, totalSnaps, nil
}

func (sDao *snapshotDaoImpl) ListByTemplate(
	ctx context.Context,
	orgID string,
	template api.TemplateResponse,
	repositorySearch string,
	paginationData api.PaginationData,
) (api.SnapshotCollectionResponse, int64, error) {
	var snaps []api.SnapshotResponse
	var totalSnaps int64
	pulpContentPath, err := sDao.pulpClient.GetContentPath(ctx)
	if err != nil {
		return api.SnapshotCollectionResponse{}, 0, err
	}

	// Repository search/filter and ordering
	sortMap := map[string]string{
		"repository_name": "repo_name",
		"created_at":      "snapshots.created_at",
	}
	order := convertSortByToSQL(paginationData.SortBy, sortMap, "repo_name ASC")

	baseQuery := readableSnapshots(sDao.db.WithContext(ctx), orgID).
		Joins("JOIN templates_repository_configurations ON templates_repository_configurations.snapshot_uuid = snapshots.uuid").
		Where("templates_repository_configurations.template_uuid = ?", template.UUID).
		Where("repository_configurations.name ILIKE ?", fmt.Sprintf("%%%s%%", repositorySearch))

	countQuery := baseQuery.
		Count(&totalSnaps)
	if countQuery.Error != nil {
		return api.SnapshotCollectionResponse{}, totalSnaps, countQuery.Error
	}

	var filteredSnaps []models.Snapshot
	listQuery := baseQuery.
		Select("snapshots.*, STRING_AGG(repository_configurations.name, '') as repo_name").
		Group("snapshots.uuid").
		Limit(paginationData.Limit).
		Offset(paginationData.Offset).
		Order(order).
		Find(&filteredSnaps)
	if listQuery.Error != nil {
		return api.SnapshotCollectionResponse{}, totalSnaps, listQuery.Error
	}

	snaps = snapshotConvertToResponses(filteredSnaps, pulpContentPath)
	if totalSnaps == 0 {
		return api.SnapshotCollectionResponse{Data: []api.SnapshotResponse{}}, totalSnaps, nil
	}

	return api.SnapshotCollectionResponse{Data: snaps}, totalSnaps, nil
}

func readableSnapshots(db *gorm.DB, orgId string) *gorm.DB {
	return db.Model(&models.Snapshot{}).
		Preload("RepositoryConfiguration").
		Joins("JOIN repository_configurations ON repository_configuration_uuid = repository_configurations.uuid").
		Where("repository_configurations.org_id IN (?,?)", orgId, config.RedHatOrg).
		Where("snapshots.deleted_at IS NULL")
}

func (sDao *snapshotDaoImpl) Fetch(ctx context.Context, uuid string) (api.SnapshotResponse, error) {
	var snapAPI api.SnapshotResponse
	snapModel, err := sDao.fetch(ctx, uuid)
	if err != nil {
		return api.SnapshotResponse{}, err
	}
	SnapshotModelToApi(snapModel, &snapAPI)
	return snapAPI, nil
}

func (sDao *snapshotDaoImpl) fetch(ctx context.Context, uuid string) (models.Snapshot, error) {
	var snapshot models.Snapshot
	result := sDao.db.WithContext(ctx).
		Preload("RepositoryConfiguration").
		Where("uuid = ?", UuidifyString(uuid)).
		First(&snapshot)
	if result.Error != nil {
		return models.Snapshot{}, SnapshotsDBToApiError(result.Error, &uuid)
	}
	return snapshot, nil
}

func (sDao *snapshotDaoImpl) FetchModel(ctx context.Context, uuid string, includeSoftDel bool) (models.Snapshot, error) {
	var snap models.Snapshot
	var result *gorm.DB
	if includeSoftDel {
		result = sDao.db.WithContext(ctx).Unscoped().Where("uuid = ?", uuid).First(&snap)
	} else {
		result = sDao.db.WithContext(ctx).Where("uuid = ?", uuid).First(&snap)
	}
	if result.Error != nil {
		return models.Snapshot{}, SnapshotsDBToApiError(result.Error, &uuid)
	}
	return snap, nil
}

func SnapshotsDBToApiError(e error, uuid *string) *ce.DaoError {
	if e == nil {
		return nil
	}

	daoError := ce.DaoError{}
	if errors.Is(e, gorm.ErrRecordNotFound) {
		msg := "Snapshot not found"
		if uuid != nil {
			msg = fmt.Sprintf("Snapshot with UUID %s not found", *uuid)
		}
		daoError = ce.DaoError{
			Message:  msg,
			NotFound: true,
		}
	} else {
		daoError = ce.DaoError{
			Message:  e.Error(),
			NotFound: ce.HttpCodeForDaoError(e) == 404, // Check if isNotFoundError
		}
	}
	daoError.Wrap(e)
	return &daoError
}

func (sDao *snapshotDaoImpl) GetRepositoryConfigurationFile(ctx context.Context, orgID, snapshotUUID string, isLatest bool, isTemplate bool, templateUUID string) (string, error) {
	var repoID string
	snapshot, err := sDao.fetch(ctx, snapshotUUID)
	if err != nil {
		return "", err
	}

	rcDao := repositoryConfigDaoImpl{db: sDao.db}
	repoConfig, err := rcDao.fetchRepoConfig(ctx, orgID, snapshot.RepositoryConfigurationUUID, true)
	if err != nil {
		return "", err
	}

	pc := sDao.pulpClient
	contentPath, err := pc.GetContentPath(ctx)
	if err != nil {
		return "", err
	}

	contentURL := ""
	domain := strings.Split(snapshot.RepositoryPath, "/")[0]
	parsedRepoURL, err := url.Parse(repoConfig.Repository.URL)
	if err != nil {
		return "", err
	}
	path := parsedRepoURL.Path

	if isLatest {
		contentURL = pulpContentURL(contentPath, fmt.Sprintf("%v/%v/%v", domain, snapshot.RepositoryConfigurationUUID, "latest"))
	} else if isTemplate {
		contentURL = templateContentURL(contentPath, domain, templateUUID, snapshot.RepositoryConfigurationUUID, path)
	} else {
		contentURL = pulpContentURL(contentPath, snapshot.RepositoryPath)
	}

	// Replace any nonalphanumeric characters with an underscore
	// e.g: "!!my repo?test15()" => "__my_repo_test15__"
	re, err := regexp.Compile(`[^a-zA-Z0-9:space]`)
	if err != nil {
		return "", err
	}

	if repoConfig.IsRedHat() {
		repoID = repoConfig.Label
	} else {
		repoID = re.ReplaceAllString(repoConfig.Name, "_")
	}

	gpgCheck := 1
	gpgKeyField := models.RepoConfigGpgKeyURL(orgID, repoConfig.UUID)
	if err != nil {
		return "", fmt.Errorf("could not get GPGKey URL %w", err)
	}
	if gpgKeyField == nil {
		gpgKeyField = utils.Ptr("")
		gpgCheck = 0
	}

	moduleHotfixes := 0
	if repoConfig.ModuleHotfixes {
		moduleHotfixes = 1
	}

	// TODO purposefully setting repo_gpgcheck to 0 for now until pulp issue is resolved
	// normally set to 1 when metadata verification is enabled
	repoGpgCheck := 0

	fileConfig := fmt.Sprintf(""+
		"[%v]\n"+
		"name=%v\n"+
		"baseurl=%v\n"+
		"module_hotfixes=%v\n"+
		"gpgcheck=%v\n"+ // set to verify packages
		"repo_gpgcheck=%v\n"+ // set to verify metadata
		"enabled=1\n"+
		"gpgkey=%v\n"+
		"sslclientcert=/etc/pki/consumer/cert.pem\n"+
		"sslclientkey=/etc/pki/consumer/key.pem\n",
		repoID, repoConfig.Name, contentURL, moduleHotfixes, gpgCheck, repoGpgCheck, *gpgKeyField)

	return fileConfig, nil
}

func (sDao *snapshotDaoImpl) FetchForRepoConfigUUID(ctx context.Context, repoConfigUUID string) ([]models.Snapshot, error) {
	var snaps []models.Snapshot
	result := sDao.db.WithContext(ctx).Model(&models.Snapshot{}).
		Where("repository_configuration_uuid = ?", repoConfigUUID).
		Find(&snaps)
	if result.Error != nil {
		return snaps, result.Error
	}
	return snaps, nil
}

func (sDao *snapshotDaoImpl) SoftDelete(ctx context.Context, snapUUID string) error {
	var snap models.Snapshot
	err := sDao.db.WithContext(ctx).Where("uuid = ?", snapUUID).First(&snap).Error
	if err != nil {
		return SnapshotsDBToApiError(err, &snapUUID)
	}
	err = sDao.db.WithContext(ctx).Delete(&snap).Error
	if err != nil {
		return err
	}
	return nil
}

func (sDao *snapshotDaoImpl) Delete(ctx context.Context, snapUUID string) error {
	var snap models.Snapshot
	result := sDao.db.WithContext(ctx).Unscoped().Where("uuid = ?", snapUUID).First(&snap)
	if result.Error != nil {
		return result.Error
	}
	result = sDao.db.WithContext(ctx).Unscoped().Delete(&snap)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (sDao *snapshotDaoImpl) BulkDelete(ctx context.Context, uuids []string) []error {
	var errs []error

	_ = sDao.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		_, errs = sDao.bulkDelete(ctx, tx, uuids)
		if len(errs) > 0 {
			err = errors.New("rollback bulk delete")
		}
		return err
	})

	return errs
}

func (sDao *snapshotDaoImpl) bulkDelete(ctx context.Context, tx *gorm.DB, uuids []string) ([]api.SnapshotResponse, []error) {
	const savePoint = "before_delete"
	hasErr := false
	size := len(uuids)
	errs := make([]error, size)
	responses := make([]api.SnapshotResponse, size)

	tx.SavePoint(savePoint)
	for i := 0; i < size; i++ {
		snap, err := sDao.fetch(ctx, uuids[i])
		if err != nil {
			hasErr = true
			errs[i] = err
			tx.RollbackTo(savePoint)
			continue
		}

		err = tx.Delete(&snap).Error
		if err != nil {
			hasErr = true
			errs[i] = err
			tx.RollbackTo(savePoint)
			continue
		}

		if !hasErr {
			SnapshotModelToApi(snap, &responses[i])
		}
	}

	if hasErr {
		return []api.SnapshotResponse{}, errs
	}
	return responses, []error{}
}

func (sDao *snapshotDaoImpl) ClearDeletedAt(ctx context.Context, snapUUID string) error {
	var snap models.Snapshot
	err := sDao.db.WithContext(ctx).Unscoped().Where("uuid = ?", snapUUID).First(&snap).Error
	if err != nil {
		return SnapshotsDBToApiError(err, &snapUUID)
	}
	err = sDao.db.WithContext(ctx).Unscoped().Model(&snap).Update("deleted_at", nil).Error
	if err != nil {
		return err
	}
	return nil
}

func (sDao *snapshotDaoImpl) FetchLatestSnapshot(ctx context.Context, repoConfigUUID string) (api.SnapshotResponse, error) {
	var snap models.Snapshot
	snap, err := sDao.FetchLatestSnapshotModel(ctx, repoConfigUUID)
	if err != nil {
		return api.SnapshotResponse{}, err
	}
	var apiSnap api.SnapshotResponse
	SnapshotModelToApi(snap, &apiSnap)
	return apiSnap, nil
}

func (sDao *snapshotDaoImpl) FetchLatestSnapshotModel(ctx context.Context, repoConfigUUID string) (models.Snapshot, error) {
	var snap models.Snapshot
	result := sDao.db.WithContext(ctx).
		Preload("RepositoryConfiguration").
		Where("snapshots.repository_configuration_uuid = ?", repoConfigUUID).
		Order("created_at DESC").
		First(&snap)
	if result.Error != nil {
		return models.Snapshot{}, result.Error
	}
	return snap, nil
}

func (sDao *snapshotDaoImpl) FetchSnapshotByVersionHref(ctx context.Context, repoConfigUUID string, versionHref string) (*api.SnapshotResponse, error) {
	var snap models.Snapshot
	result := sDao.db.WithContext(ctx).
		Preload("RepositoryConfiguration").
		Where("snapshots.repository_configuration_uuid = ? AND version_href = ?", repoConfigUUID, versionHref).
		Order("created_at DESC").
		Limit(1).
		Find(&snap)
	if result.Error != nil {
		return nil, result.Error
	}
	if snap.UUID == "" {
		return nil, nil
	}
	var apiSnap api.SnapshotResponse
	SnapshotModelToApi(snap, &apiSnap)
	return &apiSnap, nil
}

func (sDao *snapshotDaoImpl) FetchSnapshotsModelByDateAndRepository(ctx context.Context, orgID string, request api.ListSnapshotByDateRequest) ([]models.Snapshot, error) {
	snaps := []models.Snapshot{}
	date := request.Date.UTC().Format(time.RFC3339)

	// finds the snapshot for each repo that is just before (or equal to) our date
	beforeQuery := sDao.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (s.repository_configuration_uuid) s.uuid
			FROM snapshots s
			INNER JOIN repository_configurations ON s.repository_configuration_uuid = repository_configurations.uuid
			WHERE s.repository_configuration_uuid IN ?
			AND s.deleted_at IS NULL
			AND repository_configurations.org_id IN ?
			AND date_trunc('second', s.created_at::timestamptz) <= ?
			ORDER BY s.repository_configuration_uuid,  s.created_at DESC
	`, request.RepositoryUUIDS, []string{orgID, config.RedHatOrg}, date)

	// finds the snapshot for each repo that is the first one after our date
	afterQuery := sDao.db.WithContext(ctx).Raw(`SELECT DISTINCT ON (s.repository_configuration_uuid) s.uuid
			FROM snapshots s
			INNER JOIN repository_configurations ON s.repository_configuration_uuid = repository_configurations.uuid
			WHERE s.repository_configuration_uuid IN ?
			AND s.deleted_at IS NULL
			AND repository_configurations.org_id IN ?
			AND date_trunc('second', s.created_at::timestamptz)  > ?
			ORDER BY s.repository_configuration_uuid, s.created_at ASC
	`, request.RepositoryUUIDS, []string{orgID, config.RedHatOrg}, date)
	// For each repo, pick the oldest of this combined set (ideally the one just before our date, if that doesn't exist, the one after)
	combined := sDao.db.WithContext(ctx).Raw(`
			select DISTINCT ON (s2.repository_configuration_uuid) s2.uuid
				from snapshots s2
				where s2.uuid in  ((?) UNION (?))
				ORDER BY s2.repository_configuration_uuid, s2.created_at ASC
		`, beforeQuery, afterQuery)

	result := sDao.db.WithContext(ctx).Model(&models.Snapshot{}).Where("uuid in (?)", combined).Find(&snaps)

	if result.Error != nil {
		return nil, fmt.Errorf("could not query snapshots for date %w", result.Error)
	}
	return snaps, nil
}

// FetchSnapshotsByDateAndRepository returns a list of snapshots by date.
func (sDao *snapshotDaoImpl) FetchSnapshotsByDateAndRepository(ctx context.Context, orgID string, request api.ListSnapshotByDateRequest) (api.ListSnapshotByDateResponse, error) {
	var snaps []models.Snapshot
	dateString := request.Date.Format(time.DateOnly)
	date, _ := time.Parse(time.DateOnly, dateString)
	date = date.AddDate(0, 0, 1) // Set the date to 24 hours later, inclusive of the current day

	snaps, err := sDao.FetchSnapshotsModelByDateAndRepository(ctx, orgID, request)
	if err != nil {
		return api.ListSnapshotByDateResponse{}, err
	}

	pulpContentPath, err := sDao.pulpClient.GetContentPath(ctx)
	if err != nil {
		return api.ListSnapshotByDateResponse{}, err
	}

	repoUUIDCount := len(request.RepositoryUUIDS)
	listResponse := make([]api.SnapshotForDate, repoUUIDCount)

	for i, uuid := range request.RepositoryUUIDS {
		listResponse[i].RepositoryUUID = uuid
		listResponse[i].IsAfter = false

		indx := slices.IndexFunc(snaps, func(c models.Snapshot) bool {
			return c.RepositoryConfigurationUUID == uuid
		})

		if indx != -1 {
			apiResponse := snapshotConvertToResponses([]models.Snapshot{snaps[indx]}, pulpContentPath)
			listResponse[i].Match = &apiResponse[0]
		}

		listResponse[i].IsAfter = indx != -1 && snaps[indx].Base.CreatedAt.After(date)
	}

	return api.ListSnapshotByDateResponse{Data: listResponse}, nil
}

// Converts the database models to our response objects
func snapshotConvertToResponses(snapshots []models.Snapshot, pulpContentPath string) []api.SnapshotResponse {
	snapsAPI := make([]api.SnapshotResponse, len(snapshots))
	for i := 0; i < len(snapshots); i++ {
		SnapshotModelToApi(snapshots[i], &snapsAPI[i])
		snapsAPI[i].URL = pulpContentURL(pulpContentPath, snapshots[i].RepositoryPath)
	}
	return snapsAPI
}

func SnapshotModelToApi(model models.Snapshot, resp *api.SnapshotResponse) {
	resp.UUID = model.UUID
	resp.CreatedAt = model.CreatedAt
	resp.RepositoryPath = model.RepositoryPath
	resp.ContentCounts = model.ContentCounts
	resp.AddedCounts = model.AddedCounts
	resp.RemovedCounts = model.RemovedCounts
	resp.RepositoryName = model.RepositoryConfiguration.Name
	resp.RepositoryUUID = model.RepositoryConfiguration.UUID
}

// pulpContentURL combines content path and repository path to get content URL
func pulpContentURL(pulpContentPath string, repositoryPath string) string {
	return pulpContentPath + repositoryPath + "/"
}

// templateContentURL combines content path, domain, template UUID, and either the RH path or repository UUID to get template content URL
func templateContentURL(pulpContentPath string, domain string, templateUUID string, repositoryUUID string, path string) string {
	if domain == config.RedHatDomainName {
		return pulpContentPath + domain + "/templates/" + templateUUID + path
	}
	return pulpContentPath + domain + "/templates/" + templateUUID + "/" + repositoryUUID
}
