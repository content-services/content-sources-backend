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

func (r rpmDaoImpl) isOwnedRepository(orgID string, repoUUID string) (bool, error) {
	var repoConfigs []models.RepositoryConfiguration
	var count int64
	if err := r.db.
		Where("org_id = ? and repository_uuid = ?", orgID, repoUUID).
		Find(&repoConfigs).
		Count(&count).
		Error; err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}
	return true, nil
}

// Create a record in rpms table, and the relation between it
// and the Repository record in the repositories_rpms table.
// Before create the record, it checks that the provided
// Repository register is related with a RepositoryConfiguration
// that belong to the indicated organization.
// orgID It is used to check the repository which we are trying
// to add the Rpm record belongs to the indicated organization.
// repo The repository record that the new rpm record will
// belong to.
// newRpm The Rpm record to be created into the database.
// Return error if something goes wrong, else nil.
func (r rpmDaoImpl) Create(orgID string, repo *models.Repository, newRpm *models.Rpm) error {
	// Check arguments
	if repo == nil {
		return fmt.Errorf("repo can not be nil")
	}
	if newRpm == nil {
		return fmt.Errorf("newRpm can not be nil")
	}

	if ok, err := r.isOwnedRepository(orgID, repo.UUID); !ok {
		if err != nil {
			return DBErrorToApi(err)
		}
		return fmt.Errorf("repository_uuid = %s is not owned", repo.UUID)
	}

	// Add Rpm record
	if err := r.db.Create(newRpm).Error; err != nil {
		return DBErrorToApi(err)
	}

	// Add to repositories_rpms the entry to relate
	// the rpm with the repository it belongs to
	var repositories_rpms models.RepositoriesRpms = models.RepositoriesRpms{
		RepositoryUUID: repo.UUID,
		RpmUUID:        newRpm.UUID,
	}
	if err := r.db.Create(&repositories_rpms).Error; err != nil {
		return DBErrorToApi(err)
	}

	return nil
}

func (r rpmDaoImpl) List(orgID string, uuidRepo string, limit int, offset int) (api.RepositoryRpmCollectionResponse, int64, error) {
	// Check arguments
	if orgID == "" {
		return api.RepositoryRpmCollectionResponse{}, 0, fmt.Errorf("orgID can not be an empty string")
	}

	var totalRpms int64
	repoRpms := []models.Rpm{}

	if ok, err := r.isOwnedRepository(orgID, uuidRepo); !ok {
		if err != nil {
			return api.RepositoryRpmCollectionResponse{},
				totalRpms,
				DBErrorToApi(err)
		}
		return api.RepositoryRpmCollectionResponse{},
			totalRpms,
			fmt.Errorf("repository_uuid = %s is not owned", uuidRepo)
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

	repoRpmResponse := r.RepositoryRpmListFromModelToResponse(repoRpms)
	return api.RepositoryRpmCollectionResponse{
		Data: repoRpmResponse,
		Meta: api.ResponseMetadata{
			Count:  totalRpms,
			Offset: offset,
			Limit:  limit,
		},
	}, totalRpms, nil
}

func (r rpmDaoImpl) RepositoryRpmListFromModelToResponse(repoRpm []models.Rpm) []api.RepositoryRpm {
	repos := make([]api.RepositoryRpm, len(repoRpm))
	for i := 0; i < len(repoRpm); i++ {
		// repos[i].CopyFromModel(&repoRpm[i])
		r.modelToApiFields(&repoRpm[i], &repos[i])
	}
	return repos
}

// Fetch retrieve a RPM that belongs identified by rpmUUUID.
// OrgId The organization id for the current request.
// AccountId The account number for the current request.
// rpmUUID The rpm id in the database.
func (r rpmDaoImpl) Fetch(OrgID string, rpmUUID string) (*api.RepositoryRpm, error) {
	// Check arguments
	if OrgID == "" {
		return nil, fmt.Errorf("OrgID can not be an empty string")
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
		Where("org_id = ? AND repository_uuid in ?", OrgID, listUUID).
		Find(&repoConfigData).
		Error; err != nil {
		return nil, DBErrorToApi(err)
	}

	// Map data to the api response
	var apiData api.RepositoryRpm
	r.modelToApiFields(&rpmData, &apiData)
	return &apiData, nil
}

// apiFieldsToModel transform from database model to API request.
// in the source models.Rpm structure.
// out the output api.RepositoryRpm structure.
//
// NOTE: This encapsulate transformation into rpmDaoImpl implementation
// as the methods are not used outside; if they were used
// out of this place, decouple into a new struct and make
// he methods publics.
func (r rpmDaoImpl) modelToApiFields(in *models.Rpm, out *api.RepositoryRpm) {
	if in == nil || out == nil {
		return
	}
	out.UUID = in.Base.UUID
	out.Name = in.Name
	out.Arch = in.Arch
	out.Version = in.Version
	out.Release = in.Release
	out.Epoch = in.Epoch
	out.Summary = in.Summary
	out.Checksum = in.Checksum
}
