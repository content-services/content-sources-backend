package dao

import (
	"time"

	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Repository internal (non-user facing) representation of a repository
type Repository struct {
	UUID                         string
	URL                          string
	RepomdChecksum               string
	LastIntrospectionTime        *time.Time
	LastIntrospectionSuccessTime *time.Time
	LastIntrospectionUpdateTime  *time.Time
	LastIntrospectionError       *string
	Status                       string
	PackageCount                 int
}

// RepositoryUpdate internal representation of repository, nil field value means do not change
type RepositoryUpdate struct {
	UUID                         string
	URL                          *string
	RepomdChecksum               *string
	LastIntrospectionTime        *time.Time
	LastIntrospectionSuccessTime *time.Time
	LastIntrospectionUpdateTime  *time.Time
	LastIntrospectionError       *string
	Status                       *string
	PackageCount                 *int
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

func (p repositoryDaoImpl) List() ([]Repository, error) {
	var dbRepos []models.Repository
	var repos []Repository
	var repo Repository
	result := p.db.Find(&dbRepos)
	if result.Error != nil {
		return repos, result.Error
	}
	for i := 0; i < len(dbRepos); i++ {
		modelToInternal(dbRepos[i], &repo)
		repos = append(repos, repo)
	}
	return repos, nil
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
	// lookup orphans
	query := r.db.Model(&models.Repository{}).
		Joins("left join repository_configurations on repositories.uuid = repository_configurations.repository_uuid").
		Where("repository_configurations.uuid is NULL").
		Where("repositories.public is false").
		Where("repositories.created_at < (LOCALTIMESTAMP - INTERVAL '1 week' ) ").
		Select("repositories.uuid")

	// Delete orphans
	tx := r.db.
		Unscoped().
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
	internal.RepomdChecksum = model.RepomdChecksum
	internal.LastIntrospectionError = model.LastIntrospectionError
	internal.LastIntrospectionTime = model.LastIntrospectionTime
	internal.LastIntrospectionUpdateTime = model.LastIntrospectionUpdateTime
	internal.LastIntrospectionSuccessTime = model.LastIntrospectionSuccessTime
	internal.Status = model.Status
	internal.PackageCount = model.PackageCount
}

// internalToModel updates model Repository with fields of internal
func internalToModel(internal RepositoryUpdate, model *models.Repository) {
	if internal.URL != nil {
		model.URL = *internal.URL
	}
	if internal.RepomdChecksum != nil {
		model.RepomdChecksum = *internal.RepomdChecksum
	}
	if internal.LastIntrospectionError != nil {
		model.LastIntrospectionError = internal.LastIntrospectionError
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
}
