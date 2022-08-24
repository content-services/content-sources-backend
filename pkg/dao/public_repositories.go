package dao

import (
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

type PublicRepository struct {
	UUID     string
	URL      string
	Revision string
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
	result := p.db.Where("URL = ?", url).First(&repo)
	if result.Error != nil {
		return result.Error, PublicRepository{}
	}
	return nil, PublicRepository{
		UUID:     repo.UUID,
		URL:      repo.URL,
		Revision: repo.Revision,
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

func (p publicRepositoryDaoImpl) UpdateRepository(pubRepo PublicRepository) error {
	var repo models.Repository

	result := p.db.Where("uuid = ?", pubRepo.UUID).First(&repo)
	if result.Error != nil {
		return result.Error
	}

	repo.URL = pubRepo.URL
	repo.Revision = pubRepo.Revision

	result = p.db.Model(&repo).Updates(repo.MapForUpdate())
	if result.Error != nil {
		return result.Error
	}

	return nil
}
