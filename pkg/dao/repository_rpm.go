package dao

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
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
		return DBErrorToApi(err)
	}
	if repo.ReferRepoConfig == nil || *repo.ReferRepoConfig == "" {
		return fmt.Errorf("The referenced repository configuration uuid can not be an empty string")
	}

	// Retrieve the related RepositoryConfiguration record
	var repoConfig models.RepositoryConfiguration
	if err := r.db.Model(&models.RepositoryConfiguration{}).Where("uuid = ? AND org_id = ? AND account_id = ?", *repo.ReferRepoConfig, OrgID, AccountId).First(&repoConfig).Error; err != nil {
		return DBErrorToApi(err)
	}

	// Now that the tenant has been verified, we can create the record
	if err := r.db.Create(&newRepoRpm).Error; err != nil {
		return DBErrorToApi(err)
	}
	return nil
}

func (r repositoryRpmDaoImpl) List(OrgID string, AccountId string, uuidRepo string, limit int, offset int) (api.RepositoryRpmCollectionResponse, int64, error) {
	// Check arguments
	if OrgID == "" {
		return api.RepositoryRpmCollectionResponse{}, 0, fmt.Errorf("OrgID can not be an empty string")
	}
	if AccountId == "" {
		return api.RepositoryRpmCollectionResponse{}, 0, fmt.Errorf("AccountId can not be an empty string")
	}

	var totalRpms int64
	repoRpms := []models.RepositoryRpm{}

	result := r.db.
		// Model(models.RepositoryRpm{}).
		Table("repository_rpms").
		Where("refer_repo = ?", uuidRepo).
		Count(&totalRpms).
		Offset(offset).
		Limit(limit).
		Scan(&repoRpms)
	if result.Error != nil {
		return api.RepositoryRpmCollectionResponse{}, totalRpms, result.Error
	}

	repoRpmResponse := RepositoryRpmListFromModelToResponse(repoRpms)
	return api.RepositoryRpmCollectionResponse{
		Data: repoRpmResponse,
		Meta: api.ResponseMetadata{
			Count:  totalRpms,
			Offset: offset,
			Limit:  limit,
		},
	}, totalRpms, nil
}

func RepositoryRpmListFromModelToResponse(repoRpm []models.RepositoryRpm) []api.RepositoryRpm {
	repos := make([]api.RepositoryRpm, len(repoRpm))
	for i := 0; i < len(repoRpm); i++ {
		repos[i].CopyFromModel(&repoRpm[i])
	}
	return repos
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
		return nil, DBErrorToApi(err)
	}
	if repoRpm.ReferRepo == "" {
		return nil, fmt.Errorf("The referenced repo can not be empty")
	}

	// Retrieve the repository that the package belong to
	repo := &models.Repository{}
	if err := r.db.Model(repo).Where("uuid = ?", repoRpm.ReferRepo).First(repo).Error; err != nil {
		return nil, DBErrorToApi(err)
	}

	// Retrieve the RepositoryConfig that the repository belong to
	repoConfig := &models.RepositoryConfiguration{}
	if err := r.db.Model(repoConfig).Where("uuid = ?", repo.ReferRepoConfig).First(repoConfig).Error; err != nil {
		return nil, DBErrorToApi(err)
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
