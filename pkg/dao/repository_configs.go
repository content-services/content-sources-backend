package dao

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/RedHatInsights/event-schemas-go/apps/repositories/v1"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type repositoryConfigDaoImpl struct {
	db         *gorm.DB
	yumRepo    yum.YumRepository
	pulpClient pulp_client.PulpClient
}

func GetRepositoryConfigDao(db *gorm.DB, pulpClient pulp_client.PulpClient) RepositoryConfigDao {
	return &repositoryConfigDaoImpl{
		db:         db,
		yumRepo:    &yum.Repository{},
		pulpClient: pulpClient,
	}
}

func DBErrorToApi(e error) *ce.DaoError {
	var dupKeyName string
	if e == nil {
		return nil
	}

	pgError, ok := e.(*pgconn.PgError)
	if ok {
		if pgError.Code == "23505" {
			switch pgError.ConstraintName {
			case "repo_config_repo_org_id_deleted_null_unique":
				dupKeyName = "URL"
			case "repositories_unique_url":
				dupKeyName = "URL"
			case "repo_config_name_deleted_org_id_unique":
				dupKeyName = "name"
			case "repo_config_label_deleted_org_id_unique":
				dupKeyName = "label"
			}
			return &ce.DaoError{AlreadyExists: true, Message: "Repository with this " + dupKeyName + " already belongs to organization"}
		}
		if pgError.Code == "22021" {
			return &ce.DaoError{BadValidation: true, Message: "Request parameters contain invalid syntax"}
		}
	}
	dbError, ok := e.(models.Error)
	if ok {
		return &ce.DaoError{BadValidation: dbError.Validation, Message: dbError.Message}
	}

	daoErr := ce.DaoError{
		Message:  "Database Error",
		NotFound: ce.HttpCodeForDaoError(e) == 404, // Check if isNotFoundError
	}

	daoErr.Wrap(e)
	return &daoErr
}

func (r repositoryConfigDaoImpl) Create(ctx context.Context, newRepoReq api.RepositoryRequest) (api.RepositoryResponse, error) {
	var newRepo models.Repository
	var newRepoConfig models.RepositoryConfiguration

	if *newRepoReq.OrgID == config.RedHatOrg {
		return api.RepositoryResponse{}, errors.New("Creating of Red Hat repositories is not permitted")
	}

	if newRepoReq.Origin == nil || *newRepoReq.Origin == "" {
		// Default to external origin
		newRepoReq.Origin = utils.Ptr(config.OriginExternal)
	}
	if *newRepoReq.Origin == config.OriginUpload && (newRepoReq.Snapshot == nil || !*newRepoReq.Snapshot) {
		return api.RepositoryResponse{}, &ce.DaoError{BadValidation: true, Message: "Snapshot must be true for upload repositories"}
	}

	ApiFieldsToModel(newRepoReq, &newRepoConfig, &newRepo)

	if newRepo.URL == "" || newRepo.Origin == config.OriginUpload {
		if err := r.db.WithContext(ctx).Create(&newRepo).Error; err != nil {
			return api.RepositoryResponse{}, DBErrorToApi(err)
		}
	} else if newRepo.URL != "" {
		cleanedUrl := models.CleanupURL(newRepo.URL)
		// Repo configs with the same URL share a repository object
		if err := r.db.WithContext(ctx).Where("url = ?", cleanedUrl).FirstOrCreate(&newRepo).Error; err != nil {
			return api.RepositoryResponse{}, DBErrorToApi(err)
		}
	}
	if newRepoReq.OrgID != nil {
		newRepoConfig.OrgID = *newRepoReq.OrgID
	}
	if newRepoReq.AccountID != nil {
		newRepoConfig.AccountID = *newRepoReq.AccountID
	}
	newRepoConfig.RepositoryUUID = newRepo.Base.UUID

	if err := r.db.WithContext(ctx).Create(&newRepoConfig).Error; err != nil {
		return api.RepositoryResponse{}, DBErrorToApi(err)
	}

	// reload the repoConfig to fetch repository info too
	newRepoConfig, err := r.fetchRepoConfig(ctx, newRepoConfig.OrgID, newRepoConfig.UUID, false)
	if err != nil {
		return api.RepositoryResponse{}, DBErrorToApi(err)
	}

	var created api.RepositoryResponse
	newRepoConfig.Repository = newRepo
	ModelToApiFields(newRepoConfig, &created)

	created.URL = newRepo.URL
	created.LastIntrospectionStatus = newRepo.LastIntrospectionStatus

	event.SendNotification(
		newRepoConfig.OrgID,
		event.RepositoryCreated,
		[]repositories.Repositories{event.MapRepositoryResponse(created)},
	)

	return created, nil
}

func (r repositoryConfigDaoImpl) BulkCreate(ctx context.Context, newRepositories []api.RepositoryRequest) ([]api.RepositoryResponse, []error) {
	var responses []api.RepositoryResponse
	var errs []error

	_ = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		responses, errs = r.bulkCreate(tx.WithContext(ctx), newRepositories)
		if len(errs) > 0 {
			err = errors.New("rollback bulk create")
		}
		return err
	})

	mappedValues := []repositories.Repositories{}
	for i := 0; i < len(responses); i++ {
		mappedValues = append(mappedValues, event.MapRepositoryResponse(responses[i]))
	}
	event.SendNotification(*newRepositories[0].OrgID, event.RepositoryCreated, mappedValues)

	return responses, errs
}

func (r repositoryConfigDaoImpl) bulkCreate(tx *gorm.DB, newRepositories []api.RepositoryRequest) ([]api.RepositoryResponse, []error) {
	var dbErr error
	size := len(newRepositories)
	newRepoConfigs := make([]models.RepositoryConfiguration, size)
	newRepos := make([]models.Repository, size)
	responses := make([]api.RepositoryResponse, size)
	errorList := make([]error, size)
	tx.SavePoint("beforecreate")
	for i := 0; i < size; i++ {
		if newRepositories[i].Origin == nil {
			newRepositories[i].Origin = utils.Ptr(config.OriginExternal)
		}

		if *newRepositories[i].OrgID == config.RedHatOrg {
			dbErr = errors.New("Creating of Red Hat repositories is not permitted")
			errorList[i] = dbErr
			tx.RollbackTo("beforecreate")
			continue
		}
		if *newRepositories[i].Origin == config.OriginUpload && (newRepositories[i].Snapshot == nil || !*newRepositories[i].Snapshot) {
			dbErr = &ce.DaoError{BadValidation: true, Message: "Snapshot must be true for upload repositories"}
			errorList[i] = dbErr
			tx.RollbackTo("beforecreate")
			continue
		}

		if newRepositories[i].OrgID != nil {
			newRepoConfigs[i].OrgID = *(newRepositories[i].OrgID)
		}

		if newRepositories[i].AccountID != nil {
			newRepoConfigs[i].AccountID = *(newRepositories[i].AccountID)
		}

		ApiFieldsToModel(newRepositories[i], &newRepoConfigs[i], &newRepos[i])
		newRepos[i].LastIntrospectionStatus = "Pending"
		var err error
		if newRepos[i].URL == "" {
			err = tx.Create(&newRepos[i]).Error
		} else {
			cleanedUrl := models.CleanupURL(newRepos[i].URL)
			err = tx.Where("url = ?", cleanedUrl).FirstOrCreate(&newRepos[i]).Error
		}

		if err != nil {
			dbErr = DBErrorToApi(err)
			errorList[i] = dbErr
			tx.RollbackTo("beforecreate")
			continue
		}
		newRepoConfigs[i].RepositoryUUID = newRepos[i].UUID
		if err := tx.Create(&newRepoConfigs[i]).Error; err != nil {
			dbErr = DBErrorToApi(err)
			errorList[i] = dbErr
			tx.RollbackTo("beforecreate")
			continue
		}
		newRepoConfigs[i].Repository = newRepos[i] // Set repo on config for proper response values

		// If there is at least 1 error, skip creating responses
		if dbErr == nil {
			ModelToApiFields(newRepoConfigs[i], &responses[i])
			responses[i].URL = newRepos[i].URL
		}
	}

	// If there are no errors at all, return empty error slice.
	// If there is at least 1 error, return empty response slice.
	if dbErr == nil {
		return responses, []error{}
	}
	return []api.RepositoryResponse{}, errorList
}

