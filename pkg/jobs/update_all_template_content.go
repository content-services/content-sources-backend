package jobs

import (
	"context"
	"fmt"
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

const (
	defaultUpdateAllTemplateContentBatchSize = 100
	maxConsecutiveTaskStatusErrors           = 10
)

// UpdateAllTemplateContent enqueues update-template-content tasks for every non-deleted template
// and waits for each batch to finish before enqueueing the next, so a full refresh does not
// overwhelm the tasking system. Templates are loaded one org at a time to limit memory use.
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

	var orgIDs []string
	err = db.DB.Model(&models.Template{}).
		Distinct("org_id").
		Order("org_id").
		Pluck("org_id", &orgIDs).Error
	if err != nil {
		log.Fatal().Err(err).Msg("failed to fetch template org ids")
	}

	log.Info().
		Int("org_count", len(orgIDs)).
		Int("batch_size", batchSize).
		Msg("Found orgs with templates to refresh")

	if len(orgIDs) == 0 {
		return
	}

	enqueued := 0
	skipped := 0
	failed := 0
	totalTemplates := 0

	for _, orgID := range orgIDs {
		var templates []models.Template
		err = db.DB.
			Preload("TemplateRepositoryConfigurations").
			Where("org_id = ?", orgID).
			Order("uuid").
			Find(&templates).Error
		if err != nil {
			log.Fatal().Err(err).Str("org_id", orgID).Msg("failed to fetch templates")
		}

		totalTemplates += len(templates)
		log.Info().
			Str("org_id", orgID).
			Int("template_count", len(templates)).
			Msg("Processing org templates")

		for i := 0; i < len(templates); i += batchSize {
			end := i + batchSize
			if end > len(templates) {
				end = len(templates)
			}
			batch := templates[i:end]

			log.Info().
				Str("org_id", orgID).
				Int("batch_start", i+1).
				Int("batch_end", end).
				Int("org_total", len(templates)).
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
				Str("org_id", orgID).
				Int("task_count", len(batchTaskIDs)).
				Msg("Waiting for update-template-content batch to finish")
			if waitErr := waitForTasks(&pgQueue, batchTaskIDs); waitErr != nil {
				log.Fatal().Err(waitErr).Str("org_id", orgID).Msg("failed while waiting for update-template-content batch")
			}
		}
	}

	log.Info().
		Int("enqueued", enqueued).
		Int("skipped", skipped).
		Int("enqueue_failures", failed).
		Int("total_templates", totalTemplates).
		Int("org_count", len(orgIDs)).
		Msg("Finished update-all-template-content")
}

func waitForTasks(pgQueue *queue.PgQueue, taskIDs []uuid.UUID) error {
	pending := make(map[uuid.UUID]struct{}, len(taskIDs))
	for _, id := range taskIDs {
		pending[id] = struct{}{}
	}

	consecutiveStatusErrors := 0
	for len(pending) > 0 {
		for id := range pending {
			info, err := pgQueue.Status(id)
			if err != nil {
				consecutiveStatusErrors++
				log.Error().Err(err).
					Str("task_uuid", id.String()).
					Int("consecutive_errors", consecutiveStatusErrors).
					Msg("error checking task status")
				if consecutiveStatusErrors >= maxConsecutiveTaskStatusErrors {
					return fmt.Errorf("aborting wait after %d consecutive task status errors: %w", consecutiveStatusErrors, err)
				}
				continue
			}
			consecutiveStatusErrors = 0
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
			return nil
		}
		log.Info().Int("remaining", len(pending)).Msg("Batch still in progress; sleeping")
		time.Sleep(5 * time.Second)
	}
	return nil
}
