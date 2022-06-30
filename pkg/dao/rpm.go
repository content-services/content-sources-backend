package dao

import (
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

type rpmDaoImpl struct {
	db *gorm.DB
}

func GetRpmDao(db *gorm.DB) RpmDao {
	return rpmDaoImpl{
		db: db,
	}
}

func (r rpmDaoImpl) isOwnedRepository(orgID string, accountID string, repoUUID string) error {
	var repoConfigs []models.RepositoryConfiguration
	if err := r.db.
		Where("org_id = ? and account_id = ? and repository_uuid = ?", orgID, accountID, repoUUID).
		Find(&repoConfigs).
		Error; err != nil {
		return err
	}
	return nil
}

func (r rpmDaoImpl) Create(orgID string, accountID string, repo *models.Repository, newRpm *models.Rpm) error {
	// Check arguments
	if orgID == "" {
		return fmt.Errorf("orgID can not be an empty string")
	}
	if accountID == "" {
		return fmt.Errorf("accountID can not be an empty string")
	}
	if repo == nil {
		return fmt.Errorf("repo can not be nil")
	}
	if newRpm == nil {
		return fmt.Errorf("newRpm can not be nil")
	}

	if err := r.isOwnedRepository(orgID, accountID, repo.UUID); err != nil {
		return DBErrorToApi(err)
	}

	// Add Rpm record
	if err := r.db.Create(newRpm).Error; err != nil {
		return DBErrorToApi(err)
	}

	// Add at repositories_rpms the entry to relate to
	var repositories_rpms []map[string]interface{} = []map[string]interface{}{
		{
			"repository_uuid": repo.UUID,
			"rpm_uuid":        newRpm.UUID,
		},
	}
	if err := r.db.Table(models.TableNameRpmsRepositories).Create(&repositories_rpms).Error; err != nil {
		return DBErrorToApi(err)
	}

	return nil
}

func (r rpmDaoImpl) List(orgID string, accountID string, uuidRepo string, limit int, offset int) (api.RepositoryRpmCollectionResponse, int64, error) {
	// Check arguments
	if orgID == "" {
		return api.RepositoryRpmCollectionResponse{}, 0, fmt.Errorf("orgID can not be an empty string")
	}
	if accountID == "" {
		return api.RepositoryRpmCollectionResponse{}, 0, fmt.Errorf("accountID can not be an empty string")
	}

	var totalRpms int64
	repoRpms := []models.Rpm{}

	if err := r.isOwnedRepository(orgID, accountID, uuidRepo); err != nil {
		return api.RepositoryRpmCollectionResponse{}, totalRpms, DBErrorToApi(err)
	}

	//
	if err := r.db.
		Model(&repoRpms).
		Joins(strings.Join([]string{"left join", models.TableNameRpmsRepositories, "on uuid = rpm_uuid"}, " ")).
		Where("repository_uuid = ?", uuidRepo).
		Count(&totalRpms).
		Offset(offset).
		Limit(limit).
		Find(&repoRpms).
		Error; err != nil {
		return api.RepositoryRpmCollectionResponse{}, totalRpms, err
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

func RepositoryRpmListFromModelToResponse(repoRpm []models.Rpm) []api.RepositoryRpm {
	repos := make([]api.RepositoryRpm, len(repoRpm))
	for i := 0; i < len(repoRpm); i++ {
		repos[i].CopyFromModel(&repoRpm[i])
	}
	return repos
}

// Fetch retrieve a RPM that belongs identified by rpmUUUID.
// OrgId The organization id for the current request.
// AccountId The account number for the current request.
// rpmUUID The rpm id in the database.
func (r rpmDaoImpl) Fetch(OrgID string, AccountID string, rpmUUID string) (*api.RepositoryRpm, error) {
	// Check arguments
	if OrgID == "" {
		return nil, fmt.Errorf("OrgID can not be an empty string")
	}
	if AccountID == "" {
		return nil, fmt.Errorf("AccountID can not be an empty string")
	}
	if rpmUUID == "" {
		return nil, fmt.Errorf("uuid can not be an empty string")
	}

	// Read the RPM record
	var rpmData models.Rpm = models.Rpm{
		Base: models.Base{
			UUID: rpmUUID,
		},
	}
	if err := r.db.Preload("Repositories").First(&rpmData).Error; err != nil {
		return nil, DBErrorToApi(err)
	}

	// Prepare list of Repositories.UUID
	var listUUID []string = make([]string, len(rpmData.Repositories))
	for i, item := range rpmData.Repositories {
		listUUID[i] = item.Base.UUID
	}

	// Check that any has a valid RepositoryConfiguration
	var repoConfigData []models.RepositoryConfiguration
	if err := r.db.
		Where("org_id = ? and account_id = ?", OrgID, AccountID).
		Find(&repoConfigData, listUUID).
		Error; err != nil {
		return nil, DBErrorToApi(err)
	}

	// Map data to the api response
	return &api.RepositoryRpm{
		UUID:        rpmData.Base.UUID,
		Name:        rpmData.Name,
		Arch:        rpmData.Arch,
		Version:     rpmData.Version,
		Release:     rpmData.Release,
		Epoch:       rpmData.Epoch,
		Summary:     rpmData.Summary,
		Description: rpmData.Description,
	}, nil
}
