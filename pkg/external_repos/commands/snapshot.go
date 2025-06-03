package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
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
	"github.com/urfave/cli/v2"
)

func SnapshotAction(c *cli.Context) error {
	ctx := c.Context
	urlsParam := c.StringSlice("url")
	force := c.Bool("force")

	var urls []string
	for _, url := range urlsParam {
		urls = append(urls, models.CleanupURL(url))
	}

	if config.Get().Features.Snapshots.Enabled {
		waitForPulp(ctx)
		err := enqueueSnapshotRepos(ctx, &urls, nil, force)
		if err != nil {
			log.Warn().Msgf("Error enqueuing snapshot tasks: %v", err)
		}
	} else {
		log.Warn().Msg("Snapshotting disabled")
	}
	return nil
}

func waitForPulp(ctx context.Context) {
	failedOnce := false
	for {
		_, err := pulp_client.GetGlobalPulpClient().LookupDomain(ctx, pulp_client.DefaultDomain)
		if err == nil {
			if failedOnce {
				log.Warn().Msg("Pulp user has been created, sleeping for role creation to happen")
				time.Sleep(20 * time.Second)
			}
			return
		}
		failedOnce = true
		log.Warn().Err(err).Msg("Pulp isn't up yet, waiting 5s.")
		time.Sleep(5 * time.Second)
	}
}

func enqueueSnapshotRepos(ctx context.Context, urls *[]string, interval *int, force bool) error {
	q, err := queue.NewPgQueue(ctx, db.GetUrl())
	if err != nil {
		return fmt.Errorf("error getting new task queue: %w", err)
	}
	defer q.Close()
	c := client.NewTaskClient(&q)
	fs, err := feature_service_client.NewFeatureServiceClient()
	if err != nil {
		return fmt.Errorf("error getting feature service client: %w", err)
	}

	repoConfigDao := dao.GetRepositoryConfigDao(db.DB, pulp_client.GetPulpClientWithDomain(""), fs)
	filter := &dao.ListRepoFilter{
		URLs:            urls,
		MinimumInterval: interval,
		Force:           utils.Ptr(force),
	}
	repoConfigs, err := repoConfigDao.InternalOnly_ListReposToSnapshot(ctx, filter)

	if err != nil {
		return fmt.Errorf("error getting repository configurations: %w", err)
	}

	for _, repo := range repoConfigs {
		t := queue.Task{
			Typename: config.IntrospectTask,
			Payload: payloads.IntrospectPayload{
				Url: repo.Repository.URL,
			},
			OrgId:      repo.OrgID,
			AccountId:  repo.AccountID,
			ObjectUUID: &repo.RepositoryUUID,
			ObjectType: utils.Ptr(config.ObjectTypeRepository),
		}
		_, err = c.Enqueue(t)
		if err != nil {
			log.Err(err).Msgf("error enqueueing introspection for repository %v", repo.Name)
		}

		t = queue.Task{
			Typename:   config.RepositorySnapshotTask,
			Payload:    payloads.SnapshotPayload{},
			OrgId:      repo.OrgID,
			AccountId:  repo.AccountID,
			ObjectUUID: &repo.RepositoryUUID,
			ObjectType: utils.Ptr(config.ObjectTypeRepository),
		}
		taskUuid, err := c.Enqueue(t)
		if err == nil {
			log.Info().Msgf("enqueued snapshot for repository config %v", repo.UUID)
			if err := repoConfigDao.UpdateLastSnapshotTask(ctx, taskUuid.String(), repo.OrgID, repo.RepositoryUUID); err != nil {
				log.Error().Err(err).Msgf("error UpdatingLastSnapshotTask task during nightly job")
			}
			t = queue.Task{
				Typename:     config.UpdateLatestSnapshotTask,
				Payload:      tasks.UpdateLatestSnapshotPayload{RepositoryConfigUUID: repo.UUID},
				OrgId:        repo.OrgID,
				AccountId:    repo.AccountID,
				ObjectUUID:   &repo.RepositoryUUID,
				ObjectType:   utils.Ptr(config.ObjectTypeRepository),
				Dependencies: []uuid.UUID{taskUuid},
			}
			_, err = c.Enqueue(t)
			if err != nil {
				log.Err(err).Msgf("error enqueueing update-lastest-snapshot for repository %v", repo.Name)
			}
		} else {
			log.Err(err).Msgf("error enqueueing snapshot for repository %v", repo.Name)
		}
	}
	return nil
}
