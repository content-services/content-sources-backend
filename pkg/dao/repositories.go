package dao

import (
	"time"

	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

// Repository internal (non-user facing) representation of a repository
type Repository struct {
	UUID                         string
	URL                          string
	Revision                     string
	Public                       bool
	LastIntrospectionTime        *time.Time
	LastIntrospectionSuccessTime *time.Time
	LastIntrospectionUpdateTime  *time.Time
	LastIntrospectionError       *string
	Status                       string
}

func GetRepositoryDao(db *gorm.DB) RepositoryDao {
	return repositoryDaoImpl{
		db: db,
	}
}

type repositoryDaoImpl struct {
	db *gorm.DB
}

func (p repositoryDaoImpl) FetchForUrl(url string) (error, Repository) {
	repo := models.Repository{}
	result := p.db.Where("URL = ?", url).Order("url asc").First(&repo)
	if result.Error != nil {
		return result.Error, Repository{}
	}
	return nil, modelToInternal(repo)
}

func (p repositoryDaoImpl) List() (error, []Repository) {
	var dbRepos []models.Repository
	var repos []Repository
	result := p.db.Find(&dbRepos)
	if result.Error != nil {
		return result.Error, repos
	}
	for i := 0; i < len(dbRepos); i++ {
		repos = append(repos, modelToInternal(dbRepos[i]))
	}
	return nil, repos
}

func (p repositoryDaoImpl) Update(repoIn Repository) error {
	var dbRepo models.Repository

	result := p.db.Where("uuid = ?", repoIn.UUID).First(&dbRepo)
	if result.Error != nil {
		return result.Error
	}

	repo := internalToModel(repoIn)

	result = p.db.Model(&repo).Updates(repo.MapForUpdate())
	if result.Error != nil {
		return result.Error
	}

	return nil
}

// modelToInternal returns internal Repository with fields of model
func modelToInternal(model models.Repository) Repository {
	var internal Repository
	internal.UUID = model.UUID
	internal.URL = model.URL
	internal.Public = model.Public
	internal.Revision = model.Revision
	internal.LastIntrospectionError = model.LastIntrospectionError
	internal.LastIntrospectionTime = model.LastIntrospectionTime
	internal.LastIntrospectionUpdateTime = model.LastIntrospectionUpdateTime
	internal.LastIntrospectionSuccessTime = model.LastIntrospectionSuccessTime
	internal.Status = model.Status
	return internal
}

// internalToModel returns model Repository with fields of internal
func internalToModel(internal Repository) models.Repository {
	var model models.Repository
	model.UUID = internal.UUID
	model.URL = internal.URL
	model.Public = internal.Public
	model.Revision = internal.Revision
	model.LastIntrospectionError = internal.LastIntrospectionError
	model.LastIntrospectionTime = internal.LastIntrospectionTime
	model.LastIntrospectionUpdateTime = internal.LastIntrospectionUpdateTime
	model.LastIntrospectionSuccessTime = internal.LastIntrospectionSuccessTime
	model.Status = internal.Status
	return model
}
