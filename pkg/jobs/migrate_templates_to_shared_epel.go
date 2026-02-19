package jobs

import (
	"context"
	"slices"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/rs/zerolog/log"
)

func MigrateTemplatesToSharedEpel(_ []string) {
	ctx := context.Background()
	daoReg := dao.GetDaoRegistry(db.DB)

	pgQueue, err := queue.NewPgQueue(ctx, db.GetUrl())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to pgqueue")
	}
	defer pgQueue.Close()
	taskClient := client.NewTaskClient(&pgQueue)

	// Fetch templates with custom epel
	var templates []models.Template
	err = db.DB.
		Preload("TemplateRepositoryConfigurations").
		Joins("INNER JOIN templates_repository_configurations trc ON trc.template_uuid = templates.uuid").
		Joins("INNER JOIN repository_configurations rc ON rc.uuid = trc.repository_configuration_uuid").
		Joins("INNER JOIN repositories r ON r.uuid = rc.repository_uuid").
		Where("templates.deleted_at IS NULL").
		Where("r.origin = ?", config.OriginExternal).
		Where("r.url IN ?", config.EPELUrls).
		Find(&templates).
		Error
	if err != nil {
		log.Fatal().Err(err).Msg("failed to fetch templates")
	}

	if len(templates) == 0 {
		log.Info().Msgf("No templates found with custom epel")
		return
	}

	log.Info().Msgf("Found %v template(s) with custom epel", len(templates))

	// Fetch shared epel repo configs
	var sharedEpelRepoConfigs []models.RepositoryConfiguration
	err = db.DB.
		Model(models.RepositoryConfiguration{}).
		Preload("Repository").
		Joins("INNER JOIN repositories ON repositories.uuid = repository_configurations.repository_uuid").
		Where("url IN ? AND org_id = ? AND origin = ?", config.EPELUrls, config.CommunityOrg, config.OriginCommunity).
		Find(&sharedEpelRepoConfigs).
		Error
	if err != nil {
		log.Fatal().Err(err).Msg("failed to fetch shared epel repository configurations")
	}

	// Map shared epel urls to repo config uuid
	sharedEpelMap := make(map[string]string, len(sharedEpelRepoConfigs))
	for _, repoConfig := range sharedEpelRepoConfigs {
		repoURL := models.CleanupURL(repoConfig.Repository.URL)
		sharedEpelMap[repoURL] = repoConfig.UUID
	}

	var updatedTemplates []models.Template
	for _, template := range templates {
		// Fetch repo configs for each template with custom epel
		repoConfigs, err := daoReg.RepositoryConfig.InternalOnly_FetchRepoConfigsForTemplate(ctx, template)
		if err != nil {
			log.Error().Err(err).Msgf("failed to fetch repository configurations for template %s", template.UUID)
			continue
		}

		updatedRepoConfigUUIDs := make([]string, len(repoConfigs))
		replaced := false
		for i, repoConfig := range repoConfigs {
			repoURL := models.CleanupURL(repoConfig.Repository.URL)
			if !replaced && repoConfig.Repository.Origin == config.OriginExternal && slices.Contains(config.EPELUrls, repoURL) {
				// Check for shared epel with matching url, save shared epel to updated repo list if there's a match
				if sharedEpelRepoConfigUUID, ok := sharedEpelMap[repoURL]; ok {
					log.Info().
						Str("template_uuid", template.UUID).
						Str("org_id", repoConfig.OrgID).
						Str("custom_epel_uuid", repoConfig.UUID).
						Str("shared_epel_uuid", sharedEpelRepoConfigUUID).
						Msg("Replacing custom EPEL repo with shared EPEL repo")
					updatedRepoConfigUUIDs[i] = sharedEpelRepoConfigUUID
					replaced = true
					continue
				}
			}
			// Save unchanged repos to updated repo list
			updatedRepoConfigUUIDs[i] = repoConfig.UUID
		}

		// Update the template (this will also update the template_repository_configurations table)
		templateUpdateRequest := api.TemplateUpdateRequest{RepositoryUUIDS: updatedRepoConfigUUIDs}
		_, err = daoReg.Template.Update(ctx, template.OrgID, template.UUID, templateUpdateRequest)
		if err != nil {
			log.Error().Err(err).Msgf("failed to update template %s", template.UUID)
			continue
		}

		// Enqueue update-template-content task
		// Repos not included in the payload but that exist in the template_repository_configurations table will be removed from the template
		payload := payloads.UpdateTemplateContentPayload{
			TemplateUUID:    template.UUID,
			RepoConfigUUIDs: updatedRepoConfigUUIDs,
		}
		task := queue.Task{
			Typename:   config.UpdateTemplateContentTask,
			Payload:    payload,
			OrgId:      template.OrgID,
			ObjectUUID: &template.UUID,
			ObjectType: utils.Ptr(config.ObjectTypeTemplate),
		}
		taskID, err := taskClient.Enqueue(task)
		logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
		if err != nil {
			logger.Error().Msgf("error enqueuing task for template %v", template.UUID)
		}
		if err == nil {
			if err = daoReg.Template.UpdateLastUpdateTask(ctx, taskID.String(), template.OrgID, template.UUID); err != nil {
				logger.Error().Msgf("error updating last_update_task for template %v", template.UUID)
			} else {
				template.LastUpdateTaskUUID = taskID.String()
			}
		}

		updatedTemplates = append(updatedTemplates, template)
	}

	log.Warn().Msgf("Updated %v template(s)", len(updatedTemplates))
}
