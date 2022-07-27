package dao

import (
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

type PublicRepository struct {
	UUID string
	URL  string
}

func GetPublicRepositoryDao(db *gorm.DB) PublicRepositoryDao {
	return publicRepositoryDaoImpl{
		db: db,
	}
}

type publicRepositoryDaoImpl struct {
	db *gorm.DB
}

func (p publicRepositoryDaoImpl) FetchForUrl(url string) (error, PublicRepository) {
	repo := models.Repository{}
	result := p.db.Where("public = true and URL = ?", url).First(&repo)
	if result.Error != nil {
		return result.Error, PublicRepository{}
	}
	return nil, PublicRepository{
		UUID: repo.UUID,
		URL:  repo.URL,
	}
}

func (p publicRepositoryDaoImpl) List() (error, []PublicRepository) {
	var repos []models.Repository
	var publicRepos []PublicRepository
	result := p.db.Where("public = true").Find(&repos)
	if result.Error != nil {
		return result.Error, publicRepos
	}
	for i := 0; i < len(repos); i++ {
		publicRepos = append(publicRepos, PublicRepository{
			UUID: repos[i].UUID,
			URL:  repos[i].URL,
		})
	}
	return nil, publicRepos
}
