package dao

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
)

type repositoryDaoImpl struct {
}

func GetRepositoryDao() repositoryDaoImpl {
	return repositoryDaoImpl{}
}

const uniqueConstraintErrMsg = "ERROR: duplicate key value violates unique constraint \"url_and_org_id_unique\" (SQLSTATE 23505)"

func (r repositoryDaoImpl) Create(newRepo api.CreateRepository) error {

	newRepoConfig := models.RepositoryConfiguration{
		Name:      newRepo.Name,
		URL:       newRepo.URL,
		Version:   newRepo.DistributionVersion,
		Arch:      newRepo.DistributionArch,
		AccountID: newRepo.AccountID,
		OrgID:     newRepo.OrgID,
	}

	dest := models.RepositoryConfiguration{}
	result := db.DB.Where("org_id = ? AND url = ?", newRepo.OrgID, newRepo.URL).Find(&dest)
	if result.Error != nil {
		return result.Error
	}
	if err := db.DB.Create(&newRepoConfig).Error; err != nil {
		if err.Error() == uniqueConstraintErrMsg {
			return fmt.Errorf("repository with this URL already belongs to organization")
		}
		return err
	}
	return nil
}
