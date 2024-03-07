package dao

import (
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Repository internal (non-user facing) representation of a repository
type Repository struct {
	UUID                         string
	URL                          string
	Public                       bool
	RepomdChecksum               string
	LastIntrospectionTime        *time.Time
	LastIntrospectionSuccessTime *time.Time
	LastIntrospectionUpdateTime  *time.Time
	LastIntrospectionError       *string
	Status                       string
	PackageCount                 int
	FailedIntrospectionsCount    int
}

// RepositoryUpdate internal representation of repository, nil field value means do not change
type RepositoryUpdate struct {
	UUID                         string
	URL                          *string
	Public                       *bool
	RepomdChecksum               *string
	LastIntrospectionTime        *time.Time
	LastIntrospectionSuccessTime *time.Time
	LastIntrospectionUpdateTime  *time.Time
	LastIntrospectionError       *string
	Status                       *string
	PackageCount                 *int
	FailedIntrospectionsCount    *int
}

func GetRepositoryDao(db *gorm.DB) RepositoryDao {
	return repositoryDaoImpl{
		db: db,
	}
}

type repositoryDaoImpl struct {
	db *gorm.DB
}

func (p repositoryDaoImpl) FetchRepositoryRPMCount(repoUUID string) (int, error) {
	var dbRepos []models.RepositoryRpm
	var count int64 = 0
	result := p.db.Model(&dbRepos).Where("repository_uuid = ?", repoUUID).Count(&count)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(count), nil
}

func (p repositoryDaoImpl) FetchForUrl(url string) (Repository, error) {
	repo := models.Repository{}
	internalRepo := Repository{}
	url = models.CleanupURL(url)
	result := p.db.Where("URL = ?", url).Order("url asc").First(&repo)
	if result.Error != nil {
		return Repository{}, result.Error
	}
	modelToInternal(repo, &internalRepo)
	return internalRepo, nil
}

func (p repositoryDaoImpl) ListForIntrospection(urls *[]string, force bool) ([]Repository, error) {
	var dbRepos []models.Repository
	var repos []Repository
	var repo Repository

	if urls != nil {
		for i, url := range *urls {
			(*urls)[i] = models.CleanupURL(url)
		}
	}

	db := p.db
	if !force && !config.Get().Options.AlwaysRunCronTasks {
		introspectThreshold := time.Now().Add(config.IntrospectTimeInterval * -1) // Add a negative duration
		db = db.Where(
			db.Where("status != ?", config.StatusValid).
				Or("last_introspection_time is NULL").                   // It was never introspected
				Or("last_introspection_time < ?", introspectThreshold)). // It was introspected more than the threshold ago)
			Where( // It is over the introspection limit and has failed once due to being over the limit, so that last_introspection_error says 'over the limit of failed'
				db.Where("failed_introspections_count < ?", config.FailedIntrospectionsLimit+1).
					Or("public = true"))
	}
	if urls != nil {
		db = db.Where("url in ?", *urls)
	}

	result := db.Find(&dbRepos)
	if result.Error != nil {
		return repos, result.Error
	}
	for i := 0; i < len(dbRepos); i++ {
		modelToInternal(dbRepos[i], &repo)
		repos = append(repos, repo)
	}
	return repos, nil
}

func (p repositoryDaoImpl) ListPublic(paginationData api.PaginationData, _ api.FilterData) (api.PublicRepositoryCollectionResponse, int64, error) {
	var dbRepos []models.Repository
	var result *gorm.DB
	var totalRepos int64

	filteredDB := p.db.Where("public = true")

	filteredDB.
		Model(&dbRepos).
		Count(&totalRepos)

	if filteredDB.Error != nil {
		return api.PublicRepositoryCollectionResponse{}, 0, result.Error
	}

	filteredDB.
		Limit(paginationData.Limit).
		Offset(paginationData.Offset).
		Find(&dbRepos)

	if filteredDB.Error != nil {
		return api.PublicRepositoryCollectionResponse{}, 0, result.Error
	}
	repos := make([]api.PublicRepositoryResponse, len(dbRepos))
	for i := 0; i < len(dbRepos); i++ {
		repoModelToPublicRepoApi(dbRepos[i], &repos[i])
	}
	return api.PublicRepositoryCollectionResponse{Data: repos}, totalRepos, nil
}

func (p repositoryDaoImpl) Update(repoIn RepositoryUpdate) error {
	var dbRepo models.Repository

	result := p.db.Where("uuid = ?", repoIn.UUID).First(&dbRepo)
	if result.Error != nil {
		return result.Error
	}

	internalToModel(repoIn, &dbRepo)

	result = p.db.Model(&dbRepo).Updates(dbRepo.MapForUpdate())
	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (r repositoryDaoImpl) OrphanCleanup() error {
	// lookup orphans.  Use unscoped to not try to delete a repo that has a 'soft deleted' repo_config
	query := r.db.Unscoped().Model(&models.Repository{}).
		Joins("left join repository_configurations on repositories.uuid = repository_configurations.repository_uuid").
		Where("repository_configurations.uuid is NULL").
		Where("repositories.public is false").
		Where("repositories.created_at < (LOCALTIMESTAMP - INTERVAL '1 week' ) ").
		Select("repositories.uuid")

	// Delete orphans
	tx := r.db.
		Where("repositories.uuid in (?)", query).
		Delete(&models.Repository{})
	if tx.Error != nil {
		return tx.Error
	}

	log.Debug().Msgf("Cleaned up %v orphaned repositories", tx.RowsAffected)
	return nil
}

// modelToInternal returns internal Repository with fields of model
func modelToInternal(model models.Repository, internal *Repository) {
	internal.UUID = model.UUID
	internal.URL = model.URL
	internal.Public = model.Public
	internal.RepomdChecksum = model.RepomdChecksum
	internal.LastIntrospectionError = model.LastIntrospectionError
	internal.LastIntrospectionTime = model.LastIntrospectionTime
	internal.LastIntrospectionUpdateTime = model.LastIntrospectionUpdateTime
	internal.LastIntrospectionSuccessTime = model.LastIntrospectionSuccessTime
	internal.Status = model.Status
	internal.PackageCount = model.PackageCount
	internal.FailedIntrospectionsCount = model.FailedIntrospectionsCount
}

// internalToModel updates model Repository with fields of internal
func internalToModel(internal RepositoryUpdate, model *models.Repository) {
	if internal.URL != nil {
		model.URL = *internal.URL
	}
	if internal.RepomdChecksum != nil {
		model.RepomdChecksum = *internal.RepomdChecksum
	}
	if internal.Public != nil {
		model.Public = *internal.Public
	}
	if internal.LastIntrospectionError != nil {
		cleaned := strings.ToValidUTF8(*internal.LastIntrospectionError, "")
		model.LastIntrospectionError = &cleaned
	}
	if internal.LastIntrospectionTime != nil {
		model.LastIntrospectionTime = internal.LastIntrospectionTime
	}
	if internal.LastIntrospectionUpdateTime != nil {
		model.LastIntrospectionUpdateTime = internal.LastIntrospectionUpdateTime
	}
	if internal.LastIntrospectionSuccessTime != nil {
		model.LastIntrospectionSuccessTime = internal.LastIntrospectionSuccessTime
	}
	if internal.Status != nil {
		model.Status = *internal.Status
	}
	if internal.PackageCount != nil {
		model.PackageCount = *internal.PackageCount
	}
	if internal.FailedIntrospectionsCount != nil {
		model.FailedIntrospectionsCount = *internal.FailedIntrospectionsCount
	}
}

func repoModelToPublicRepoApi(model models.Repository, resp *api.PublicRepositoryResponse) {
	resp.URL = model.URL
	resp.Status = model.Status
	resp.PackageCount = model.PackageCount
	if model.LastIntrospectionTime != nil {
		resp.LastIntrospectionTime = model.LastIntrospectionTime.Format(time.RFC3339)
	}
	if model.LastIntrospectionSuccessTime != nil {
		resp.LastIntrospectionSuccessTime = model.LastIntrospectionSuccessTime.Format(time.RFC3339)
	}
	if model.LastIntrospectionUpdateTime != nil {
		resp.LastIntrospectionUpdateTime = model.LastIntrospectionUpdateTime.Format(time.RFC3339)
	}
	if model.LastIntrospectionError != nil {
		resp.LastIntrospectionError = *model.LastIntrospectionError
	}
}
