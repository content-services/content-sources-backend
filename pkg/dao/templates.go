package dao

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/jackc/pgx/v5/pgconn"
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
			return &ce.DaoError{AlreadyExists: true, Message: "Template with this " + dupKeyName + " already belongs to organization"}
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
		resp, err = t.create(ctx, tx, reqTemplate)
		return err
	})
	return resp, err
}

func (t templateDaoImpl) create(ctx context.Context, tx *gorm.DB, reqTemplate api.TemplateRequest) (api.TemplateResponse, error) {
	var modelTemplate models.Template
	var respTemplate api.TemplateResponse

	if err := t.validateRepositoryUUIDs(ctx, *reqTemplate.OrgID, reqTemplate.RepositoryUUIDS); err != nil {
		return api.TemplateResponse{}, err
	}

	// Create a template
	templatesCreateApiToModel(reqTemplate, &modelTemplate)
	err := tx.Create(&modelTemplate).Error
	if err != nil {
		return api.TemplateResponse{}, t.DBToApiError(err)
	}

	// Associate the template to repositories
	if reqTemplate.RepositoryUUIDS == nil {
		return api.TemplateResponse{}, &ce.DaoError{
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

func (t templateDaoImpl) validateRepositoryUUIDs(ctx context.Context, orgId string, uuids []string) error {
	var count int64
	resp := t.db.WithContext(ctx).Model(models.RepositoryConfiguration{}).Where("org_id = ? or org_id = ?", orgId, config.RedHatOrg).Where("uuid in ?", UuidifyStrings(uuids)).Count(&count)
	if resp.Error != nil {
		return fmt.Errorf("could not query repository uuids: %w", resp.Error)
	}
	if count != int64(len(uuids)) {
		return &ce.DaoError{NotFound: true, Message: "One or more Repository UUIDs was invalid."}
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

func (t templateDaoImpl) DeleteTemplateRepoConfigs(ctx context.Context, templateUUID string, keepRepoConfigUUIDs []string) error {
	err := t.db.WithContext(ctx).Unscoped().Where("template_uuid = ? AND repository_configuration_uuid not in ?", UuidifyString(templateUUID), UuidifyStrings(keepRepoConfigUUIDs)).
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

func (t templateDaoImpl) Fetch(ctx context.Context, orgID string, uuid string, includeSoftDel bool) (api.TemplateResponse, error) {
	var respTemplate api.TemplateResponse
	modelTemplate, err := t.fetch(ctx, orgID, uuid, includeSoftDel)
	if err != nil {
		return api.TemplateResponse{}, err
	}
	templatesModelToApi(modelTemplate, &respTemplate)
	return respTemplate, nil
}

func (t templateDaoImpl) fetch(ctx context.Context, orgID string, uuid string, includeSoftDel bool) (models.Template, error) {
	var modelTemplate models.Template
	query := t.db.WithContext(ctx)
	if includeSoftDel {
		query = query.Unscoped()
	}
	err := query.Where("uuid = ? AND org_id = ?", UuidifyString(uuid), orgID).
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
		return resp, err
	}

	resp, err = t.Fetch(ctx, orgID, uuid, false)
	if err != nil {
		return resp, err
	}

	event.SendTemplateEvent(orgID, event.TemplateUpdated, []api.TemplateResponse{resp})

	return resp, err
}

func (t templateDaoImpl) update(ctx context.Context, tx *gorm.DB, orgID string, uuid string, templParams api.TemplateUpdateRequest) error {
	dbTempl, err := t.fetch(ctx, orgID, uuid, false)
	if err != nil {
		return err
	}

	templatesUpdateApiToModel(templParams, &dbTempl)

	// copy fields to validate before updating the template
	validateTemplate := models.Template{
		Name:      dbTempl.Name,
		OrgID:     dbTempl.OrgID,
		Arch:      dbTempl.Arch,
		UseLatest: dbTempl.UseLatest,
		Version:   dbTempl.Version,
		Date:      dbTempl.Date,
	}

	if err := tx.Model(&validateTemplate).Where("uuid = ?", UuidifyString(uuid)).Updates(dbTempl.MapForUpdate()).Error; err != nil {
		return DBErrorToApi(err)
	}

	var existingRepoConfigUUIDs []string
	if err := tx.Model(&models.TemplateRepositoryConfiguration{}).Select("repository_configuration_uuid").Where("template_uuid = ?", dbTempl.UUID).Find(&existingRepoConfigUUIDs).Error; err != nil {
		return DBErrorToApi(err)
	}

	if templParams.RepositoryUUIDS != nil {
		if err := t.validateRepositoryUUIDs(ctx, orgID, templParams.RepositoryUUIDS); err != nil {
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
	if filteredDB.
		Model(&templates).
		Distinct("uuid").
		Count(&totalTemplates).Error != nil {
		return api.TemplateCollectionResponse{}, totalTemplates, t.DBToApiError(filteredDB.Error)
	}

	if filteredDB.
		Distinct("templates.*").
		Preload("RepositoryConfigurations").
		Order(order).
		Limit(paginationData.Limit).
		Offset(paginationData.Offset).
		Find(&templates).Error != nil {
		return api.TemplateCollectionResponse{}, totalTemplates, t.DBToApiError(filteredDB.Error)
	}

	responses := templatesConvertToResponses(templates)

	return api.TemplateCollectionResponse{Data: responses}, totalTemplates, nil
}

func (t templateDaoImpl) InternalOnlyFetchByName(ctx context.Context, name string) (models.Template, error) {
	var modelTemplate models.Template
	err := t.db.WithContext(ctx).
		Where("name = ? ", name).
		Preload("RepositoryConfigurations").First(&modelTemplate).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return modelTemplate, &ce.DaoError{NotFound: true, Message: "Could not find template with name " + name}
		}
		return modelTemplate, t.DBToApiError(err)
	}
	return modelTemplate, nil
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
	if len(filterData.RepositoryUUIDs) > 0 {
		filteredDB = filteredDB.Joins("INNER JOIN templates_repository_configurations on templates_repository_configurations.template_uuid = templates.uuid").
			Where("templates_repository_configurations.repository_configuration_uuid in ?", UuidifyStrings(filterData.RepositoryUUIDs))
	}
	if filterData.UseLatest {
		filteredDB = filteredDB.Where("use_latest = ?", filterData.UseLatest)
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

	if err = t.db.WithContext(ctx).Delete(&modelTemplate).Error; err != nil {
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

// GetRepoChanges given a template UUID and a slice of repo config uuids, returns the added/removed/unchanged/all between the existing and given repos
func (t templateDaoImpl) GetRepoChanges(ctx context.Context, templateUUID string, newRepoConfigUUIDs []string) ([]string, []string, []string, []string, error) {
	var templateRepoConfigs []models.TemplateRepositoryConfiguration
	if err := t.db.WithContext(ctx).Model(&models.TemplateRepositoryConfiguration{}).Unscoped().Where("template_uuid = ?", templateUUID).Find(&templateRepoConfigs).Error; err != nil {
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

func (t templateDaoImpl) GetDistributionHref(ctx context.Context, templateUUID string, repoConfigUUID string) (string, error) {
	var distributionHref string
	err := t.db.WithContext(ctx).Model(&models.TemplateRepositoryConfiguration{}).Select("distribution_href").Unscoped().Where("template_uuid = ? AND repository_configuration_uuid =  ?", templateUUID, repoConfigUUID).Find(&distributionHref).Error
	if err != nil {
		return "", err
	}
	return distributionHref, nil
}

func (t templateDaoImpl) UpdateDistributionHrefs(ctx context.Context, templateUUID string, repoUUIDs []string, repoDistributionMap map[string]string) error {
	templateRepoConfigs := make([]models.TemplateRepositoryConfiguration, len(repoUUIDs))
	for i, repo := range repoUUIDs {
		templateRepoConfigs[i].TemplateUUID = templateUUID
		templateRepoConfigs[i].RepositoryConfigurationUUID = repo
		if repoDistributionMap != nil {
			templateRepoConfigs[i].DistributionHref = repoDistributionMap[repo]
		}
	}

	err := t.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "template_uuid"}, {Name: "repository_configuration_uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{"distribution_href"}),
	}).Create(&templateRepoConfigs).Error
	if err != nil {
		return t.DBToApiError(err)
	}
	return nil
}

func templatesCreateApiToModel(api api.TemplateRequest, model *models.Template) {
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
		model.Date = api.Date.AsTime()
	}
	if api.OrgID != nil {
		model.OrgID = *api.OrgID
	}
	if api.User != nil {
		model.CreatedBy = *api.User
		model.LastUpdatedBy = *api.User
	}
	if api.UseLatest != nil {
		model.UseLatest = *api.UseLatest
	}
}

func templatesUpdateApiToModel(api api.TemplateUpdateRequest, model *models.Template) {
	if api.Description != nil {
		model.Description = *api.Description
	}
	if api.Date != nil {
		model.Date = api.Date.AsTime()
	}
	if api.OrgID != nil {
		model.OrgID = *api.OrgID
	}
	if api.User != nil {
		model.LastUpdatedBy = *api.User
	}
	if api.Name != nil {
		model.Name = *api.Name
	}
	if api.UseLatest != nil {
		model.UseLatest = *api.UseLatest
	}
}

func templatesModelToApi(model models.Template, api *api.TemplateResponse) {
	api.UUID = model.UUID
	api.RHSMEnvironmentID = candlepin_client.GetEnvironmentID(model.UUID)
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
	api.CreatedBy = model.CreatedBy
	api.LastUpdatedBy = model.LastUpdatedBy
	api.CreatedAt = model.CreatedAt
	api.UpdatedAt = model.UpdatedAt
	api.UseLatest = model.UseLatest
}

func templatesConvertToResponses(templates []models.Template) []api.TemplateResponse {
	responses := make([]api.TemplateResponse, len(templates))
	for i := 0; i < len(templates); i++ {
		templatesModelToApi(templates[i], &responses[i])
	}
	return responses
}
