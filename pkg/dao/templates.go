package dao

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
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
	db         *gorm.DB
	pulpClient pulp_client.PulpClient
	fsClient   feature_service_client.FeatureServiceClient
}

func GetTemplateDao(db *gorm.DB, pulpClient pulp_client.PulpClient, fsClient feature_service_client.FeatureServiceClient) TemplateDao {
	return &templateDaoImpl{
		db:         db,
		pulpClient: pulpClient,
		fsClient:   fsClient,
	}
}

func TemplateDBToApiError(e error, uuid *string) *ce.DaoError {
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
		daoError := ce.DaoError{BadValidation: dbError.Validation, Message: dbError.Message}
		daoError.Wrap(e)
		return &daoError
	}
	daoError := ce.DaoError{}
	if errors.Is(e, gorm.ErrRecordNotFound) {
		msg := "Template not found"
		if uuid != nil {
			msg = fmt.Sprintf("Template with UUID %s not found", *uuid)
		}
		daoError = ce.DaoError{
			Message:  msg,
			NotFound: true,
		}
	} else {
		daoError = ce.DaoError{
			Message:  e.Error(),
			NotFound: ce.HttpCodeForDaoError(e) == 404, // Check if isNotFoundError
		}
	}
	daoError.Wrap(e)
	return &daoError
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
		return api.TemplateResponse{}, TemplateDBToApiError(err, nil)
	}

	// Associate the template to repositories
	if reqTemplate.RepositoryUUIDS == nil {
		return api.TemplateResponse{}, &ce.DaoError{
			Message:       "template must include repository uuids",
			BadValidation: true,
		}
	}

	err = t.insertTemplateRepoConfigsAndSnapshots(tx, ctx, *reqTemplate.OrgID, modelTemplate, reqTemplate.RepositoryUUIDS)
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

	var repos []models.RepositoryConfiguration
	var missingSnapshotRepos []string
	resp = t.db.WithContext(ctx).Model(models.RepositoryConfiguration{}).
		Where("org_id = ? or org_id = ?", orgId, config.RedHatOrg).
		Where("uuid in ?", UuidifyStrings(uuids)).
		Find(&repos)
	if resp.Error != nil {
		return fmt.Errorf("could not query repository uuids: %w", resp.Error)
	}
	for _, repo := range repos {
		if repo.LastSnapshotUUID == "" {
			missingSnapshotRepos = append(missingSnapshotRepos, repo.Name)
		}
	}
	if len(missingSnapshotRepos) > 0 {
		msg := fmt.Sprintf("No snapshots found for the following repositories: [%v]", strings.Join(missingSnapshotRepos, ", "))
		return &ce.DaoError{NotFound: true, Message: msg}
	}

	return nil
}

func (t templateDaoImpl) insertTemplateRepoConfigsAndSnapshots(tx *gorm.DB, ctx context.Context, orgId string, template models.Template, repoUUIDs []string) error {
	templateRepoConfigs := make([]models.TemplateRepositoryConfiguration, len(repoUUIDs))

	var templateDate time.Time
	if template.UseLatest {
		templateDate = time.Now()
	} else {
		templateDate = template.Date
	}

	sDao := snapshotDaoImpl{db: tx}
	req := api.ListSnapshotByDateRequest{Date: templateDate, RepositoryUUIDS: repoUUIDs}
	snapshots, err := sDao.FetchSnapshotsModelByDateAndRepository(ctx, orgId, req)
	if err != nil {
		return err
	}

	for i, repo := range repoUUIDs {
		snapIndex := slices.IndexFunc(snapshots, func(s models.Snapshot) bool {
			return s.RepositoryConfigurationUUID == repo
		})

		templateRepoConfigs[i].TemplateUUID = template.UUID
		templateRepoConfigs[i].RepositoryConfigurationUUID = repo
		templateRepoConfigs[i].SnapshotUUID = snapshots[snapIndex].UUID
	}

	err = tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "template_uuid"}, {Name: "repository_configuration_uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{"deleted_at", "snapshot_uuid"}),
	}).Create(&templateRepoConfigs).Error
	if err != nil {
		return TemplateDBToApiError(err, nil)
	}
	return nil
}

