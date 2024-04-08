package dao

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type templateDaoImpl struct {
	db *gorm.DB
}

func (t templateDaoImpl) DBToApiError(e error) *ce.DaoError {
	var dupKeyName string
	if e == nil {
		return nil
	}

	pgError, ok := e.(*pgconn.PgError)
	if ok {
		if pgError.Code == "23505" {
			switch pgError.ConstraintName {
			case "name_org_id_not_deleted_unique":
				dupKeyName = "name"
			}
			return &ce.DaoError{BadValidation: true, Message: "Template with this " + dupKeyName + " already belongs to organization"}
		}
		if pgError.Code == "22021" {
			return &ce.DaoError{BadValidation: true, Message: "Request parameters contain invalid syntax"}
		}
	}
	dbError, ok := e.(models.Error)
	if ok {
		return &ce.DaoError{BadValidation: dbError.Validation, Message: dbError.Message}
	}

	return &ce.DaoError{
		Message:  e.Error(),
		NotFound: ce.HttpCodeForDaoError(e) == 404, // Check if isNotFoundError
	}
}

func (t templateDaoImpl) Create(reqTemplate api.TemplateRequest) (api.TemplateResponse, error) {
	var resp api.TemplateResponse
	var err error

	_ = t.db.Transaction(func(tx *gorm.DB) error {
		resp, err = t.create(tx, reqTemplate)
		return err
	})
	return resp, err
}

func (t templateDaoImpl) create(tx *gorm.DB, reqTemplate api.TemplateRequest) (api.TemplateResponse, error) {
	var modelTemplate models.Template
	var respTemplate api.TemplateResponse

	if err := t.validateRepositoryUUIDs(*reqTemplate.OrgID, reqTemplate.RepositoryUUIDS); err != nil {
		return api.TemplateResponse{}, err
	}

	// Create a template
	templatesApiToModel(reqTemplate, &modelTemplate)
	err := tx.Create(&modelTemplate).Error
	if err != nil {
		return api.TemplateResponse{}, t.DBToApiError(err)
	}

	// Associate the template to repositories
	if reqTemplate.RepositoryUUIDS == nil {
		return api.TemplateResponse{}, ce.DaoError{
			Message:       "template must include repository uuids",
			BadValidation: true,
		}
	}

	err = t.insertTemplateRepoConfigs(tx, modelTemplate.UUID, reqTemplate.RepositoryUUIDS)
	if err != nil {
		return api.TemplateResponse{}, err
	}

	log.Info().Msg("Create: insert into template repo configs")

	templatesModelToApi(modelTemplate, &respTemplate)
	respTemplate.RepositoryUUIDS = reqTemplate.RepositoryUUIDS

	event.SendTemplateEvent(*reqTemplate.OrgID, event.TemplateCreated, []api.TemplateResponse{respTemplate})

	return respTemplate, nil
}

func (t templateDaoImpl) validateRepositoryUUIDs(orgId string, uuids []string) error {
	var count int64
	resp := t.db.Model(models.RepositoryConfiguration{}).Where("org_id = ? or org_id = ?", orgId, config.RedHatOrg).Where("uuid in ?", UuidifyStrings(uuids)).Count(&count)
	if resp.Error != nil {
		return fmt.Errorf("could not query repository uuids: %w", resp.Error)
	}
	if count != int64(len(uuids)) {
		return &ce.DaoError{BadValidation: true, Message: "One or more Repository UUIDs was invalid."}
	}
	return nil
}

func (t templateDaoImpl) insertTemplateRepoConfigs(tx *gorm.DB, templateUUID string, repoUUIDs []string) error {
	templateRepoConfigs := make([]models.TemplateRepositoryConfiguration, len(repoUUIDs))
	for i, repo := range repoUUIDs {
		templateRepoConfigs[i].TemplateUUID = templateUUID
		templateRepoConfigs[i].RepositoryConfigurationUUID = repo
	}

	err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "template_uuid"}, {Name: "repository_configuration_uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{"deleted_at"}),
	}).Create(&templateRepoConfigs).Error
	if err != nil {
		return t.DBToApiError(err)
	}
	return nil
}