type ListRepoFilter struct {
	URLs            *[]string
	RedhatOnly      *bool
	MinimumInterval *int // return enough repos so that at least this many times per day all repos will be returned
}

// Given the total number of repos needing snapshot in a day, find the minimum number to
//
//	snapshot in this iteration
func (r repositoryConfigDaoImpl) minimumSnapshotCount(pdb *gorm.DB, runsPerDay int) int {
	var totalCount int64
	query := pdb.Model(&models.RepositoryConfiguration{}).Where("snapshot IS TRUE").Count(&totalCount)
	if query.Error != nil {
		log.Logger.Error().Err(query.Error).Msg("Could not calculate total repo count")
		return 0
	}
	return (int(totalCount) / runsPerDay) + 1 // remainder will be less than runsPerDay, so just add 1 each time
}

func (r repositoryConfigDaoImpl) extraReposToSnapshot(pdb *gorm.DB, notIn *gorm.DB, count int) ([]models.RepositoryConfiguration, error) {
	extra := []models.RepositoryConfiguration{}
	query := snapshottableRepoConfigs(pdb.Joins("INNER JOIN repositories r on r.uuid = repository_configurations.repository_uuid")).
		Joins("LEFT JOIN tasks on last_snapshot_task_uuid = tasks.id").
		Where("repository_configurations.uuid not in (?)", notIn.Select("repository_configurations.uuid")).
		Where(pdb.Or("tasks.status NOT IN ?", []string{config.TaskStatusPending, config.TaskStatusRunning})).
		Order("tasks.finished_at ASC NULLS FIRST").Limit(count).Find(&extra)
	return extra, query.Error
}

func snapshottableRepoConfigs(db *gorm.DB) *gorm.DB {
	return db.Where("snapshot IS TRUE").Where("r.origin in ?", []string{config.OriginRedHat, config.OriginExternal})
}

func (r repositoryConfigDaoImpl) InternalOnly_ListReposToSnapshot(ctx context.Context, filter *ListRepoFilter) ([]models.RepositoryConfiguration, error) {
	var dbRepos []models.RepositoryConfiguration
	var query *gorm.DB
	pdb := r.db.WithContext(ctx)

	interval := fmt.Sprintf("%v hours", config.SnapshotForceInterval)
	if config.Get().Options.AlwaysRunCronTasks {
		query = pdb.Where("snapshot IS TRUE")
	} else {
		query = pdb.Where("snapshot IS TRUE").Joins("LEFT JOIN tasks on last_snapshot_task_uuid = tasks.id").
			Where(pdb.Where("tasks.queued_at <= (now() - cast(? as interval))", interval).
				Or("tasks.status NOT IN ?", []string{config.TaskStatusCompleted, config.TaskStatusPending, config.TaskStatusRunning}).
				Or("last_snapshot_task_uuid is NULL"))
	}
	query = query.Joins("INNER JOIN repositories r on r.uuid = repository_configurations.repository_uuid")
	query = snapshottableRepoConfigs(query)
	if filter != nil {
		if filter.RedhatOnly != nil && *filter.RedhatOnly {
			query = query.Where("r.origin = ?", config.OriginRedHat)
		}
		if filter.URLs != nil {
			query = query.Where("r.url in ?", *filter.URLs)
		}
	}
	result := snapshottableRepoConfigs(query).Find(&dbRepos)

	if result.Error != nil {
		return dbRepos, result.Error
	}
	if filter != nil && filter.MinimumInterval != nil && *filter.MinimumInterval > 0 {
		min := r.minimumSnapshotCount(pdb, *filter.MinimumInterval)
		if len(dbRepos) < min {
			extraRepos, err := r.extraReposToSnapshot(pdb, query, min-len(dbRepos))
			if err != nil {
				return dbRepos, err
			}
			dbRepos = append(dbRepos, extraRepos...)
		}
	}
	return dbRepos, nil
}

func (r repositoryConfigDaoImpl) List(
	ctx context.Context,
	OrgID string,
	pageData api.PaginationData,
	filterData api.FilterData,
) (api.RepositoryCollectionResponse, int64, error) {
	var totalRepos int64
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	var contentPath string

	filteredDB, err := r.filteredDbForList(OrgID, r.db.WithContext(ctx), filterData)
	if err != nil {
		return api.RepositoryCollectionResponse{}, totalRepos, err
	}

	sortMap := map[string]string{
		"name":                      "name",
		"url":                       "url",
		"distribution_arch":         "arch",
		"distribution_versions":     "array_to_string(versions, ',')",
		"package_count":             "package_count",
		"last_introspection_time":   "last_introspection_time",
		"last_introspection_status": "last_introspection_status",
	}

	order := convertSortByToSQL(pageData.SortBy, sortMap, "name asc")

	// Get count
	filteredDB.
		Model(&repoConfigs).
		Count(&totalRepos)

	if filteredDB.Error != nil {
		return api.RepositoryCollectionResponse{}, totalRepos, filteredDB.Error
	}

	// Get Data
	filteredDB.
		Order(order).
		Preload("Repository").
		Preload("LastSnapshot").
		Preload("LastSnapshotTask").
		Limit(pageData.Limit).
		Offset(pageData.Offset).
		Find(&repoConfigs)

	if filteredDB.Error != nil {
		return api.RepositoryCollectionResponse{}, totalRepos, filteredDB.Error
	}

	if config.Get().Features.Snapshots.Enabled {
		dDao := domainDaoImpl{db: r.db}
		domain, err := dDao.Fetch(ctx, OrgID)
		if err != nil {
			return api.RepositoryCollectionResponse{}, totalRepos, err
		}

		contentPath, err = r.pulpClient.WithDomain(domain).GetContentPath(ctx)
		if err != nil {
			return api.RepositoryCollectionResponse{}, totalRepos, err
		}
	}

	repos := convertToResponses(repoConfigs, contentPath)

	return api.RepositoryCollectionResponse{Data: repos}, totalRepos, nil
}