func (t templateDaoImpl) DeleteTemplateRepoConfigs(ctx context.Context, templateUUID string, keepRepoConfigUUIDs []string) error {
	err := t.db.WithContext(ctx).Unscoped().Where("template_uuid = ? AND repository_configuration_uuid not in ?", UuidifyString(templateUUID), UuidifyStrings(keepRepoConfigUUIDs)).
		Delete(models.TemplateRepositoryConfiguration{}).Error

	if err != nil {
		return TemplateDBToApiError(err, nil)
	}
	return nil
}

func (t templateDaoImpl) softDeleteTemplateRepoConfigs(tx *gorm.DB, templateUUID string, keepRepoConfigUUIDs []string) error {
	err := tx.Debug().Where("template_uuid = ? AND repository_configuration_uuid not in ?", UuidifyString(templateUUID), UuidifyStrings(keepRepoConfigUUIDs)).
		Delete(&models.TemplateRepositoryConfiguration{}).Error

	if err != nil {
		return TemplateDBToApiError(err, nil)
	}
	return nil
}

func (t templateDaoImpl) Fetch(ctx context.Context, orgID string, uuid string, includeSoftDel bool) (api.TemplateResponse, error) {
	modelTemplate, err := t.fetch(ctx, orgID, uuid, includeSoftDel)
	if err != nil {
		return api.TemplateResponse{}, err
	}
	pulpContentPath := ""
	if config.Get().Features.Snapshots.Enabled {
		var err error
		pulpContentPath, err = t.pulpClient.GetContentPath(ctx)
		if err != nil {
			return api.TemplateResponse{}, err
		}
	}
	lastSnapshotUUIDs, err := t.fetchLatestSnapshotUUIDsForReposOfTemplates(ctx, []models.Template{modelTemplate})
	if err != nil {
		return api.TemplateResponse{}, TemplateDBToApiError(err, nil)
	}
	return templatesConvertToResponses([]models.Template{modelTemplate}, lastSnapshotUUIDs, pulpContentPath)[0], nil
}

