package dao

import (
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

// Repository internal (non-user facing) representation of a repository
type Repository struct {
	UUID     string
	URL      string
	Revision string
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
	result := p.db.Where("URL = ?", url).First(&repo)
	if result.Error != nil {
		return result.Error, Repository{}
	}
	return nil, Repository{
		UUID:     repo.UUID,
		URL:      repo.URL,
		Revision: repo.Revision,
	}
}

func (p repositoryDaoImpl) List() (error, []Repository) {
	var dbRepos []models.Repository
	var repos []Repository
	result := p.db.Find(&dbRepos)
	if result.Error != nil {
		return result.Error, repos
	}
	for i := 0; i < len(dbRepos); i++ {
		repos = append(repos, Repository{
			UUID:     dbRepos[i].UUID,
			URL:      dbRepos[i].URL,
			Revision: dbRepos[i].Revision,
		})
	}
	return nil, repos
}

func (p repositoryDaoImpl) Update(repoIn Repository) error {
	var repo models.Repository

	result := p.db.Where("uuid = ?", repoIn.UUID).First(&repo)
	if result.Error != nil {
		return result.Error
	}

	repo.URL = repoIn.URL
	repo.Revision = repoIn.Revision

	result = p.db.Model(&repo).Updates(repo.MapForUpdate())
	if result.Error != nil {
		return result.Error
	}

	return nil
}