func (r repositoryConfigDaoImpl) filteredDbForList(OrgID string, filteredDB *gorm.DB, filterData api.FilterData) (*gorm.DB, error) {
	filteredDB = filteredDB.Where("repository_configurations.org_id in ?", []string{OrgID, config.RedHatOrg}).
		Joins("inner join repositories on repository_configurations.repository_uuid = repositories.uuid")

	if filterData.Name != "" {
		filteredDB = filteredDB.Where("name = ?", filterData.Name)
	}

	if filterData.ContentType != "" {
		filteredDB = filteredDB.Where("repositories.content_type = ?", filterData.ContentType)
	}

	if filterData.URL != "" {
		urls := strings.Split(filterData.URL, ",")
		for i := 0; i < len(urls); i++ {
			urls[i] = models.CleanupURL(urls[i])
		}
		filteredDB = filteredDB.Where("repositories.url IN ?", urls)
	}

	if filterData.UUID != "" {
		uuids := strings.Split(filterData.UUID, ",")
		filteredDB = filteredDB.Where("repository_configurations.uuid IN ?", UuidifyStrings(uuids))
	}

	if filterData.AvailableForArch != "" {
		filteredDB = filteredDB.Where("arch = ? OR arch = '' OR arch = 'any'", filterData.AvailableForArch)
	}
	if filterData.AvailableForVersion != "" {
		filteredDB = filteredDB.
			Where("? = any (versions) OR 'any' = any (versions) OR array_length(versions, 1) IS NULL", filterData.AvailableForVersion)
	}

	if filterData.Search != "" {
		containsSearch := "%" + filterData.Search + "%"
		filteredDB = filteredDB.
			Where("name ILIKE ? OR url ILIKE ?", containsSearch, containsSearch)
	}

	if filterData.Origin != "" {
		origins := strings.Split(filterData.Origin, ",")
		filteredDB = filteredDB.Where("repositories.origin IN ?", origins)
	}

	if filterData.Arch != "" {
		arches := strings.Split(filterData.Arch, ",")
		filteredDB = filteredDB.Where("arch IN ?", arches)
	}

	if filterData.Version != "" {
		versions := strings.Split(filterData.Version, ",")
		orGroup := r.db.Where("? = any (versions)", versions[0])
		for i := 1; i < len(versions); i++ {
			orGroup = orGroup.Or("? = any (versions)", versions[i])
		}
		filteredDB = filteredDB.Where(orGroup)
	}

	if filterData.Status != "" {
		statuses := strings.Split(filterData.Status, ",")

		filteredDB = filteredDB.
			Joins("LEFT JOIN tasks ON repository_configurations.last_snapshot_task_uuid = tasks.id").
			Preload("Repository").
			Preload("LastSnapshotTask")

		var filterChain *gorm.DB
		for _, status := range statuses {
			switch status {
			case config.StatusValid, config.StatusUnavailable, config.StatusInvalid, config.StatusPending:
				if filterChain == nil {
					// first where statement, so generate just it
					filterChain = getStatusFilter(status, r.db)
				} else {
					// after the first, OR it with the previous statements
					filterChain = filterChain.Or(getStatusFilter(status, r.db))
				}
			default:
				return filteredDB, &ce.DaoError{
					BadValidation: true,
					Message:       "Invalid status provided: " + status,
				}
			}
		}
		filteredDB = filteredDB.Where(filterChain)
	}
	return filteredDB, nil
}

func getStatusFilter(status string, filteredDB *gorm.DB) *gorm.DB {
	if status == "Valid" {
		filteredDB = filteredDB.Where("(repositories.last_introspection_status = 'Valid' AND tasks.type = 'snapshot' AND tasks.status = 'completed')").
			Or("(repositories.last_introspection_status = 'Valid' AND repository_configurations.snapshot = 'false')")
	}
	if status == "Pending" {
		filteredDB = filteredDB.Where(
			"repositories.last_introspection_status = 'Pending' AND (repository_configurations.last_snapshot_task_uuid IS NULL OR (tasks.type = 'snapshot' AND (tasks.status = 'running' OR tasks.status = 'pending' OR tasks.status = 'completed')))").
			Or("repositories.last_introspection_status = 'Valid' AND repository_configurations.last_snapshot_uuid IS NULL AND tasks.type = 'snapshot' AND (tasks.status = 'running' OR tasks.status = 'pending')").
			Or("repositories.last_introspection_status = 'Valid' AND repository_configurations.last_snapshot_uuid IS NOT NULL AND tasks.type = 'snapshot' AND (tasks.status = 'running' OR tasks.status = 'pending')").
			Or("repositories.last_introspection_status = 'Pending' AND repository_configurations.snapshot = 'false'")
	}
	if status == "Unavailable" {
		filteredDB = filteredDB.Where(
			"repositories.last_introspection_status = 'Unavailable' AND repository_configurations.last_snapshot_uuid IS NOT NULL AND tasks.type = 'snapshot' AND (tasks.status = 'failed' OR tasks.status = 'completed')").
			Or("repositories.last_introspection_status = 'Invalid' AND repository_configurations.last_snapshot_uuid IS NOT NULL AND tasks.type = 'snapshot' AND tasks.status = 'failed'").
			Or("repositories.last_introspection_status = 'Valid' AND repository_configurations.last_snapshot_uuid IS NOT NULL AND tasks.type = 'snapshot' AND tasks.status = 'failed'").
			Or("repositories.last_introspection_status = 'Unavailable' AND repository_configurations.snapshot = 'false'")
	}
	if status == "Invalid" {
		filteredDB = filteredDB.Where(
			"repositories.last_introspection_status = 'Invalid' AND tasks.type = 'snapshot' AND tasks.status = 'completed'").
			Or("repositories.last_introspection_status = 'Unavailable' AND repository_configurations.last_snapshot_uuid IS NULL AND tasks.type = 'snapshot' AND tasks.status = 'failed'").
			Or("repositories.last_introspection_status = 'Valid' AND repository_configurations.last_snapshot_uuid IS NULL AND tasks.type = 'snapshot' AND tasks.status = 'failed'").
			Or("repositories.last_introspection_status = 'Invalid' AND repository_configurations.snapshot = 'false'")
	}
	return filteredDB
}

func (r repositoryConfigDaoImpl) InternalOnly_FetchRepoConfigsForRepoUUID(ctx context.Context, uuid string) []api.RepositoryResponse {
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	filteredDB := r.db.WithContext(ctx).Where("repositories.uuid = ?", UuidifyString(uuid)).
		Joins("inner join repositories on repository_configurations.repository_uuid = repositories.uuid")

	filteredDB.Preload("Repository").Preload("LastSnapshot").Preload("LastSnapshotTask").Find(&repoConfigs)
	if filteredDB.Error != nil {
		if !errors.Is(filteredDB.Error, context.Canceled) {
			log.Error().Err(filteredDB.Error).Msgf("error fetching repoConfigs for repo")
		}
		return []api.RepositoryResponse{}
	}

	return convertToResponses(repoConfigs, "")
}

func (r repositoryConfigDaoImpl) Fetch(ctx context.Context, orgID string, uuid string) (api.RepositoryResponse, error) {
	var repo api.RepositoryResponse

	repoConfig, err := r.fetchRepoConfig(ctx, orgID, uuid, true)
	if err != nil {
		return api.RepositoryResponse{}, err
	}

	ModelToApiFields(repoConfig, &repo)

	if repoConfig.LastSnapshot != nil && config.Get().Features.Snapshots.Enabled {
		dDao := domainDaoImpl{db: r.db}
		domainName, err := dDao.Fetch(ctx, orgID)
		if err != nil {
			return api.RepositoryResponse{}, err
		}
		contentPath, err := r.pulpClient.WithDomain(domainName).GetContentPath(ctx)
		if err != nil {
			return api.RepositoryResponse{}, err
		}
		contentURL := pulpContentURL(contentPath, repoConfig.LastSnapshot.RepositoryPath)
		repo.LastSnapshot.URL = contentURL
	}
	return repo, nil
}

// fetchRepConfig: "includeRedHatRepos" allows the fetching of red_hat repositories
func (r repositoryConfigDaoImpl) fetchRepoConfig(ctx context.Context, orgID string, uuid string, includeRedHatRepos bool) (models.RepositoryConfiguration, error) {
	found := models.RepositoryConfiguration{}

	orgIdsToCheck := []string{orgID}

	if includeRedHatRepos {
		orgIdsToCheck = append(orgIdsToCheck, config.RedHatOrg)
	}

	result := r.db.WithContext(ctx).
		Preload("Repository").Preload("LastSnapshot").Preload("LastSnapshotTask").
		Where("UUID = ? AND ORG_ID IN ?", UuidifyString(uuid), orgIdsToCheck).
		First(&found)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return found, &ce.DaoError{NotFound: true, Message: "Could not find repository with UUID " + uuid}
		}
		return found, DBErrorToApi(result.Error)
	}
	return found, nil
}

