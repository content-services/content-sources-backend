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

// DeleteInvalidRedHatRepos deletes repositories with origin='red_hat' that are not
// associated with the RedHatOrg.
func DeleteInvalidRedHatRepos(_ []string) {
	ctx := context.Background()

	pgQueue, err := queue.NewPgQueue(ctx, db.GetUrl())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to pgq")
	}
	defer pgQueue.Close()
	taskClient := client.NewTaskClient(&pgQueue)

	reposToDelete := []models.RepositoryConfiguration{}

	query := db.DB.Model(models.RepositoryConfiguration{}).Where("repository_configurations.org_id not in (?)", []string{config.RedHatOrg, config.CommunityOrg})
	query = query.Joins("INNER JOIN repositories on repositories.uuid = repository_configurations.repository_uuid")
	query = query.Where("repositories.origin = ?", config.OriginRedHat).Preload("Repository")
	res := query.Find(&reposToDelete)
	if res.Error != nil {
		log.Fatal().Err(res.Error).Msg("failed to query invalid red_hat repositories")
	}

	if len(reposToDelete) == 0 {
		log.Info().Msg("No invalid red_hat repositories found to delete")
		return
	}

	log.Info().Msgf("Found %d invalid red_hat repositories to delete", len(reposToDelete))
	for _, repo := range reposToDelete {
		log.Warn().Msgf("Deleting repository UUID: %s, ORG_ID: %s, URL: %s, Origin: %s", repo.UUID, repo.OrgID, repo.Repository.URL, repo.Repository.Origin)
		daoReg := dao.GetDaoRegistry(db.DB)

		if err := daoReg.RepositoryConfig.SoftDelete(ctx, repo.OrgID, repo.UUID); err != nil {
			log.Fatal().Err(res.Error).Msg("failed to soft delete invalid red_hat repository")
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
			logger.Error().Msg("error enqueuing task")
		}
	}
	if res.Error != nil {
		log.Fatal().Err(res.Error).Msg("failed to delete invalid red_hat repositories")
	}

	log.Info().Msgf("Successfully deleted %d invalid red_hat repositories.", res.RowsAffected)
}
