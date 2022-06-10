package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/jackc/pgconn"
)

type repositoryDaoImpl struct {
}

func GetRepositoryDao() RepositoryDao {
	return repositoryDaoImpl{}
}

func (r repositoryDaoImpl) Create(newRepo api.RepositoryRequest) error {
	newRepoConfig := models.RepositoryConfiguration{
		AccountID: *newRepo.AccountID,
		OrgID:     *newRepo.OrgID,
	}
	ApiFieldsToModel(&newRepo, &newRepoConfig)

	if err := db.DB.Create(&newRepoConfig).Error; err != nil {
		if isUniqueViolation(err) {
			return &Error{BadValidation: true, Message: "RepositoryResponse with this URL already belongs to organization "}
		}
		return err
	}
	return nil
}

func (r repositoryDaoImpl) Fetch(orgId string, uuid string) api.RepositoryResponse {
	repo := api.RepositoryResponse{}
	repo.FromRepositoryConfiguration(r.fetchRepoConfig(orgId, uuid))
	return repo
}

func (r repositoryDaoImpl) fetchRepoConfig(orgId string, uuid string) models.RepositoryConfiguration {
	found := models.RepositoryConfiguration{}
	db.DB.Where("UUID = ? AND ORG_ID = ?", uuid, orgId).First(&found)
	return found
}

func isUniqueViolation(err error) bool {
	pgError, ok := err.(*pgconn.PgError)
	if ok {
		if pgError.Code == "23505" {
			return true
		}
	}
	return false
}

func (r repositoryDaoImpl) Update(orgId string, uuid string, repoParams api.RepositoryRequest) error {
	repoConfig := r.fetchRepoConfig(orgId, uuid)
	if repoConfig.UUID == "" {
		return &Error{NotFound: true, Message: "Could not find RepositoryResponse with uuid " + uuid}
	}
	ApiFieldsToModel(&repoParams, &repoConfig)
	result := db.DB.Model(&repoConfig).Updates(repoConfig.MapForUpdate())
	if result.Error != nil {
		return &Error{Message: result.Error.Error(), BadValidation: isUniqueViolation(result.Error)}
	}
	return nil
}

func ApiFieldsToModel(apiRepo *api.RepositoryRequest, repoConfig *models.RepositoryConfiguration) {
	if apiRepo.Name != nil {
		repoConfig.Name = *apiRepo.Name
	}
	if apiRepo.URL != nil {
		repoConfig.URL = *apiRepo.URL
	}
	if apiRepo.DistributionArch != nil {
		repoConfig.Arch = *apiRepo.DistributionArch
	}
	if apiRepo.DistributionVersions != nil {
		repoConfig.Versions = *apiRepo.DistributionVersions
	}
}