func (r repositoryConfigDaoImpl) FetchByRepoUuid(ctx context.Context, orgID string, repoUuid string) (api.RepositoryResponse, error) {
	repoConfig := models.RepositoryConfiguration{}
	repo := api.RepositoryResponse{}

	result := r.db.WithContext(ctx).
		Preload("Repository").Preload("LastSnapshot").Preload("LastSnapshotTask").
		Joins("Inner join repositories on repositories.uuid = repository_configurations.repository_uuid").
		Where("Repositories.UUID = ? AND ORG_ID = ?", UuidifyString(repoUuid), orgID).
		First(&repoConfig)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return repo, &ce.DaoError{NotFound: true, Message: "Could not find repository with UUID " + repoUuid}
		}
		return repo, DBErrorToApi(result.Error)
	}

	ModelToApiFields(repoConfig, &repo)
	return repo, nil
}

func (r repositoryConfigDaoImpl) FetchWithoutOrgID(ctx context.Context, uuid string) (api.RepositoryResponse, error) {
	found := models.RepositoryConfiguration{}
	var repo api.RepositoryResponse
	result := r.db.WithContext(ctx).
		Preload("Repository").Preload("LastSnapshot").Preload("LastSnapshotTask").
		Where("UUID = ?", UuidifyString(uuid)).
		First(&found)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return repo, &ce.DaoError{NotFound: true, Message: "Could not find repository with UUID " + uuid}
		} else {
			return repo, DBErrorToApi(result.Error)
		}
	}
	ModelToApiFields(found, &repo)
	return repo, nil
}

// Update updates a RepositoryConfig with changed parameters.  Returns whether the url changed, and an error if updating failed
func (r repositoryConfigDaoImpl) Update(ctx context.Context, orgID, uuid string, repoParams api.RepositoryUpdateRequest) (bool, error) {
	var repo models.Repository
	var repoConfig models.RepositoryConfiguration
	var err error
	updatedUrl := false

	// We are updating the repo config & snapshots, so bundle in a transaction
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Setting "includeRedHatRepos" to false here to prevent updating red_hat repositories
		if repoConfig, err = r.fetchRepoConfig(ctx, orgID, uuid, false); err != nil {
			return err
		}
		ApiUpdateFieldsToModel(repoParams, &repoConfig, &repo)

		if repoConfig.Repository.Origin == config.OriginUpload && repoParams.URL != nil && *repoParams.URL != "" {
			return &ce.DaoError{BadValidation: true, Message: "Cannot set URL on upload repositories"}
		}
		if repoConfig.Repository.Origin == config.OriginUpload && (repoParams.Snapshot != nil && !*repoParams.Snapshot) {
			return &ce.DaoError{BadValidation: true, Message: "Snapshot must be true for upload repositories"}
		}

		// If URL is included in params, and not an upload repo, search for existing
		// Repository record, or create a new one.
		// Then replace existing Repository/RepoConfig association.
		if repoParams.URL != nil && repoConfig.Repository.Origin != config.OriginUpload {
			cleanedUrl := models.CleanupURL(*repoParams.URL)
			err = tx.FirstOrCreate(&repo, "url = ?", cleanedUrl).Error
			if err != nil {
				return DBErrorToApi(err)
			}
			repoConfig.RepositoryUUID = repo.UUID
			updatedUrl = repoConfig.Repository.URL != *repoParams.URL
		}

		repoConfig.Repository = models.Repository{}
		if err := tx.Model(&repoConfig).Omit("LastSnapshot").Updates(repoConfig.MapForUpdate()).Error; err != nil {
			return DBErrorToApi(err)
		}

		repositoryResponse := api.RepositoryResponse{}
		ModelToApiFields(repoConfig, &repositoryResponse)

		event.SendNotification(
			orgID,
			event.RepositoryUpdated,
			[]repositories.Repositories{event.MapRepositoryResponse(repositoryResponse)},
		)
		return nil
	})
	if err != nil {
		return updatedUrl, err
	}

	repositoryResponse := api.RepositoryResponse{}
	ModelToApiFields(repoConfig, &repositoryResponse)

	event.SendNotification(
		orgID,
		event.RepositoryUpdated,
		[]repositories.Repositories{event.MapRepositoryResponse(repositoryResponse)},
	)

	repoConfig.Repository = models.Repository{}
	if err := r.db.WithContext(ctx).Model(&repoConfig).Omit("LastSnapshot").Updates(repoConfig.MapForUpdate()).Error; err != nil {
		return updatedUrl, DBErrorToApi(err)
	}

	return updatedUrl, nil
}

// UpdateLastSnapshotTask updates the RepositoryConfig with the latest SnapshotTask
func (r repositoryConfigDaoImpl) UpdateLastSnapshotTask(ctx context.Context, taskUUID string, orgID string, repoUUID string) error {
	result := r.db.WithContext(ctx).Exec(`
			UPDATE repository_configurations 
			SET last_snapshot_task_uuid = ? 
			WHERE repository_configurations.org_id = ?
			AND repository_configurations.repository_uuid = ?`,
		taskUUID,
		orgID,
		repoUUID,
	)

	if result.Error != nil {
		return result.Error
	}
	return nil
}

// SavePublicRepos saves a list of urls and marks them as "Public"
// This is meant for the list of repositories that are preloaded for all
// users.
func (r repositoryConfigDaoImpl) SavePublicRepos(ctx context.Context, urls []string) error {
	var repos []models.Repository
	cleanedUrls := []string{}
	for _, url := range urls {
		cleanedUrls = append(cleanedUrls, models.CleanupURL(url))
	}

	for i := 0; i < len(urls); i++ {
		repos = append(repos, models.Repository{URL: models.CleanupURL(urls[i]), Public: true})
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "url"}},
		DoUpdates: clause.AssignmentColumns([]string{"public"})}).Create(&repos)
	if result.Error != nil {
		return result.Error
	}

	result = r.db.WithContext(ctx).Model(&models.Repository{}).Where("public = true and url not in (?)", cleanedUrls).Update("public", false)
	return result.Error
}

func (r repositoryConfigDaoImpl) SoftDelete(ctx context.Context, orgID string, uuid string) error {
	var repoConfig models.RepositoryConfiguration
	var err error

	if repoConfig, err = r.fetchRepoConfig(ctx, orgID, uuid, false); err != nil {
		return err
	}

	if err = r.db.WithContext(ctx).Delete(&repoConfig).Error; err != nil {
		return err
	}

	repositoryResponse := api.RepositoryResponse{}
	ModelToApiFields(repoConfig, &repositoryResponse)

	event.SendNotification(
		orgID,
		event.RepositoryDeleted,
		[]repositories.Repositories{event.MapRepositoryResponse(repositoryResponse)},
	)

	return nil
}

func (r repositoryConfigDaoImpl) Delete(ctx context.Context, orgID string, uuid string) error {
	var repoConfig models.RepositoryConfiguration

	err := r.db.WithContext(ctx).Unscoped().Where("uuid = ? AND org_id = ?", UuidifyString(uuid), orgID).First(&repoConfig).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &ce.DaoError{NotFound: true, Message: "Could not find repository with UUID " + uuid}
		}
		return DBErrorToApi(err)
	}

	if err = r.db.WithContext(ctx).Unscoped().Delete(&repoConfig).Error; err != nil {
		return err
	}

	return nil
}

