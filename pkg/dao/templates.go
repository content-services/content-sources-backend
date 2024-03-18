package dao

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/jackc/pgx/v5/pgconn"
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

func (t templateDaoImpl) Create(ctx context.Context, reqTemplate api.TemplateRequest) (api.TemplateResponse, error) {
	var resp api.TemplateResponse
	var err error

	_ = t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		DoNothing: true,
	}).Create(&templateRepoConfigs).Error
	if err != nil {
		return t.DBToApiError(err)
	}
	return nil
}

func (t templateDaoImpl) removeTemplateRepoConfigs(tx *gorm.DB, templateUUID string, keepRepoConfigUUIDs []string) error {
	err := tx.Where("template_uuid = ? AND repository_configuration_uuid not in ?", UuidifyString(templateUUID), UuidifyStrings(keepRepoConfigUUIDs)).
		Delete(models.TemplateRepositoryConfiguration{}).Error

	if err != nil {
		return t.DBToApiError(err)
	}
	return nil
}

func (t templateDaoImpl) Fetch(ctx context.Context, orgID string, uuid string) (api.TemplateResponse, error) {
	var respTemplate api.TemplateResponse
	modelTemplate, err := t.fetch(ctx, orgID, uuid)
	if err != nil {
		return api.TemplateResponse{}, err
	}
	templatesModelToApi(modelTemplate, &respTemplate)
	return respTemplate, nil
}

func (t templateDaoImpl) fetch(ctx context.Context, orgID string, uuid string) (models.Template, error) {
	var modelTemplate models.Template
	err := t.db.WithContext(ctx).
		Where("uuid = ? AND org_id = ?", UuidifyString(uuid), orgID).
		Preload("RepositoryConfigurations").First(&modelTemplate).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return modelTemplate, &ce.DaoError{NotFound: true, Message: "Could not find template with UUID " + uuid}
		}
		return modelTemplate, t.DBToApiError(err)
	}
	return modelTemplate, nil
}

func (t templateDaoImpl) Update(ctx context.Context, orgID string, uuid string, templParams api.TemplateUpdateRequest) (api.TemplateResponse, error) {
	var resp api.TemplateResponse
	var err error

	err = t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return t.update(ctx, tx, orgID, uuid, templParams)
	})

	if err != nil {
		return resp, fmt.Errorf("could not update template %w", err)
	}

	resp, err = t.Fetch(ctx, orgID, uuid)
	if err != nil {
		return resp, fmt.Errorf("could not fetch template %w", err)
	}

	event.SendTemplateEvent(orgID, event.TemplateUpdated, []api.TemplateResponse{resp})

	return resp, err
}

func (t templateDaoImpl) update(ctx context.Context, tx *gorm.DB, orgID string, uuid string, templParams api.TemplateUpdateRequest) error {
	dbTempl, err := t.fetch(ctx, orgID, uuid)
	if err != nil {
		return err
	}

	templatesUpdateApiToModel(templParams, &dbTempl)

	if err := tx.Model(&models.Template{}).Where("uuid = ?", UuidifyString(uuid)).Updates(dbTempl.MapForUpdate()).Error; err != nil {
		return DBErrorToApi(err)
	}
	if templParams.RepositoryUUIDS != nil {
		if err := t.validateRepositoryUUIDs(orgID, templParams.RepositoryUUIDS); err != nil {
			return err
		}
		err = t.removeTemplateRepoConfigs(tx, uuid, templParams.RepositoryUUIDS)
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

func (t templateDaoImpl) List(ctx context.Context, orgID string, paginationData api.PaginationData, filterData api.TemplateFilterData) (api.TemplateCollectionResponse, int64, error) {
	var totalTemplates int64
	templates := make([]models.Template, 0)

	filteredDB := t.filteredDbForList(orgID, t.db.WithContext(ctx), filterData)

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

func (t templateDaoImpl) SoftDelete(ctx context.Context, orgID string, uuid string) error {
	var modelTemplate models.Template

	err := t.db.WithContext(ctx).Where("uuid = ? AND org_id = ?", UuidifyString(uuid), orgID).First(&modelTemplate).Error
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

func (t templateDaoImpl) Delete(ctx context.Context, orgID string, uuid string) error {
	var modelTemplate models.Template

	err := t.db.WithContext(ctx).Unscoped().Where("uuid = ? AND org_id = ?", UuidifyString(uuid), orgID).First(&modelTemplate).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ce.DaoError{NotFound: true, Message: "Could not find template with UUID " + uuid}
		}
		return t.DBToApiError(err)
	}

	if err = t.db.WithContext(ctx).Unscoped().Delete(&modelTemplate).Error; err != nil {
		return err
	}

	return nil
}

func (t templateDaoImpl) ClearDeletedAt(ctx context.Context, orgID string, uuid string) error {
	var modelTemplate models.Template

	err := t.db.WithContext(ctx).Unscoped().Where("uuid = ? AND org_id = ?", UuidifyString(uuid), orgID).First(&modelTemplate).Error
	if err != nil {
		return err
	}

	err = t.db.WithContext(ctx).Unscoped().Model(&modelTemplate).Update("deleted_at", nil).Error
	if err != nil {
		return err
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
