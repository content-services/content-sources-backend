package dao

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/openlyinc/pointy"
	"gorm.io/gorm"
)

type repositoryRpmDaoImpl struct {
	db *gorm.DB
}

func GetRepositoryRpmDao(db *gorm.DB) repositoryRpmDaoImpl {
	return repositoryRpmDaoImpl{
		db: db,
	}
}

func (r repositoryRpmDaoImpl) Create(OrgID string, AccountId string, newRepoRpm *models.RepositoryRpm) error {
	// Check arguments
	if OrgID == "" {
		return fmt.Errorf("OrgID can not be an empty string")
	}
	if AccountId == "" {
		return fmt.Errorf("AccountId can not be an empty string")
	}
	if newRepoRpm == nil {
		return fmt.Errorf("It can not create a nil RepositoryRpm record")
	}
	if newRepoRpm.ReferRepo == "" {
		return fmt.Errorf("The referenced repository uuid can not be an empty string")
	}

	// Retrieve the related Repository record
	var repo models.Repository
	if err := r.db.Model(&models.Repository{}).Where("uuid = ?", newRepoRpm.ReferRepo).First(&repo).Error; err != nil {
		return err
	}
	if repo.ReferRepoConfig == nil || *repo.ReferRepoConfig == "" {
		return fmt.Errorf("The referenced repository configuration uuid can not be an empty string")
	}

	// Retrieve the related RepositoryConfiguration record
	var repoConfig models.RepositoryConfiguration
	if err := db.DB.Model(&models.RepositoryConfiguration{}).Where("uuid = ? AND org_id = ? AND account_id = ?", *repo.ReferRepoConfig, OrgID, AccountId).First(&repoConfig).Error; err != nil {
		return err
	}

	// Now that the tenant has been verified, we can create the record
	if err := db.DB.Create(&newRepoRpm).Error; err != nil {
		if isUniqueViolation(err) {
			return &Error{BadValidation: true, Message: "Repository with this URL already belongs to organization "}
		}
		return err
	}
	return nil
}

func (r repositoryRpmDaoImpl) List(OrgID string, AccountId string, uuidRepo string, limit int, offset int) (api.RepositoryCollectionResponse, int64, error) {
	// Check arguments
	if OrgID == "" {
		return api.RepositoryCollectionResponse{}, 0, fmt.Errorf("OrgID can not be an empty string")
	}
	if AccountId == "" {
		return api.RepositoryCollectionResponse{}, 0, fmt.Errorf("AccountId can not be an empty string")
	}

	var totalRepos int64
	repoConfigs := make([]models.RepositoryConfiguration, 0)

	result := db.DB.Where("org_id = ?", OrgID).Find(&repoConfigs).Count(&totalRepos)
	if result.Error != nil {
		return api.RepositoryCollectionResponse{}, totalRepos, result.Error
	}

	result = db.DB.Where("org_id = ?", OrgID).Limit(limit).Offset(offset).Find(&repoConfigs)
	if result.Error != nil {
		return api.RepositoryCollectionResponse{}, totalRepos, result.Error
	}

	repos := convertToResponses(repoConfigs)
	return api.RepositoryCollectionResponse{Data: repos}, totalRepos, nil
}

func (r repositoryRpmDaoImpl) Fetch(OrgID string, AccountId string, uuid string) (*api.RepositoryRpm, error) {
	// Check arguments
	if OrgID == "" {
		return nil, fmt.Errorf("OrgID can not be an empty string")
	}
	if AccountId == "" {
		return nil, fmt.Errorf("AccountId can not be an empty string")
	}
	if uuid == "" {
		return nil, fmt.Errorf("uuid can not be an empty string")
	}

	// Retrieve RepositoryRpm record
	repoRpm := &models.RepositoryRpm{}
	if err := r.db.Model(repoRpm).Where("uuid = ?", uuid).First(repoRpm).Error; err != nil {
		return nil, err
	}
	if repoRpm.ReferRepo == "" {
		return nil, fmt.Errorf("The referenced repo can not be empty")
	}

	// Retrieve the repository that the package belong to
	repo := &models.Repository{}
	if err := r.db.Model(repo).Where("uuid = ?", repoRpm.ReferRepo).First(repo).Error; err != nil {
		return nil, err
	}

	// Retrieve the RepositoryConfig that the repository belong to
	repoConfig := &models.RepositoryConfiguration{}
	if err := r.db.Model(repoConfig).Where("uuid = ?", repo.ReferRepoConfig).First(repoConfig).Error; err != nil {
		return nil, err
	}

	var epoch *int32
	if repoRpm.Epoch != nil {
		epoch = pointy.Int32(*repoRpm.Epoch)
	}
	return &api.RepositoryRpm{
		UUID:        repoRpm.Base.UUID,
		Name:        repoRpm.Name,
		Arch:        repoRpm.Arch,
		Version:     repoRpm.Version,
		Release:     repoRpm.Release,
		Epoch:       epoch,
		Summary:     repoRpm.Summary,
		Description: repoRpm.Description,
		ReferRepo:   repoRpm.ReferRepo,
	}, nil
}