func (r repositoryConfigDaoImpl) BulkDelete(ctx context.Context, orgID string, uuids []string) []error {
	var responses []api.RepositoryResponse
	var errs []error

	_ = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		responses, errs = r.bulkDelete(ctx, tx, orgID, uuids)
		if len(errs) > 0 {
			err = errors.New("rollback bulk delete")
		}
		return err
	})

	if len(responses) > 0 {
		mappedValues := make([]repositories.Repositories, len(responses))
		for i := 0; i < len(responses); i++ {
			mappedValues[i] = event.MapRepositoryResponse(responses[i])
		}
		event.SendNotification(orgID, event.RepositoryDeleted, mappedValues)
	}

	return errs
}

func (r repositoryConfigDaoImpl) bulkDelete(ctx context.Context, tx *gorm.DB, orgID string, uuids []string) ([]api.RepositoryResponse, []error) {
	var dbErr error
	size := len(uuids)
	errors := make([]error, size)
	responses := make([]api.RepositoryResponse, size)
	const save = "beforedelete"

	tx.SavePoint(save)
	for i := 0; i < size; i++ {
		var err error
		var repoConfig models.RepositoryConfiguration

		if repoConfig, err = r.fetchRepoConfig(ctx, orgID, uuids[i], false); err != nil {
			dbErr = DBErrorToApi(err)
			errors[i] = dbErr
			tx.RollbackTo(save)
			continue
		}

		if err = tx.Delete(&repoConfig).Error; err != nil {
			dbErr = DBErrorToApi(err)
			errors[i] = dbErr
			tx.RollbackTo(save)
			continue
		}

		if dbErr == nil {
			ModelToApiFields(repoConfig, &responses[i])
		}
	}

	if dbErr == nil {
		return responses, []error{}
	} else {
		return []api.RepositoryResponse{}, errors
	}
}

func (r repositoryConfigDaoImpl) BulkExport(ctx context.Context, orgID string, reposToExport api.RepositoryExportRequest) ([]api.RepositoryExportResponse, error) {
	var repoConfigs []models.RepositoryConfiguration

	if err := r.validateRepositoryUUIDs(ctx, orgID, reposToExport.RepositoryUuids); err != nil {
		return []api.RepositoryExportResponse{}, err
	}

	result := r.db.WithContext(ctx).Model(&repoConfigs).
		Preload("Repository").
		Joins("inner join repositories on repository_configurations.repository_uuid = repositories.uuid").
		Where("repository_configurations.uuid IN ? and repository_configurations.org_id = ?", reposToExport.RepositoryUuids, orgID).
		Find(&repoConfigs)
	if result.Error != nil {
		return []api.RepositoryExportResponse{}, result.Error
	}

	repos := make([]api.RepositoryExportResponse, len(repoConfigs))
	for i := 0; i < len(repoConfigs); i++ {
		ModelToExportRepoApi(repoConfigs[i], &repos[i])
	}

	return repos, nil
}

func (r repositoryConfigDaoImpl) validateRepositoryUUIDs(ctx context.Context, orgId string, uuids []string) error {
	var count int64
	resp := r.db.WithContext(ctx).Model(models.RepositoryConfiguration{}).Where("org_id = ?", orgId).Where("uuid in ?", UuidifyStrings(uuids)).Count(&count)
	if resp.Error != nil {
		return fmt.Errorf("could not query repository uuids: %w", resp.Error)
	}
	if count != int64(len(uuids)) {
		return &ce.DaoError{NotFound: true, Message: "One or more Repository UUIDs was invalid."}
	}
	return nil
}

func (r repositoryConfigDaoImpl) BulkImport(ctx context.Context, reposToImport []api.RepositoryRequest) ([]api.RepositoryImportResponse, []error) {
	var responses []api.RepositoryImportResponse
	var errs []error

	_ = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		responses, errs = r.bulkImport(tx.WithContext(ctx), reposToImport)
		if len(errs) > 0 {
			err = errors.New("rollback bulk import")
		}
		return err
	})

	return responses, errs
}

func (r repositoryConfigDaoImpl) bulkImport(tx *gorm.DB, reposToImport []api.RepositoryRequest) ([]api.RepositoryImportResponse, []error) {
	var dbErr error
	size := len(reposToImport)
	newRepoConfigs := make([]models.RepositoryConfiguration, size)
	newRepos := make([]models.Repository, size)
	responses := make([]api.RepositoryImportResponse, size)
	errorList := make([]error, size)
	tx.SavePoint("beforeimport")
	for i := 0; i < size; i++ {
		if reposToImport[i].Origin == nil {
			reposToImport[i].Origin = utils.Ptr(config.OriginExternal)
		}
		if *reposToImport[i].Origin == config.OriginUpload || *reposToImport[i].Origin == config.OriginRedHat {
			dbErr = &ce.DaoError{
				BadValidation: true,
				Message:       "Cannot import upload or Red Hat repositories",
			}
			errorList[i] = dbErr
			tx.RollbackTo("beforeimport")
			continue
		}
		if reposToImport[i].OrgID != nil {
			newRepoConfigs[i].OrgID = *reposToImport[i].OrgID
		}
		if reposToImport[i].AccountID != nil {
			newRepoConfigs[i].AccountID = *reposToImport[i].AccountID
		}

		ApiFieldsToModel(reposToImport[i], &newRepoConfigs[i], &newRepos[i])
		newRepos[i].LastIntrospectionStatus = "Pending"

		var err error
		cleanedUrl := models.CleanupURL(newRepos[i].URL)
		var existingRepo models.RepositoryConfiguration
		// check for existing repo
		err = tx.
			Preload("Repository").
			Preload("LastSnapshot").
			Preload("LastSnapshotTask").
			Joins("inner join repositories on repository_configurations.repository_uuid = repositories.uuid").
			Where("repositories.url = ? and repository_configurations.org_id = ?", cleanedUrl, newRepoConfigs[i].OrgID).
			First(&existingRepo).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			dbErr = DBErrorToApi(err)
			errorList[i] = dbErr
			tx.RollbackTo("beforeimport")
			continue
		}

		if err == nil {
			// repo with same URL already exists, check for mismatched fields and don't create repo
			warnings := checkWarningsOnImport(existingRepo, newRepoConfigs, i)
			if len(warnings) > 0 {
				responses[i].Warnings = warnings
			}
			if dbErr == nil {
				ModelToImportRepoApi(existingRepo, responses[i].Warnings, &responses[i])
				responses[i].URL = newRepos[i].URL
			}
		} else {
			// no existing repo, create (or find) repo and create repo config
			if err = tx.Where("url = ?", cleanedUrl).FirstOrCreate(&newRepos[i]).Error; err != nil {
				dbErr = DBErrorToApi(err)
				errorList[i] = dbErr
				tx.RollbackTo("beforeimport")
				continue
			}
			newRepoConfigs[i].RepositoryUUID = newRepos[i].UUID
			if err = tx.Create(&newRepoConfigs[i]).Error; err != nil {
				dbErr = DBErrorToApi(err)
				errorList[i] = dbErr
				tx.RollbackTo("beforeimport")
				continue
			}
			newRepoConfigs[i].Repository = newRepos[i] // Set repo on config for proper response values
			if dbErr == nil {
				ModelToImportRepoApi(newRepoConfigs[i], responses[i].Warnings, &responses[i])
				responses[i].URL = newRepos[i].URL
			}
		}
	}

	// If there are no errors at all, return empty error slice.
	// If there is at least 1 error, return empty response slice.
	if dbErr == nil {
		return responses, []error{}
	}
	return []api.RepositoryImportResponse{}, errorList
}