func (t templateDaoImpl) fetch(ctx context.Context, orgID string, uuid string, includeSoftDel bool) (models.Template, error) {
	var modelTemplate models.Template
	query := t.db.WithContext(ctx)
	if includeSoftDel {
		query = query.Unscoped()
	}
	err := query.Where("uuid = ? AND org_id = ?", UuidifyString(uuid), orgID).
		Preload("TemplateRepositoryConfigurations.Snapshot.RepositoryConfiguration").
		Preload("LastUpdateTask").
		First(&modelTemplate).Error
	if err != nil {
		return modelTemplate, TemplateDBToApiError(err, &uuid)
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

	tx = tx.WithContext(ctx)
	if err := tx.Model(&validateTemplate).Where("uuid = ?", UuidifyString(uuid)).Updates(dbTempl.MapForUpdate()).Error; err != nil {
		return TemplateDBToApiError(err, &uuid)
	}

	var existingRepoConfigUUIDs []string
	if err := tx.Model(&models.TemplateRepositoryConfiguration{}).Select("repository_configuration_uuid").Where("template_uuid = ?", dbTempl.UUID).Find(&existingRepoConfigUUIDs).Error; err != nil {
		return RepositoryDBErrorToApi(err, nil)
	}

	if templParams.RepositoryUUIDS != nil {
		if err := t.validateRepositoryUUIDs(ctx, orgID, templParams.RepositoryUUIDS); err != nil {
			return err
		}

		err = t.softDeleteTemplateRepoConfigs(tx, uuid, templParams.RepositoryUUIDS)
		if err != nil {
			return fmt.Errorf("could not remove uneeded template repositories %w", err)
		}

		err = t.insertTemplateRepoConfigsAndSnapshots(tx, ctx, dbTempl.OrgID, dbTempl, templParams.RepositoryUUIDS)
		if err != nil {
			return fmt.Errorf("could not insert new template repositories %w", err)
		}
	}
	return nil
}

func (t templateDaoImpl) List(ctx context.Context, orgID string, includeSoftDel bool, paginationData api.PaginationData, filterData api.TemplateFilterData) (api.TemplateCollectionResponse, int64, error) {
	var totalTemplates int64
	templates := make([]models.Template, 0)

	filteredDB := t.filteredDbForList(orgID, t.db.WithContext(ctx), filterData)
	if includeSoftDel {
		filteredDB = filteredDB.Unscoped()
	}

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
		return api.TemplateCollectionResponse{}, totalTemplates, TemplateDBToApiError(filteredDB.Error, nil)
	}

	if filteredDB.
		Distinct("templates.*").
		Preload("TemplateRepositoryConfigurations").
		Preload("LastUpdateTask").
		Order(order).
		Limit(paginationData.Limit).
		Offset(paginationData.Offset).
		Find(&templates).Error != nil {
		return api.TemplateCollectionResponse{}, totalTemplates, TemplateDBToApiError(filteredDB.Error, nil)
	}

	pulpContentPath := ""
	if config.Get().Features.Snapshots.Enabled {
		var err error
		pulpContentPath, err = t.pulpClient.GetContentPath(ctx)
		if err != nil {
			return api.TemplateCollectionResponse{}, 0, err
		}
	}
	lastSnapshotUUIDs, err := t.fetchLatestSnapshotUUIDsForReposOfTemplates(ctx, templates)
	if err != nil {
		return api.TemplateCollectionResponse{}, totalTemplates, TemplateDBToApiError(err, nil)
	}
	responses := templatesConvertToResponses(templates, lastSnapshotUUIDs, pulpContentPath)

	return api.TemplateCollectionResponse{Data: responses}, totalTemplates, nil
}

func (t templateDaoImpl) InternalOnlyFetchByName(ctx context.Context, name string) (models.Template, error) {
	var modelTemplate models.Template
	err := t.db.WithContext(ctx).
		Where("name = ? ", name).
		Preload("TemplateRepositoryConfigurations").First(&modelTemplate).Error
	if err != nil {
		return modelTemplate, TemplateDBToApiError(err, nil)
	}
	return modelTemplate, nil
}

func (t templateDaoImpl) filteredDbForList(orgID string, filteredDB *gorm.DB, filterData api.TemplateFilterData) *gorm.DB {
	filteredDB = filteredDB.Where("org_id = ? ", orgID).Preload("TemplateRepositoryConfigurations.Snapshot.RepositoryConfiguration")

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

	if len(filterData.RepositoryUUIDs) > 0 || len(filterData.SnapshotUUIDs) > 0 {
		filteredDB = filteredDB.
			Joins("INNER JOIN templates_repository_configurations on templates_repository_configurations.template_uuid = templates.uuid")
	}
	if len(filterData.RepositoryUUIDs) > 0 && len(filterData.SnapshotUUIDs) > 0 {
		filteredDB = filteredDB.
			Where("templates_repository_configurations.repository_configuration_uuid in ?", UuidifyStrings(filterData.RepositoryUUIDs)).
			Or("templates_repository_configurations.snapshot_uuid in ?", UuidifyStrings(filterData.SnapshotUUIDs))
	} else if len(filterData.RepositoryUUIDs) > 0 {
		filteredDB = filteredDB.
			Where("templates_repository_configurations.repository_configuration_uuid in ?", UuidifyStrings(filterData.RepositoryUUIDs))
	} else if len(filterData.SnapshotUUIDs) > 0 {
		filteredDB = filteredDB.
			Where("templates_repository_configurations.snapshot_uuid in ?", UuidifyStrings(filterData.SnapshotUUIDs))
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
		return TemplateDBToApiError(err, &uuid)
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
		return TemplateDBToApiError(err, &uuid)
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
		return nil, nil, nil, nil, TemplateDBToApiError(err, nil)
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

func (t templateDaoImpl) UpdateDistributionHrefs(ctx context.Context, templateUUID string, repoUUIDs []string, snapshots []models.Snapshot, repoDistributionMap map[string]string) error {
	templateRepoConfigs := make([]models.TemplateRepositoryConfiguration, len(repoUUIDs))
	for i, repo := range repoUUIDs {
		snapIndex := slices.IndexFunc(snapshots, func(s models.Snapshot) bool {
			return s.RepositoryConfigurationUUID == repo
		})

		templateRepoConfigs[i].TemplateUUID = templateUUID
		templateRepoConfigs[i].RepositoryConfigurationUUID = repo
		templateRepoConfigs[i].SnapshotUUID = snapshots[snapIndex].UUID
		if repoDistributionMap != nil {
			templateRepoConfigs[i].DistributionHref = repoDistributionMap[repo]
		}
	}

	err := t.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "template_uuid"}, {Name: "repository_configuration_uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{"distribution_href"}),
	}).Create(&templateRepoConfigs).Error
	if err != nil {
		return TemplateDBToApiError(err, nil)
	}
	return nil
}

