package commands

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

func ProcessReposAction(c *cli.Context) error {
	ctx := c.Context
	var interval *int
	if c.IsSet("interval") {
		interval = utils.Ptr(c.Int("interval"))
	}

	err := enqueueIntrospectAllRepos(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error queueing introspection tasks")
	}
	if config.Get().Features.Snapshots.Enabled {
		waitForPulp(ctx)
		err = enqueueSnapshotRepos(ctx, nil, interval, false)
		if err != nil {
			log.Error().Err(err).Msg("error queueing snapshot tasks")
		}
	}
	return nil
}

func enqueueIntrospectAllRepos(ctx context.Context) error {
	q, err := queue.NewPgQueue(ctx, db.GetUrl())
	if err != nil {
		return fmt.Errorf("error getting new task queue: %w", err)
	}
	defer q.Close()
	c := client.NewTaskClient(&q)

	repoDao := dao.GetRepositoryDao(db.DB)
	err = repoDao.OrphanCleanup(ctx)
	if err != nil {
		log.Err(err).Msg("error during orphan cleanup")
	}
	err = dao.GetTaskInfoDao(db.DB).Cleanup(ctx)
	if err != nil {
		log.Err(err).Msg("error during task cleanup")
	}

	repos, err := repoDao.ListForIntrospection(ctx, nil, false)
	if err != nil {
		return fmt.Errorf("error getting repositories: %w", err)
	}
	for _, repo := range repos {
		t := queue.Task{
			Typename: payloads.Introspect,
			Payload: payloads.IntrospectPayload{
				Url: repo.URL,
			},
			ObjectUUID: &repo.UUID,
			ObjectType: utils.Ptr(config.ObjectTypeRepository),
		}
		_, err = c.Enqueue(t)
		if err != nil {
			log.Err(err).Msgf("error enqueueing introspecting for repository %v", repo.URL)
		}
	}

	return nil
}