func checkWarningsOnImport(existingRepo models.RepositoryConfiguration, newRepoConfigs []models.RepositoryConfiguration, index int) []map[string]interface{} {
	var warnings []map[string]interface{}
	warnings = append(warnings, map[string]interface{}{
		"field":    "url",
		"existing": existingRepo.Repository.URL,
	})
	if existingRepo.Name != newRepoConfigs[index].Name {
		warnings = append(warnings, map[string]interface{}{
			"field":    "name",
			"existing": existingRepo.Name,
			"new":      newRepoConfigs[index].Name,
		})
	}
	if existingRepo.Arch != newRepoConfigs[index].Arch {
		warnings = append(warnings, map[string]interface{}{
			"field":    "distribution_arch",
			"existing": existingRepo.Arch,
			"new":      newRepoConfigs[index].Arch,
		})
	}
	sort.Strings(existingRepo.Versions)
	sort.Strings(newRepoConfigs[index].Versions)
	if !utils.SlicesEqual(existingRepo.Versions, newRepoConfigs[index].Versions) {
		warnings = append(warnings, map[string]interface{}{
			"field":    "distribution_versions",
			"existing": existingRepo.Versions,
			"new":      newRepoConfigs[index].Versions,
		})
	}
	if existingRepo.GpgKey != newRepoConfigs[index].GpgKey {
		warnings = append(warnings, map[string]interface{}{
			"field":    "gpg_key",
			"existing": existingRepo.GpgKey,
			"new":      newRepoConfigs[index].GpgKey,
		})
	}
	if existingRepo.MetadataVerification != newRepoConfigs[index].MetadataVerification {
		warnings = append(warnings, map[string]interface{}{
			"field":    "metadata_verification",
			"existing": existingRepo.MetadataVerification,
			"new":      newRepoConfigs[index].MetadataVerification,
		})
	}
	if existingRepo.ModuleHotfixes != newRepoConfigs[index].ModuleHotfixes {
		warnings = append(warnings, map[string]interface{}{
			"field":    "module_hotfixes",
			"existing": existingRepo.ModuleHotfixes,
			"new":      newRepoConfigs[index].ModuleHotfixes,
		})
	}
	if existingRepo.Snapshot != newRepoConfigs[index].Snapshot {
		warnings = append(warnings, map[string]interface{}{
			"field":    "snapshot",
			"existing": existingRepo.Snapshot,
			"new":      newRepoConfigs[index].Snapshot,
		})
	}
	return warnings
}

func ApiUpdateFieldsToModel(apiRepo api.RepositoryUpdateRequest, repoConfig *models.RepositoryConfiguration, repo *models.Repository) {
	if apiRepo.Name != nil {
		repoConfig.Name = *apiRepo.Name
	}
	if apiRepo.DistributionArch != nil {
		repoConfig.Arch = *apiRepo.DistributionArch
	}
	if apiRepo.DistributionVersions != nil {
		repoConfig.Versions = *apiRepo.DistributionVersions
	}
	if apiRepo.URL != nil {
		repo.URL = *apiRepo.URL
	}
	if apiRepo.GpgKey != nil {
		repoConfig.GpgKey = *apiRepo.GpgKey
	}
	if apiRepo.MetadataVerification != nil {
		repoConfig.MetadataVerification = *apiRepo.MetadataVerification
	}
	if apiRepo.ModuleHotfixes != nil {
		repoConfig.ModuleHotfixes = *apiRepo.ModuleHotfixes
	}
	if apiRepo.Snapshot != nil {
		repoConfig.Snapshot = *apiRepo.Snapshot
	}
}

func ApiFieldsToModel(apiRepo api.RepositoryRequest, repoConfig *models.RepositoryConfiguration, repo *models.Repository) {
	// Origin can only be set on creation, cannot be changed
	if repoConfig.UUID == "" {
		if apiRepo.Origin != nil {
			repo.Origin = *apiRepo.Origin
		}
	}

	// copied from ApiUpdateFieldsToModel
	if apiRepo.Name != nil {
		repoConfig.Name = *apiRepo.Name
	}
	if apiRepo.DistributionArch != nil {
		repoConfig.Arch = *apiRepo.DistributionArch
	}
	if apiRepo.DistributionVersions != nil {
		repoConfig.Versions = *apiRepo.DistributionVersions
	}
	if apiRepo.URL != nil {
		repo.URL = *apiRepo.URL
	}
	if apiRepo.GpgKey != nil {
		repoConfig.GpgKey = *apiRepo.GpgKey
	}
	if apiRepo.MetadataVerification != nil {
		repoConfig.MetadataVerification = *apiRepo.MetadataVerification
	}
	if apiRepo.ModuleHotfixes != nil {
		repoConfig.ModuleHotfixes = *apiRepo.ModuleHotfixes
	}
	if apiRepo.Snapshot != nil {
		repoConfig.Snapshot = *apiRepo.Snapshot
	}
}

func ModelToApiFields(repoConfig models.RepositoryConfiguration, apiRepo *api.RepositoryResponse) {
	apiRepo.UUID = repoConfig.UUID
	apiRepo.PackageCount = repoConfig.Repository.PackageCount
	apiRepo.Origin = repoConfig.Repository.Origin
	apiRepo.ContentType = repoConfig.Repository.ContentType
	apiRepo.URL = repoConfig.Repository.URL
	apiRepo.Name = repoConfig.Name
	apiRepo.DistributionVersions = repoConfig.Versions
	apiRepo.DistributionArch = repoConfig.Arch
	apiRepo.AccountID = repoConfig.AccountID
	apiRepo.OrgID = repoConfig.OrgID
	apiRepo.Status = combineIntrospectionAndSnapshotStatuses(&repoConfig, &repoConfig.Repository)
	apiRepo.LastIntrospectionStatus = repoConfig.Repository.LastIntrospectionStatus
	apiRepo.GpgKey = repoConfig.GpgKey
	apiRepo.MetadataVerification = repoConfig.MetadataVerification
	apiRepo.ModuleHotfixes = repoConfig.ModuleHotfixes
	apiRepo.FailedIntrospectionsCount = repoConfig.Repository.FailedIntrospectionsCount
	apiRepo.RepositoryUUID = repoConfig.RepositoryUUID
	apiRepo.Snapshot = repoConfig.Snapshot
	apiRepo.Label = repoConfig.Label

	apiRepo.LastSnapshotUUID = repoConfig.LastSnapshotUUID
	if repoConfig.LastSnapshot != nil {
		apiRepo.LastSnapshot = &api.SnapshotResponse{
			UUID:           repoConfig.LastSnapshot.UUID,
			CreatedAt:      repoConfig.LastSnapshot.CreatedAt,
			ContentCounts:  repoConfig.LastSnapshot.ContentCounts,
			AddedCounts:    repoConfig.LastSnapshot.AddedCounts,
			RemovedCounts:  repoConfig.LastSnapshot.RemovedCounts,
			RepositoryPath: repoConfig.LastSnapshot.RepositoryPath,
			RepositoryUUID: repoConfig.UUID,
			RepositoryName: repoConfig.Name,
		}
	}

	apiRepo.LastSnapshotTaskUUID = repoConfig.LastSnapshotTaskUUID
	if repoConfig.LastSnapshotTask != nil {
		apiRepo.LastSnapshotTask = &api.TaskInfoResponse{
			UUID:       repoConfig.LastSnapshotTaskUUID,
			Status:     repoConfig.LastSnapshotTask.Status,
			Typename:   repoConfig.LastSnapshotTask.Typename,
			OrgId:      repoConfig.LastSnapshotTask.OrgId,
			ObjectType: config.ObjectTypeRepository,
			ObjectUUID: repoConfig.UUID,
			ObjectName: repoConfig.Name,
		}
		if repoConfig.LastSnapshotTask.Started != nil {
			apiRepo.LastSnapshotTask.CreatedAt = repoConfig.LastSnapshotTask.Started.Format(time.RFC3339)
		}
		if repoConfig.LastSnapshotTask.Finished != nil {
			apiRepo.LastSnapshotTask.EndedAt = repoConfig.LastSnapshotTask.Finished.Format(time.RFC3339)
		}
		if repoConfig.LastSnapshotTask.Error != nil {
			apiRepo.LastSnapshotTask.Error = *repoConfig.LastSnapshotTask.Error
		}
	}

	if repoConfig.Repository.LastIntrospectionTime != nil {
		apiRepo.LastIntrospectionTime = repoConfig.Repository.LastIntrospectionTime.Format(time.RFC3339)
	}
	if repoConfig.Repository.LastIntrospectionSuccessTime != nil {
		apiRepo.LastIntrospectionSuccessTime = repoConfig.Repository.LastIntrospectionSuccessTime.Format(time.RFC3339)
	}
	if repoConfig.Repository.LastIntrospectionUpdateTime != nil {
		apiRepo.LastIntrospectionUpdateTime = repoConfig.Repository.LastIntrospectionUpdateTime.Format(time.RFC3339)
	}
	if repoConfig.Repository.LastIntrospectionError != nil {
		apiRepo.LastIntrospectionError = *repoConfig.Repository.LastIntrospectionError
	}
}