func (t templateDaoImpl) SetEnvironmentCreated(ctx context.Context, templateUUID string) error {
	result := t.db.WithContext(ctx).Exec(`
			UPDATE templates
			SET rhsm_environment_created = true 			
			WHERE uuid = ?`,
		templateUUID,
	)

	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (t templateDaoImpl) UpdateLastUpdateTask(ctx context.Context, taskUUID string, orgID string, templateUUID string) error {
	result := t.db.WithContext(ctx).Exec(`
			UPDATE templates
			SET last_update_task_uuid = ? 
			WHERE org_id = ?
			AND uuid = ?`,
		taskUUID,
		orgID,
		templateUUID,
	)

	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (t templateDaoImpl) UpdateLastError(ctx context.Context, orgID string, templateUUID string, lastUpdateSnapshotError string) error {
	result := t.db.WithContext(ctx).Exec(`
			UPDATE templates
			SET last_update_snapshot_error = ? 
			WHERE org_id = ?
			AND uuid = ?`,
		lastUpdateSnapshotError,
		orgID,
		templateUUID,
	)

	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (t templateDaoImpl) UpdateSnapshots(ctx context.Context, templateUUID string, repoUUIDs []string, snapshots []models.Snapshot) error {
	var templateRepoConfigs []models.TemplateRepositoryConfiguration

	for _, repo := range repoUUIDs {
		snapIndex := slices.IndexFunc(snapshots, func(s models.Snapshot) bool {
			return s.RepositoryConfigurationUUID == repo
		})

		templateRepoConfigs = append(templateRepoConfigs, models.TemplateRepositoryConfiguration{
			TemplateUUID:                templateUUID,
			RepositoryConfigurationUUID: repo,
			SnapshotUUID:                snapshots[snapIndex].UUID,
		})
	}

	if len(templateRepoConfigs) > 0 {
		err := t.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "template_uuid"}, {Name: "repository_configuration_uuid"}},
			DoUpdates: clause.AssignmentColumns([]string{"snapshot_uuid"}),
		}).Create(&templateRepoConfigs).Error
		if err != nil {
			return TemplateDBToApiError(err, nil)
		}
	}
	return nil
}

func (t templateDaoImpl) DeleteTemplateSnapshot(ctx context.Context, snapshotUUID string) error {
	err := t.db.WithContext(ctx).Unscoped().Where("snapshot_uuid = ?", UuidifyString(snapshotUUID)).
		Delete(models.TemplateRepositoryConfiguration{}).Error

	if err != nil {
		return TemplateDBToApiError(err, nil)
	}
	return nil
}

