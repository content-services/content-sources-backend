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

	// Select all the rpms from a repository
	if err := r.db.
		Model(&repoRpms).
		Joins(strings.Join([]string{"inner join", models.TableNameRpmsRepositories, "on uuid = rpm_uuid"}, " ")).
		Where("repository_uuid = ?", uuidRepo).
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
