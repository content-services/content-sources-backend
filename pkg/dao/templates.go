package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
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
			case "name_org_id_unique":
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

	templateRepoConfigs := make([]models.TemplateRepositoryConfiguration, len(reqTemplate.RepositoryUUIDS))
	for i, repo := range reqTemplate.RepositoryUUIDS {
		templateRepoConfigs[i].TemplateUUID = modelTemplate.UUID
		templateRepoConfigs[i].RepositoryConfigurationUUID = repo
	}

	err = tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "template_uuid"}, {Name: "repository_configuration_uuid"}},
		DoNothing: true,
	}).Create(&templateRepoConfigs).Error
	if err != nil {
		return api.TemplateResponse{}, t.DBToApiError(err)
	}

	templatesModelToApi(modelTemplate, &respTemplate)

	return respTemplate, nil
}

func (t templateDaoImpl) Fetch(orgID string, uuid string) (api.TemplateResponse, error) {
	var modelTemplate models.Template
	var respTemplate api.TemplateResponse

	err := t.db.Where("uuid = ? AND org_id = ?", UuidifyString(uuid), orgID).First(&modelTemplate).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return api.TemplateResponse{}, &ce.DaoError{NotFound: true, Message: "Could not find template with UUID " + uuid}
		}
		return api.TemplateResponse{}, t.DBToApiError(err)
	}

	templatesModelToApi(modelTemplate, &respTemplate)
	return respTemplate, nil
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

	return nil
}

func (t templateDaoImpl) Delete(orgID string, uuid string) error {
	template := models.Template{Base: models.Base{UUID: uuid}, OrgID: orgID}

	if err := t.db.Unscoped().Delete(&template).Error; err != nil {
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

func templatesModelToApi(model models.Template, api *api.TemplateResponse) {
	api.UUID = model.UUID
	api.OrgID = model.OrgID
	api.Name = model.Name
	api.Description = model.Description
	api.Version = model.Version
	api.Arch = model.Arch
	api.Date = model.Date
}

func templatesConvertToResponses(templates []models.Template) []api.TemplateResponse {
	responses := make([]api.TemplateResponse, len(templates))
	for i := 0; i < len(templates); i++ {
		templatesModelToApi(templates[i], &responses[i])
	}
	return responses
}