func (t templateDaoImpl) GetRepositoryConfigurationFile(ctx context.Context, orgID, templateUUID string) (string, error) {
	sDao := snapshotDaoImpl(t)
	rcDao := repositoryConfigDaoImpl{db: t.db, fsClient: t.fsClient}

	template, err := t.Fetch(ctx, orgID, templateUUID, false)
	if err != nil {
		return "", err
	}

	pc := t.pulpClient
	contentPath, err := pc.GetContentPath(ctx)
	if err != nil {
		return "", err
	}

	var templateRepoConfigFile strings.Builder
	for _, snap := range template.Snapshots {
		repoConfig, err := rcDao.fetchRepoConfig(ctx, orgID, snap.RepositoryUUID, true)
		if err != nil {
			return "", err
		}

		repoConfigFile, err := sDao.GetRepositoryConfigurationFile(ctx, orgID, snap.UUID, false)
		if err != nil {
			return "", err
		}

		var contentURL string
		domain := strings.Split(snap.RepositoryPath, "/")[0]
		parsedRepoURL, err := url.Parse(repoConfig.Repository.URL)
		if err != nil {
			return "", err
		}
		path := parsedRepoURL.Path
		if repoConfig.IsRedHat() {
			contentURL = contentPath + domain + "/templates/" + templateUUID + path
		} else {
			contentURL = contentPath + domain + "/templates/" + templateUUID + "/" + snap.RepositoryUUID
		}

		// replace baseurl with the one specific to the template
		re, err := regexp.Compile(`(?m)^baseurl=.*`)
		if err != nil {
			return "", err
		}
		if !re.MatchString(repoConfigFile) {
			return "", fmt.Errorf("baseurl not found in config file")
		}
		repoConfigFile = re.ReplaceAllString(repoConfigFile, fmt.Sprintf("baseurl=%s", contentURL))

		templateRepoConfigFile.WriteString(repoConfigFile)
		templateRepoConfigFile.WriteString("\n")
	}
	return templateRepoConfigFile.String(), nil
}