func (t templateDaoImpl) DeleteTemplateRepoConfigs(templateUUID string, keepRepoConfigUUIDs []string) error {
	err := t.db.Unscoped().Where("template_uuid = ? AND repository_configuration_uuid not in ?", UuidifyString(templateUUID), UuidifyStrings(keepRepoConfigUUIDs)).
		Delete(models.TemplateRepositoryConfiguration{}).Error

	if err != nil {
		return t.DBToApiError(err)
	}
	return nil
}

func (t templateDaoImpl) softDeleteTemplateRepoConfigs(tx *gorm.DB, templateUUID string, keepRepoConfigUUIDs []string) error {
	err := tx.Debug().Where("template_uuid = ? AND repository_configuration_uuid not in ?", UuidifyString(templateUUID), UuidifyStrings(keepRepoConfigUUIDs)).
		Delete(&models.TemplateRepositoryConfiguration{}).Error

	if err != nil {
		return t.DBToApiError(err)
	}
	return nil
}

func (t templateDaoImpl) Fetch(orgID string, uuid string) (api.TemplateResponse, error) {
	var respTemplate api.TemplateResponse
	modelTemplate, err := t.fetch(orgID, uuid)
	if err != nil {
		return api.TemplateResponse{}, err
	}
	templatesModelToApi(modelTemplate, &respTemplate)
	return respTemplate, nil
}

func (t templateDaoImpl) fetch(orgID string, uuid string) (models.Template, error) {
	var modelTemplate models.Template
	err := t.db.Where("uuid = ? AND org_id = ?", UuidifyString(uuid), orgID).Preload("RepositoryConfigurations").First(&modelTemplate).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return modelTemplate, &ce.DaoError{NotFound: true, Message: "Could not find template with UUID " + uuid}
		}
		return modelTemplate, t.DBToApiError(err)
	}
	return modelTemplate, nil
}

func (t templateDaoImpl) Update(orgID string, uuid string, templParams api.TemplateUpdateRequest) (api.TemplateResponse, error) {
	var resp api.TemplateResponse
	var err error

	err = t.db.Transaction(func(tx *gorm.DB) error {
		return t.update(tx, orgID, uuid, templParams)
	})

	if err != nil {
		return resp, fmt.Errorf("could not update template %w", err)
	}

	resp, err = t.Fetch(orgID, uuid)
	if err != nil {
		return resp, fmt.Errorf("could not fetch template %w", err)
	}

	event.SendTemplateEvent(orgID, event.TemplateUpdated, []api.TemplateResponse{resp})

	return resp, err
}

func (t templateDaoImpl) update(tx *gorm.DB, orgID string, uuid string, templParams api.TemplateUpdateRequest) error {
	dbTempl, err := t.fetch(orgID, uuid)
	if err != nil {
		return err
	}

	templatesUpdateApiToModel(templParams, &dbTempl)

	if err := tx.Model(&models.Template{}).Debug().Where("uuid = ?", UuidifyString(uuid)).Updates(dbTempl.MapForUpdate()).Error; err != nil {
		return DBErrorToApi(err)
	}

	var existingRepoConfigUUIDs []string
	if err := tx.Model(&models.TemplateRepositoryConfiguration{}).Select("repository_configuration_uuid").Where("template_uuid = ?", dbTempl.UUID).Find(&existingRepoConfigUUIDs).Error; err != nil {
		return DBErrorToApi(err)
	}

	if templParams.RepositoryUUIDS != nil {
		if err := t.validateRepositoryUUIDs(orgID, templParams.RepositoryUUIDS); err != nil {
			return err
		}

		err = t.softDeleteTemplateRepoConfigs(tx, uuid, templParams.RepositoryUUIDS)
		if err != nil {
			return fmt.Errorf("could not remove uneeded template repositories %w", err)
		}

		err = t.insertTemplateRepoConfigs(tx, uuid, templParams.RepositoryUUIDS)
		if err != nil {
			return fmt.Errorf("could not insert new template repositories %w", err)
		}
	}
	return nil
}

