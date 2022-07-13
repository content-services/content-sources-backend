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

func (r rpmDaoImpl) isOwnedRepository(orgID string, repositoryConfigUUID string) (bool, error) {
	var repoConfigs []models.RepositoryConfiguration
	var count int64
	if err := r.db.
		Where("org_id = ? and uuid = ?", orgID, repositoryConfigUUID).
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

func (r rpmDaoImpl) List(orgID string, repositoryConfigUUID string, limit int, offset int) (api.RepositoryRpmCollectionResponse, int64, error) {
	// Check arguments
	if orgID == "" {
		return api.RepositoryRpmCollectionResponse{}, 0, fmt.Errorf("orgID can not be an empty string")
	}

	var totalRpms int64
	repoRpms := []models.Rpm{}

	if ok, err := r.isOwnedRepository(orgID, repositoryConfigUUID); !ok {
		if err != nil {
			return api.RepositoryRpmCollectionResponse{},
				totalRpms,
				DBErrorToApi(err)
		}
		return api.RepositoryRpmCollectionResponse{},
			totalRpms,
			fmt.Errorf("repositoryConfigUUID = %s is not owned", repositoryConfigUUID)
	}

	repositoryConfig := models.RepositoryConfiguration{}
	// Select Repository from RepositoryConfig
	if err := r.db.
		Preload("Repository").
		Find(&repositoryConfig, "uuid = ?", repositoryConfigUUID).
		Error; err != nil {
		return api.RepositoryRpmCollectionResponse{}, totalRpms, err
	}
	if err := r.db.
		Model(&repoRpms).
		Joins(strings.Join([]string{"inner join", models.TableNameRpmsRepositories, "on uuid = rpm_uuid"}, " ")).
		Where("repository_uuid = ?", repositoryConfig.Repository.UUID).
		Count(&totalRpms).
		Offset(offset).
		Limit(limit).
		Find(&repoRpms).
		Error; err != nil {
		return api.RepositoryRpmCollectionResponse{}, totalRpms, err
	}

	// Return the rpm list
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
		r.modelToApiFields(&repoRpm[i], &repos[i])
	}
	return repos
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

func (r rpmDaoImpl) Search(orgID string, request api.SearchRpmRequest, limit int) ([]api.SearchRpmResponse, error) {
	// Retrieve the repository id list
	if orgID == "" {
		return nil, fmt.Errorf("orgID can not be an empty string")
	}
	if len(request.URLs) == 0 {
		return nil, fmt.Errorf("request.URLs must contain at least 1 URL")
	}

	// This implement the following SELECT statement:
	//
	// SELECT DISTINCT ON (rpms.name)
	//        rpms.name, rpms.summary
	// FROM rpms
	//      inner join repositories_rpms on repositories_rpms.rpm_uuid = rpms.uuid
	//      inner join repositories on repositories.uuid = repositories_rpms.repository_uuid
	//      inner join repository_configurations on repository_configurations.repository_uuid = repositories.uuid
	// WHERE repository_configurations.org_id = 'acme'
	//       AND repositories.public
	//       AND rpms.name LIKE 'demo%'
	// ORDER BY rpms.name, rpms.epoch DESC
	// LIMIT 20;

	// https://github.com/go-gorm/gorm/issues/5318
	dataResponse := []api.SearchRpmResponse{}
	db := r.db.
		Select("DISTINCT ON(rpms.name) rpms.name as package_name", "rpms.summary").
		Table(models.TableNameRpm).
		Joins("inner join repositories_rpms on repositories_rpms.rpm_uuid = rpms.uuid").
		Joins("inner join repositories on repositories.uuid = repositories_rpms.repository_uuid").
		Joins("inner join repository_configurations on repository_configurations.repository_uuid = repositories.uuid").
		Where("repository_configurations.org_id = ?", orgID).
		Where("repositories.public").
		Where("rpms.name LIKE ?", fmt.Sprintf("%s%%", request.Query)).
		Where("repositories.url in ?", request.URLs).
		Order("rpms.name ASC").
		Order("rpms.epoch DESC").
		Limit(limit).
		Scan(&dataResponse)

	if db.Error != nil {
		return nil, db.Error
	}

	return dataResponse, nil
}
