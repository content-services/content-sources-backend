package jobs

import (
	"context"
	"strconv"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const defaultUpdateAllTemplateContentBatchSize = 100

// UpdateAllTemplateContent enqueues update-template-content tasks for every non-deleted template
// and waits for each batch to finish before enqueueing the next, so a full refresh does not
// overwhelm the tasking system.
//
// Usage: go run cmd/jobs/main.go update-all-template-content [batch-size]
// Examples:
//   - update-all-template-content       (batches of 100)
//   - update-all-template-content 50    (batches of 50)
func UpdateAllTemplateContent(args []string) {
	ctx := context.Background()
	daoReg := dao.GetDaoRegistry(db.DB)

	batchSize := defaultUpdateAllTemplateContentBatchSize
	if len(args) > 0 && args[0] != "" {
		parsed, err := strconv.Atoi(args[0])
		if err != nil || parsed < 1 {
			log.Fatal().Err(err).Msg("Invalid batch size parameter. Must be a positive integer.")
		}
		batchSize = parsed
	}

	pgQueue, err := queue.NewPgQueue(ctx, db.GetUrl())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to pgqueue")
	}
	defer pgQueue.Close()
	taskClient := client.NewTaskClient(&pgQueue)

	var templates []models.Template
	err = db.DB.
		Preload("TemplateRepositoryConfigurations").
		Where("deleted_at IS NULL").
		Order("uuid").
		Find(&templates).Error
	if err != nil {
		log.Fatal().Err(err).Msg("failed to fetch templates")
	}

	log.Info().
		Int("template_count", len(templates)).
		Int("batch_size", batchSize).
		Msg("Found templates to refresh")

	if len(templates) == 0 {
		return
	}

	enqueued := 0
	skipped := 0
	failed := 0

	for i := 0; i < len(templates); i += batchSize {
		end := i + batchSize
		if end > len(templates) {
			end = len(templates)
		}
		batch := templates[i:end]

		log.Info().
			Int("batch_start", i+1).
			Int("batch_end", end).
			Int("total", len(templates)).
			Msg("Enqueueing update-template-content batch")

		var batchTaskIDs []uuid.UUID
		for _, template := range batch {
			repoConfigUUIDs := make([]string, 0, len(template.TemplateRepositoryConfigurations))
			for _, trc := range template.TemplateRepositoryConfigurations {
				repoConfigUUIDs = append(repoConfigUUIDs, trc.RepositoryConfigurationUUID)
			}
			if len(repoConfigUUIDs) == 0 {
				log.Warn().
					Str("template_uuid", template.UUID).
					Str("org_id", template.OrgID).
					Msg("skipping template with no repository configurations")
				skipped++
				continue
			}

			payload := payloads.UpdateTemplateContentPayload{
				TemplateUUID:    template.UUID,
				RepoConfigUUIDs: repoConfigUUIDs,
			}
			task := queue.Task{
				Typename:   config.UpdateTemplateContentTask,
				Payload:    payload,
				OrgId:      template.OrgID,
				ObjectUUID: &template.UUID,
				ObjectType: utils.Ptr(config.ObjectTypeTemplate),
			}
			taskID, enqueueErr := taskClient.Enqueue(task)
			if enqueueErr != nil {
				log.Error().Err(enqueueErr).
					Str("template_uuid", template.UUID).
					Str("org_id", template.OrgID).
					Msg("error enqueuing update-template-content task")
				failed++
				continue
			}
			logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
			if updateErr := daoReg.Template.UpdateLastUpdateTask(ctx, taskID.String(), template.OrgID, template.UUID); updateErr != nil {
				logger.Error().Err(updateErr).Msgf("error updating last_update_task for template %v", template.UUID)
			}
			batchTaskIDs = append(batchTaskIDs, taskID)
			enqueued++
		}

		if len(batchTaskIDs) == 0 {
			continue
		}

		log.Info().
			Int("task_count", len(batchTaskIDs)).
			Msg("Waiting for update-template-content batch to finish")
		waitForTasks(&pgQueue, batchTaskIDs)
	}

	log.Info().
		Int("enqueued", enqueued).
		Int("skipped", skipped).
		Int("enqueue_failures", failed).
		Int("total_templates", len(templates)).
		Msg("Finished update-all-template-content")
}

func waitForTasks(pgQueue *queue.PgQueue, taskIDs []uuid.UUID) {
	pending := make(map[uuid.UUID]struct{}, len(taskIDs))
	for _, id := range taskIDs {
		pending[id] = struct{}{}
	}

	for len(pending) > 0 {
		for id := range pending {
			info, err := pgQueue.Status(id)
			if err != nil {
				log.Error().Err(err).Str("task_uuid", id.String()).Msg("error checking task status")
				continue
			}
			if info.Status == config.TaskStatusPending || info.Status == config.TaskStatusRunning {
				continue
			}
			if info.Status == config.TaskStatusFailed {
				log.Error().
					Str("task_uuid", id.String()).
					Str("status", info.Status).
					Interface("error", info.Error).
					Msg("update-template-content task finished with failure")
			} else {
				log.Info().
					Str("task_uuid", id.String()).
					Str("status", info.Status).
					Msg("update-template-content task finished")
			}
			delete(pending, id)
		}
		if len(pending) == 0 {
			return
		}
		log.Info().Int("remaining", len(pending)).Msg("Batch still in progress; sleeping")
		time.Sleep(5 * time.Second)
	}
}