func (t templateDaoImpl) List(orgID string, paginationData api.PaginationData, filterData api.TemplateFilterData) (api.TemplateCollectionResponse, int64, error) {
	var totalTemplates int64
	templates := make([]models.Template, 0)

	filteredDB := t.filteredDbForList(orgID, t.db, filterData)

	sortMap := map[string]string{
		"name":    "name",
		"url":     "url",
		"arch":    "arch",
		"version": "version",
	}

	order := convertSortByToSQL(paginationData.SortBy, sortMap, "name asc")

	// Get count
	filteredDB.
		Model(&templates).
		Count(&totalTemplates)

	if filteredDB.Error != nil {
		return api.TemplateCollectionResponse{}, totalTemplates, filteredDB.Error
	}

	filteredDB.
		Preload("RepositoryConfigurations").
		Order(order).
		Limit(paginationData.Limit).
		Offset(paginationData.Offset).
		Find(&templates)

	if filteredDB.Error != nil {
		return api.TemplateCollectionResponse{}, totalTemplates, filteredDB.Error
	}

	responses := templatesConvertToResponses(templates)

	return api.TemplateCollectionResponse{Data: responses}, totalTemplates, nil
}

func (t templateDaoImpl) filteredDbForList(orgID string, filteredDB *gorm.DB, filterData api.TemplateFilterData) *gorm.DB {
	filteredDB = filteredDB.Where("org_id = ? ", orgID)

	if filterData.Name != "" {
		filteredDB = filteredDB.Where("name = ?", filterData.Name)
	}
	if filterData.Arch != "" {
		filteredDB = filteredDB.Where("arch = ?", filterData.Arch)
	}
	if filterData.Version != "" {
		filteredDB = filteredDB.Where("version = ?", filterData.Version)
	}
	if filterData.Search != "" {
		containsSearch := "%" + filterData.Search + "%"
		filteredDB = filteredDB.
			Where("name LIKE ?", containsSearch)
	}
	return filteredDB
}

func (t templateDaoImpl) SoftDelete(orgID string, uuid string) error {
	var modelTemplate models.Template

	err := t.db.Where("uuid = ? AND org_id = ?", UuidifyString(uuid), orgID).First(&modelTemplate).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ce.DaoError{NotFound: true, Message: "Could not find template with UUID " + uuid}
		}
		return t.DBToApiError(err)
	}

	if err = t.db.Delete(&modelTemplate).Error; err != nil {
		return err
	}

	var resp api.TemplateResponse
	templatesModelToApi(modelTemplate, &resp)
	event.SendTemplateEvent(orgID, event.TemplateDeleted, []api.TemplateResponse{resp})

	return nil
}

func (t templateDaoImpl) Delete(orgID string, uuid string) error {
	var modelTemplate models.Template

	err := t.db.Unscoped().Where("uuid = ? AND org_id = ?", UuidifyString(uuid), orgID).First(&modelTemplate).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ce.DaoError{NotFound: true, Message: "Could not find template with UUID " + uuid}
		}
		return t.DBToApiError(err)
	}

	if err = t.db.Unscoped().Delete(&modelTemplate).Error; err != nil {
		return err
	}

	return nil
}

func (t templateDaoImpl) ClearDeletedAt(orgID string, uuid string) error {
	var modelTemplate models.Template

	err := t.db.Unscoped().Where("uuid = ? AND org_id = ?", UuidifyString(uuid), orgID).First(&modelTemplate).Error
	if err != nil {
		return err
	}

	err = t.db.Unscoped().Model(&modelTemplate).Update("deleted_at", nil).Error
	if err != nil {
		return err
	}

	return nil
}

