package dao

import (
	"context"
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
	LastIntrospectionStatus      string
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
	LastIntrospectionStatus      *string
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

func (p repositoryDaoImpl) FetchRepositoryRPMCount(ctx context.Context, repoUUID string) (int, error) {
	var dbRepos []models.RepositoryRpm
	var count int64 = 0
	result := p.db.WithContext(ctx).Model(&dbRepos).Where("repository_uuid = ?", repoUUID).Count(&count)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(count), nil
}

func (p repositoryDaoImpl) FetchForUrl(ctx context.Context, url string) (Repository, error) {
	repo := models.Repository{}
	internalRepo := Repository{}
	url = models.CleanupURL(url)
	result := p.db.WithContext(ctx).Where("URL = ?", url).Order("url asc").First(&repo)
	if result.Error != nil {
		return Repository{}, result.Error
	}
	modelToInternal(repo, &internalRepo)
	return internalRepo, nil
}

func (p repositoryDaoImpl) ListForIntrospection(ctx context.Context, urls *[]string, force bool) ([]Repository, error) {
	var dbRepos []models.Repository
	var repos []Repository
	var repo Repository

	if urls != nil {
		for i, url := range *urls {
			(*urls)[i] = models.CleanupURL(url)
		}
	}

	db := p.db.WithContext(ctx)
	if !force && !config.Get().Options.AlwaysRunCronTasks {
		introspectThreshold := time.Now().Add(config.IntrospectTimeInterval * -1) // Add a negative duration
		db = db.Where(
			db.Where("last_introspection_status != ?", config.StatusValid).
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

func (p repositoryDaoImpl) ListPublic(ctx context.Context, paginationData api.PaginationData, _ api.FilterData) (api.PublicRepositoryCollectionResponse, int64, error) {
	var dbRepos []models.Repository
	var result *gorm.DB
	var totalRepos int64

	filteredDB := p.db.WithContext(ctx).Where("public = true")

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

func (p repositoryDaoImpl) Update(ctx context.Context, repoIn RepositoryUpdate) error {
	var dbRepo models.Repository

	result := p.db.WithContext(ctx).Where("uuid = ?", repoIn.UUID).First(&dbRepo)
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

func (r repositoryDaoImpl) OrphanCleanup(ctx context.Context) error {
	// lookup orphans.  Use unscoped to not try to delete a repo that has a 'soft deleted' repo_config
	query := r.db.WithContext(ctx).Unscoped().Model(&models.Repository{}).
		Joins("left join repository_configurations on repositories.uuid = repository_configurations.repository_uuid").
		Where("repository_configurations.uuid is NULL").
		Where("repositories.public is false").
		Where("repositories.created_at < (LOCALTIMESTAMP - INTERVAL '1 week' ) ").
		Select("repositories.uuid")

	// Delete orphans
	tx := r.db.WithContext(ctx).
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
	internal.LastIntrospectionStatus = model.LastIntrospectionStatus
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
	if internal.LastIntrospectionStatus != nil {
		model.LastIntrospectionStatus = *internal.LastIntrospectionStatus
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
	resp.PackageCount = model.PackageCount
	resp.LastIntrospectionStatus = model.LastIntrospectionStatus
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