func ModelToExportRepoApi(model models.RepositoryConfiguration, resp *api.RepositoryExportResponse) {
	resp.URL = model.Repository.URL
	resp.Name = model.Name
	resp.DistributionVersions = model.Versions
	resp.DistributionArch = model.Arch
	resp.GpgKey = model.GpgKey
	resp.MetadataVerification = model.MetadataVerification
	resp.ModuleHotfixes = model.ModuleHotfixes
	resp.Origin = model.Repository.Origin
	resp.Snapshot = model.Snapshot
}

func ModelToImportRepoApi(model models.RepositoryConfiguration, warnings []map[string]interface{}, resp *api.RepositoryImportResponse) {
	ModelToApiFields(model, &resp.RepositoryResponse)
	if warnings != nil {
		resp.Warnings = warnings
	} else {
		resp.Warnings = []map[string]interface{}{}
	}
}

// Converts the database models to our response objects
func convertToResponses(repoConfigs []models.RepositoryConfiguration, pulpContentPath string) []api.RepositoryResponse {
	repos := make([]api.RepositoryResponse, len(repoConfigs))
	for i := 0; i < len(repoConfigs); i++ {
		ModelToApiFields(repoConfigs[i], &repos[i])
		if repoConfigs[i].LastSnapshot != nil {
			repos[i].LastSnapshot.URL = pulpContentURL(pulpContentPath, repos[i].LastSnapshot.RepositoryPath)
			repos[i].LatestSnapshotURL = pulpContentURL(pulpContentPath,
				fmt.Sprintf("%v/%v/%v", strings.Split(repos[i].LastSnapshot.RepositoryPath, "/")[0], repos[i].UUID, "latest"))
		}
	}
	return repos
}

func isTimeout(err error) bool {
	timeout, ok := err.(interface {
		Timeout() bool
	})
	if ok && timeout.Timeout() {
		return true
	}
	return false
}

func (r repositoryConfigDaoImpl) InternalOnly_RefreshRedHatRepo(ctx context.Context, request api.RepositoryRequest, label string) (*api.RepositoryResponse, error) {
	newRepoConfig := models.RepositoryConfiguration{}
	newRepo := models.Repository{}

	request.URL = utils.Ptr(models.CleanupURL(*request.URL))
	ApiFieldsToModel(request, &newRepoConfig, &newRepo)

	newRepoConfig.OrgID = config.RedHatOrg
	newRepoConfig.Label = label
	newRepo.Origin = config.OriginRedHat
	newRepo.Public = true // Ensure all RH repos can be searched

	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "url"}},
		DoUpdates: clause.AssignmentColumns([]string{"origin", "public"})}).Create(&newRepo)
	if result.Error != nil {
		return nil, result.Error
	}

	// If the repo was not updated, we have to load it to get an accurate uuid
	newRepo = models.Repository{}
	result = r.db.WithContext(ctx).Where("URL = ?", request.URL).First(&newRepo)
	if result.Error != nil {
		return nil, result.Error
	}

	newRepoConfig.RepositoryUUID = newRepo.UUID

	result = r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "repository_uuid"}, {Name: "org_id"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Eq{Column: "deleted_at", Value: nil}}},
		DoUpdates:   clause.AssignmentColumns([]string{"name", "arch", "versions", "gpg_key", "label"})}).
		Create(&newRepoConfig)
	if result.Error != nil {
		return nil, result.Error
	}
	var created api.RepositoryResponse
	newRepoConfig.Repository = newRepo
	ModelToApiFields(newRepoConfig, &created)
	return &created, nil
}

func (r repositoryConfigDaoImpl) ValidateParameters(ctx context.Context, orgId string, params api.RepositoryValidationRequest, excludedUUIDS []string) (api.RepositoryValidationResponse, error) {
	var (
		err      error
		response api.RepositoryValidationResponse
	)

	response.Name = api.GenericAttributeValidationResponse{}
	if params.Name == nil {
		response.Name.Skipped = true
	} else {
		err = r.validateName(ctx, orgId, *params.Name, &response.Name, excludedUUIDS)
		if err != nil {
			return response, err
		}
	}

	response.URL = api.UrlValidationResponse{}
	if params.URL == nil {
		response.URL.Skipped = true
	} else {
		url := models.CleanupURL(*params.URL)
		err = r.validateUrl(ctx, orgId, url, &response, excludedUUIDS)
		if err != nil {
			return response, err
		}
		if response.URL.Valid {
			r.yumRepo.Configure(yum.YummySettings{URL: &url, Client: http.DefaultClient})
			r.validateMetadataPresence(ctx, &response)
			if response.URL.MetadataPresent {
				r.checkSignaturePresent(ctx, &params, &response)
			}
		}
	}
	return response, err
}

func (r repositoryConfigDaoImpl) validateName(ctx context.Context, orgId string, name string, response *api.GenericAttributeValidationResponse, excludedUUIDS []string) error {
	if name == "" {
		response.Valid = false
		response.Error = "Name cannot be blank"
		return nil
	}

	found := models.RepositoryConfiguration{}
	query := r.db.WithContext(ctx).Where("name = ? AND ORG_ID = ?", name, orgId)
	if len(excludedUUIDS) != 0 {
		query = query.Where("repository_configurations.uuid NOT IN ?", UuidifyStrings(excludedUUIDS))
	}
	if err := query.Find(&found).Error; err != nil {
		response.Valid = false
		return DBErrorToApi(err)
	}

	if found.UUID != "" {
		response.Valid = false
		response.Error = fmt.Sprintf("A repository with the name '%s' already exists.", name)
		return nil
	}

	response.Valid = true
	return nil
}