// GetRepoChanges given a template UUID and a slice of repo config uuids, returns the added/removed/unchanged/all between the existing and given repos
func (t templateDaoImpl) GetRepoChanges(templateUUID string, newRepoConfigUUIDs []string) ([]string, []string, []string, []string, error) {
	var templateRepoConfigs []models.TemplateRepositoryConfiguration
	if err := t.db.Model(&models.TemplateRepositoryConfiguration{}).Unscoped().Where("template_uuid = ?", templateUUID).Find(&templateRepoConfigs).Error; err != nil {
		return nil, nil, nil, nil, t.DBToApiError(err)
	}

	// if the repo is being added, it's in the request and the distribution_href is nil
	// if the repo is already part of the template, it's in request and distribution_href is not nil
	// if the repo is being removed, it's not in request but is in the table
	var added, unchanged, removed, all []string
	for _, v := range templateRepoConfigs {
		if v.DistributionHref == "" && slices.Contains(newRepoConfigUUIDs, v.RepositoryConfigurationUUID) {
			added = append(added, v.RepositoryConfigurationUUID)
		} else if slices.Contains(newRepoConfigUUIDs, v.RepositoryConfigurationUUID) {
			unchanged = append(unchanged, v.RepositoryConfigurationUUID)
		} else {
			removed = append(removed, v.RepositoryConfigurationUUID)
		}
		all = append(all, v.RepositoryConfigurationUUID)
	}

	return added, removed, unchanged, all, nil
}

func (t templateDaoImpl) GetDistributionHref(templateUUID string, repoConfigUUID string) (string, error) {
	var distributionHref string
	err := t.db.Model(&models.TemplateRepositoryConfiguration{}).Select("distribution_href").Unscoped().Where("template_uuid = ? AND repository_configuration_uuid =  ?", templateUUID, repoConfigUUID).Find(&distributionHref).Error
	if err != nil {
		return "", err
	}
	return distributionHref, nil
}

func (t templateDaoImpl) UpdateDistributionHrefs(templateUUID string, repoUUIDs []string, repoDistributionMap map[string]string) error {
	templateRepoConfigs := make([]models.TemplateRepositoryConfiguration, len(repoUUIDs))
	for i, repo := range repoUUIDs {
		templateRepoConfigs[i].TemplateUUID = templateUUID
		templateRepoConfigs[i].RepositoryConfigurationUUID = repo
		if repoDistributionMap != nil {
			templateRepoConfigs[i].DistributionHref = repoDistributionMap[repo]
		}
	}

	err := t.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "template_uuid"}, {Name: "repository_configuration_uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{"distribution_href"}),
	}).Create(&templateRepoConfigs).Error
	if err != nil {
		return t.DBToApiError(err)
	}
	return nil
}

func templatesApiToModel(api api.TemplateRequest, model *models.Template) {
	if api.Name != nil {
		model.Name = *api.Name
	}
	if api.Description != nil {
		model.Description = *api.Description
	}
	if api.Version != nil {
		model.Version = *api.Version
	}
	if api.Arch != nil {
		model.Arch = *api.Arch
	}
	if api.Date != nil {
		model.Date = *api.Date
	}
	if api.OrgID != nil {
		model.OrgID = *api.OrgID
	}
}

func templatesUpdateApiToModel(api api.TemplateUpdateRequest, model *models.Template) {
	if api.Description != nil {
		model.Description = *api.Description
	}
	if api.Date != nil {
		model.Date = *api.Date
	}
	if api.OrgID != nil {
		model.OrgID = *api.OrgID
	}
}

func templatesModelToApi(model models.Template, api *api.TemplateResponse) {
	api.UUID = model.UUID
	api.OrgID = model.OrgID
	api.Name = model.Name
	api.Description = model.Description
	api.Version = model.Version
	api.Arch = model.Arch
	api.Date = model.Date
	api.RepositoryUUIDS = make([]string, 0) // prevent null responses
	for _, repoConfig := range model.RepositoryConfigurations {
		api.RepositoryUUIDS = append(api.RepositoryUUIDS, repoConfig.UUID)
	}
}

func templatesConvertToResponses(templates []models.Template) []api.TemplateResponse {
	responses := make([]api.TemplateResponse, len(templates))
	for i := 0; i < len(templates); i++ {
		templatesModelToApi(templates[i], &responses[i])
	}
	return responses
}