func (t templateDaoImpl) InternalOnlyGetTemplatesForRepoConfig(ctx context.Context, repoUUID string, useLatestOnly bool) ([]api.TemplateResponse, error) {
	var templates []models.Template
	filtered := t.db.Model(&models.Template{}).WithContext(ctx).
		Joins("INNER JOIN templates_repository_configurations on templates_repository_configurations.template_uuid = templates.uuid").
		Where("templates_repository_configurations.repository_configuration_uuid", repoUUID)
	if useLatestOnly {
		filtered = filtered.Where("use_latest = true")
	}
	res := filtered.Find(&templates)
	if res.Error != nil {
		return nil, TemplateDBToApiError(res.Error, nil)
	}

	pulpContentPath := ""
	if config.Get().Features.Snapshots.Enabled {
		var err error
		pulpContentPath, err = t.pulpClient.GetContentPath(ctx)
		if err != nil {
			return nil, err
		}
	}
	lastSnapshotUUIDs, err := t.fetchLatestSnapshotUUIDsForReposOfTemplates(ctx, templates)
	if err != nil {
		return nil, TemplateDBToApiError(err, nil)
	}
	responses := templatesConvertToResponses(templates, lastSnapshotUUIDs, pulpContentPath)

	return responses, nil
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
		model.Date = api.Date.AsTime().UTC()
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
		model.Date = api.Date.AsTime().UTC()
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

func templatesModelToApi(model models.Template, apiTemplate *api.TemplateResponse) {
	apiTemplate.UUID = model.UUID
	apiTemplate.RHSMEnvironmentID = candlepin_client.GetEnvironmentID(model.UUID)
	apiTemplate.RHSMEnvironmentCreated = model.RHSMEnvironmentCreated
	apiTemplate.OrgID = model.OrgID
	apiTemplate.Name = model.Name
	apiTemplate.Description = model.Description
	apiTemplate.Version = model.Version
	apiTemplate.Arch = model.Arch
	apiTemplate.Date = model.Date.UTC()
	apiTemplate.CreatedBy = model.CreatedBy
	apiTemplate.LastUpdatedBy = model.LastUpdatedBy
	apiTemplate.CreatedAt = model.CreatedAt.UTC()
	apiTemplate.UpdatedAt = model.UpdatedAt.UTC()
	apiTemplate.UseLatest = model.UseLatest
	apiTemplate.DeletedAt = model.DeletedAt
	if model.LastUpdateSnapshotError != nil {
		apiTemplate.LastUpdateSnapshotError = *model.LastUpdateSnapshotError
	}
	apiTemplate.LastUpdateTaskUUID = model.LastUpdateTaskUUID
	if model.LastUpdateTask != nil {
		apiTemplate.LastUpdateTask = &api.TaskInfoResponse{
			UUID:       model.LastUpdateTaskUUID,
			Status:     model.LastUpdateTask.Status,
			Typename:   model.LastUpdateTask.Typename,
			OrgId:      model.LastUpdateTask.OrgId,
			ObjectType: config.ObjectTypeTemplate,
			ObjectUUID: model.UUID,
			ObjectName: model.Name,
		}
		if model.LastUpdateTask.Started != nil {
			apiTemplate.LastUpdateTask.CreatedAt = model.LastUpdateTask.Started.Format(time.RFC3339)
		}
		if model.LastUpdateTask.Finished != nil {
			apiTemplate.LastUpdateTask.EndedAt = model.LastUpdateTask.Finished.Format(time.RFC3339)
		}
		if model.LastUpdateTask.Error != nil {
			apiTemplate.LastUpdateTask.Error = *model.LastUpdateTask.Error
		}
	}
}

func templatesConvertToResponses(templates []models.Template, lastSnapshotsUUIDs []string, pulpContentPath string) []api.TemplateResponse {
	responses := make([]api.TemplateResponse, len(templates))
	outdatedDate := time.Now().Add(-time.Duration((config.Get().Options.SnapshotRetainDaysLimit-14)*24) * time.Hour)
	for i := 0; i < len(templates); i++ {
		templatesModelToApi(templates[i], &responses[i])
		// Add in associations (Repository Config UUIDs and Snapshots)
		responses[i].RepositoryUUIDS = make([]string, 0) // prevent null responses
		responses[i].ToBeDeletedSnapshots = make([]api.SnapshotResponse, 0)
		for _, tRepoConfig := range templates[i].TemplateRepositoryConfigurations {
			responses[i].RepositoryUUIDS = append(responses[i].RepositoryUUIDS, tRepoConfig.RepositoryConfigurationUUID)
			snaps := snapshotConvertToResponses([]models.Snapshot{tRepoConfig.Snapshot}, pulpContentPath)
			responses[i].Snapshots = append(responses[i].Snapshots, snaps[0])
		}
		for _, snap := range responses[i].Snapshots {
			if snap.CreatedAt.Before(outdatedDate) && !slices.Contains(lastSnapshotsUUIDs, snap.UUID) {
				responses[i].ToBeDeletedSnapshots = append(responses[i].ToBeDeletedSnapshots, snap)
			}
		}
	}
	return responses
}

func (t templateDaoImpl) fetchLatestSnapshotUUIDsForReposOfTemplates(ctx context.Context, templates []models.Template) ([]string, error) {
	var repoUUIDs = make([]string, 0)
	var repos []models.RepositoryConfiguration
	var snapshotUUIDs = make([]string, 0)

	for _, template := range templates {
		for _, trc := range template.TemplateRepositoryConfigurations {
			repoUUIDs = append(repoUUIDs, trc.RepositoryConfigurationUUID)
		}
	}
	slices.Sort(repoUUIDs)
	repoUUIDs = slices.Compact(repoUUIDs)

	err := t.db.WithContext(ctx).
		Where("uuid IN ? ", repoUUIDs).
		Find(&repos).
		Error
	if err != nil {
		return snapshotUUIDs, err
	}
	for _, repo := range repos {
		snapshotUUIDs = append(snapshotUUIDs, repo.LastSnapshotUUID)
	}

	return snapshotUUIDs, nil
}