func (r repositoryConfigDaoImpl) validateUrl(ctx context.Context, orgId string, url string, response *api.RepositoryValidationResponse, excludedUUIDS []string) error {
	if url == "" {
		response.URL.Valid = false
		response.URL.Error = "URL cannot be blank"
		return nil
	}

	found := models.RepositoryConfiguration{}

	query := r.db.WithContext(ctx).Preload("Repository").Preload("LastSnapshot").Preload("LastSnapshotTask").
		Joins("inner join repositories on repository_configurations.repository_uuid = repositories.uuid").
		Where("Repositories.URL = ? AND ORG_ID = ?", url, orgId)

	if len(excludedUUIDS) != 0 {
		query = query.Where("repository_configurations.uuid NOT IN ?", UuidifyStrings(excludedUUIDS))
	}

	if err := query.Find(&found).Error; err != nil {
		response.URL.Valid = false
		return DBErrorToApi(err)
	}

	if found.UUID != "" {
		response.URL.Valid = false
		response.URL.Error = fmt.Sprintf("A repository with the URL '%s' already exists.", url)
		return nil
	}

	containsWhitespace := strings.ContainsAny(strings.TrimSpace(url), " \t\n\v\r\f")
	if containsWhitespace {
		response.URL.Valid = false
		response.URL.Error = "URL cannot contain whitespace."
		return nil
	}

	response.URL.Valid = true
	return nil
}

func (r repositoryConfigDaoImpl) validateMetadataPresence(ctx context.Context, response *api.RepositoryValidationResponse) {
	_, code, err := r.yumRepo.Repomd(ctx)
	if err != nil {
		response.URL.HTTPCode = code
		if isTimeout(err) {
			response.URL.Error = fmt.Sprintf("Error fetching YUM metadata: %s", "Timeout occurred")
		} else {
			response.URL.Error = fmt.Sprintf("Error fetching YUM metadata: %s", err.Error())
		}
		response.URL.MetadataPresent = false
	} else {
		response.URL.HTTPCode = code
		response.URL.MetadataPresent = code >= 200 && code < 300
	}
}

func (r repositoryConfigDaoImpl) checkSignaturePresent(ctx context.Context, request *api.RepositoryValidationRequest, response *api.RepositoryValidationResponse) {
	if request.GPGKey == nil || *request.GPGKey == "" {
		response.GPGKey.Skipped = true
		response.GPGKey.Valid = true
	} else {
		_, err := LoadGpgKey(request.GPGKey)
		if err == nil {
			response.GPGKey.Valid = true
		} else {
			response.GPGKey.Valid = false
			response.GPGKey.Error = fmt.Sprintf("Error loading GPG Key: %s.  Is this a valid GPG Key?", err.Error())
		}
	}

	sig, _, err := r.yumRepo.Signature(ctx)
	if err != nil || sig == nil {
		response.URL.MetadataSignaturePresent = false
	} else {
		response.URL.MetadataSignaturePresent = true
		if response.GPGKey.Valid && !response.GPGKey.Skipped && request.MetadataVerification { // GPG key is valid & signature present, so validate the signature
			sigErr := ValidateSignature(ctx, r.yumRepo, request.GPGKey)
			if sigErr == nil {
				response.GPGKey.Valid = true
			} else if response.GPGKey.Error == "" {
				response.GPGKey.Valid = false
				response.GPGKey.Error = fmt.Sprintf("Error validating signature: %s. Is this the correct GPG Key?", sigErr.Error())
			}
		}
	}
}

func combineIntrospectionAndSnapshotStatuses(repoConfig *models.RepositoryConfiguration, repo *models.Repository) string {
	// Return introspection status if snapshotting is turned off
	if !repoConfig.Snapshot {
		return repo.LastIntrospectionStatus
	}

	switch repo.LastIntrospectionStatus {
	case config.StatusPending:
		if repoConfig.LastSnapshotTask == nil || repoConfig.LastSnapshotTask.Status == config.TaskStatusRunning || repoConfig.LastSnapshotTask.Status == config.TaskStatusPending || repoConfig.LastSnapshotTask.Status == config.TaskStatusCompleted {
			// Both introspection and snapshot are pending / running or introspection is pending and snapshot has completed
			return config.StatusPending
		}
	case config.StatusUnavailable:
		if repoConfig.LastSnapshotTask == nil {
			return config.StatusUnavailable
		} else if repoConfig.LastSnapshotTask.Status == config.TaskStatusFailed {
			if repoConfig.LastSnapshotUUID == "" {
				// Introspection unavailable, last snapshot failed, and repo has no previous snapshots
				return config.StatusInvalid
			} else {
				// Introspection unavailable, last snapshot failed, and repo has previous snapshots
				return config.StatusUnavailable
			}
		} else {
			return config.StatusUnavailable
		}
	case config.StatusInvalid:
		if repoConfig.LastSnapshotTask == nil {
			return config.StatusInvalid
		} else if repoConfig.LastSnapshotTask.Status == config.TaskStatusCompleted {
			// Introspection failed, snapshot successful
			return config.StatusInvalid
		} else if repoConfig.LastSnapshotTask.Status == config.TaskStatusFailed && repoConfig.LastSnapshotUUID != "" {
			// Both introspection and snapshot failed and repo has previous snapshots
			return config.StatusUnavailable
		}
	case config.StatusValid:
		if repoConfig.LastSnapshotTask == nil {
			return config.StatusValid
		} else if repoConfig.LastSnapshotTask.Status == config.TaskStatusCompleted {
			// Introspection and snapshot successful
			return config.StatusValid
		} else if repoConfig.LastSnapshotTask.Status == config.TaskStatusRunning || repoConfig.LastSnapshotTask.Status == config.TaskStatusPending {
			// Introspection successful, snapshot is running
			return config.StatusPending
		} else if repoConfig.LastSnapshotTask.Status == config.TaskStatusFailed && repoConfig.LastSnapshotUUID != "" {
			// Introspection successful, last snapshot has failed, and repo has previous snapshots
			return config.StatusUnavailable
		} else if repoConfig.LastSnapshotTask.Status == config.TaskStatusFailed && repoConfig.LastSnapshotUUID == "" {
			// Introspection successful, last snapshot failed, and repo has no previous snapshots
			return config.StatusInvalid
		}
	}

	return "Unknown"
}

func LoadGpgKey(gpgKey *string) (openpgp.EntityList, error) {
	var keyRing, entity openpgp.EntityList
	var err error

	gpgKeys, err := readGpgKeys(gpgKey)
	if err != nil {
		return nil, err
	}
	for _, k := range gpgKeys {
		entity, err = openpgp.ReadArmoredKeyRing(strings.NewReader(k))
		if err != nil {
			return nil, err
		}
		keyRing = append(keyRing, entity[0])
	}
	return keyRing, nil
}

// readGpgKeys openpgp.ReadArmoredKeyRing does not correctly parse multiple gpg keys from one file.
// This is a work around that returns a list of gpgKey strings to be passed individually
// to openpgp.ReadArmoredKeyRing
func readGpgKeys(gpgKey *string) ([]string, error) {
	if gpgKey == nil {
		return nil, fmt.Errorf("gpg key cannot be nil")
	}
	const EndGpgKey = "-----END PGP PUBLIC KEY BLOCK-----"
	var gpgKeys []string
	gpgKeyCopy := *gpgKey

	for {
		val := strings.Index(gpgKeyCopy, EndGpgKey)
		if val == -1 {
			break
		}
		gpgKeys = append(gpgKeys, gpgKeyCopy[:val+len(EndGpgKey)])
		gpgKeyCopy = gpgKeyCopy[val+len(EndGpgKey):]
	}

	if len(gpgKeys) == 0 {
		return nil, fmt.Errorf("no gpg key was found")
	}
	return gpgKeys, nil
}

func ValidateSignature(ctx context.Context, repo yum.YumRepository, gpgKey *string) error {
	keyRing, err := LoadGpgKey(gpgKey)
	if err != nil {
		return err
	}

	repomd, _, _ := repo.Repomd(ctx)
	signedFileString := repomd.RepomdString
	sig, _, _ := repo.Signature(ctx)
	_, err = openpgp.CheckArmoredDetachedSignature(keyRing, strings.NewReader(*signedFileString), strings.NewReader(*sig), nil)
	if err != nil {
		return err
	}
	return nil
}