func (r repositoryRpmDaoImpl) fetchRepoConfig(orgID string, uuid string) (models.RepositoryConfiguration, error) {
	found := models.RepositoryConfiguration{}
	result := db.DB.Where("UUID = ? AND ORG_ID = ?", uuid, orgID).First(&found)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return found, &Error{NotFound: true, Message: "Could not find repository with UUID " + uuid}
		} else {
			return found, result.Error
		}
	}
	return found, nil
}

func (r repositoryRpmDaoImpl) Update(orgID string, uuid string, repoParams api.RepositoryRequest) error {
	repoConfig, err := r.fetchRepoConfig(orgID, uuid)
	if err != nil {
		return err
	}
	ApiFieldsToModel(&repoParams, &repoConfig)
	result := db.DB.Model(&repoConfig).Updates(repoConfig.MapForUpdate())
	if result.Error != nil {
		return &Error{Message: result.Error.Error(), BadValidation: isUniqueViolation(result.Error)}
	}
	return nil
}

func (r repositoryRpmDaoImpl) Delete(orgID string, uuid string) error {
	repoConfig := models.RepositoryConfiguration{}
	if err := db.DB.Debug().Where("UUID = ? AND ORG_ID = ?", uuid, orgID).First(&repoConfig).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &Error{NotFound: true, Message: "Could not find repository with UUID " + uuid}
		} else {
			return err
		}
	}
	if err := db.DB.Delete(&repoConfig).Error; err != nil {
		return err
	}
	return nil
}

// func FromApiToModel(api *api.RepositoryRpm, model *models.RepositoryRpm) error {
// 	if api == nil {
// 		return fmt.Errorf("Can not map from a nil *api.RepositoryRpm")
// 	}
// 	if model == nil {
// 		return fmt.Errorf("Can not map to a nil *models.RepositoryRpm")
// 	}

// 	// TODO Add here the mapping

// 	return nil
// }

// func FromModelToApi(mode *models.RepositoryRpm, api *api.RepositoryRpm) error {
// 	return nil
// }

// func FieldsToModel(apiRepo *api.RepositoryRequest, repoConfig *models.RepositoryConfiguration) {
// 	if apiRepo.Name != nil {
// 		repoConfig.Name = *apiRepo.Name
// 	}
// 	if apiRepo.URL != nil {
// 		repoConfig.URL = *apiRepo.URL
// 	}
// 	if apiRepo.DistributionArch != nil {
// 		repoConfig.Arch = *apiRepo.DistributionArch
// 	}
// 	if apiRepo.DistributionVersions != nil {
// 		repoConfig.Versions = *apiRepo.DistributionVersions
// 	}
// }

// func ModelToApiFields(repoConfig models.RepositoryConfiguration, apiRepo *api.RepositoryResponse) {
// 	apiRepo.UUID = repoConfig.UUID
// 	apiRepo.Name = repoConfig.Name
// 	apiRepo.URL = repoConfig.URL
// 	apiRepo.DistributionVersions = repoConfig.Versions
// 	apiRepo.DistributionArch = repoConfig.Arch
// 	apiRepo.AccountID = repoConfig.AccountID
// 	apiRepo.OrgID = repoConfig.OrgID
// }

// Converts the database models to our response objects
// func convertToResponses(repoConfigs []models.RepositoryConfiguration) []api.RepositoryResponse {
// 	repos := make([]api.RepositoryResponse, len(repoConfigs))
// 	for i := 0; i < len(repoConfigs); i++ {
// 		ModelToApiFields(repoConfigs[i], &repos[i])
// 	}
// 	return repos
// }
