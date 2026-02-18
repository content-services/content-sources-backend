package jobs

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/rs/zerolog/log"
)

func RemoveCustomEpelRepos(_ []string) {
	ctx := context.Background()

	pgQueue, err := queue.NewPgQueue(ctx, db.GetUrl())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create PgQueue")
	}
	defer pgQueue.Close()
	taskClient := client.NewTaskClient(&pgQueue)

	epelURLs := []string{
		"https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/",
		"https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/",
		"https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/",
	}

	cleanedEpelURLs := make([]string, len(epelURLs))
	for i, url := range epelURLs {
		cleanedEpelURLs[i] = models.CleanupURL(url)
	}

	reposToDelete := []models.RepositoryConfiguration{}

	query := db.DB.Model(models.RepositoryConfiguration{}).
		Joins("INNER JOIN repositories on repositories.uuid = repository_configurations.repository_uuid").
		Where("repositories.url IN (?)", cleanedEpelURLs).
		Where("repositories.origin = ?", config.OriginExternal).
		Where("repository_configurations.org_id NOT IN (?)", []string{config.CommunityOrg}).
		Preload("Repository")
	res := query.Find(&reposToDelete)
	if res.Error != nil {
		log.Fatal().Err(res.Error).Msg("failed to query custom EPEL repositories")
	}

	if len(reposToDelete) == 0 {
		log.Info().Msg("No custom EPEL repositories found to delete")
		return
	}

	log.Info().Msgf("Found %d custom EPEL repositories to delete", len(reposToDelete))

	daoReg := dao.GetDaoRegistry(db.DB)

	for _, repo := range reposToDelete {
		log.Warn().Msgf("Deleting custom EPEL repository UUID: %s, ORG_ID: %s, URL: %s, Name: %s, Origin: %s",
			repo.UUID, repo.OrgID, repo.Repository.URL, repo.Name, repo.Repository.Origin)

		taskIDs, err := daoReg.TaskInfo.FetchActiveTasks(ctx, repo.OrgID, repo.RepositoryUUID, config.RepositorySnapshotTask, config.IntrospectTask)
		if err != nil {
			log.Error().Err(err).Msg("RemoveCustomEpelRepos: failed to fetch active tasks")
		}

		for _, taskID := range taskIDs {
			err = taskClient.Cancel(ctx, taskID)
			if err != nil {
				log.Error().Err(err).Str("task_id", taskID).Msg("RemoveCustomEpelRepos: failed to cancel task")
			}
		}

		if err := daoReg.RepositoryConfig.SoftDelete(ctx, repo.OrgID, repo.UUID); err != nil {
			log.Error().Err(err).Msgf("failed to soft delete custom EPEL repository %s", repo.UUID)
			continue
		}

		payload := tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repo.UUID}
		task := queue.Task{
			Typename:   config.DeleteRepositorySnapshotsTask,
			Payload:    payload,
			OrgId:      repo.OrgID,
			AccountId:  repo.AccountID,
			ObjectUUID: &repo.RepositoryUUID,
			ObjectType: utils.Ptr(config.ObjectTypeRepository),
		}
		taskID, err := taskClient.Enqueue(task)
		if err != nil {
			logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
			logger.Error().Err(err).Msg("error enqueuing snapshot deletion task")
		} else {
			log.Info().Msgf("Enqueued snapshot deletion task %s for repository %s", taskID.String(), repo.UUID)
		}
	}

	log.Info().Msgf("Successfully processed %d custom EPEL repositories for deletion", len(reposToDelete))
}
