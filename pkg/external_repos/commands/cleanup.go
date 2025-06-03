package commands

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
	"gorm.io/gorm"
)

var allTypes = []string{"repository", "task", "snapshot", "upload", "pulp-orphan"}

func CleanupUsage() string {
	return fmt.Sprintf("Run the cleanup tasks [%v]", strings.Join(allTypes, ", "))
}

func CleanupAction(c *cli.Context) error {
	var err error
	ctx := c.Context
	types := c.StringSlice("type")
	exclude := c.StringSlice("exclude")
	pulpOrphanBatchSize := c.Int("pulp-orphan-batch-size")

	if len(types) > 0 && len(exclude) > 0 {
		return fmt.Errorf("--type and --exclude are mutually exclusive")
	}

	var cleanupTypesToRun []string
	if len(types) > 0 {
		cleanupTypesToRun = types
	} else if len(exclude) > 0 {
		cleanupTypesToRun = utils.SubtractSlices(allTypes, exclude)
	} else {
		cleanupTypesToRun = allTypes
	}

	for _, t := range cleanupTypesToRun {
		switch t {
		case "repository":
			log.Info().Msg("=== Running repository cleanup ===")
			err := dao.GetRepositoryDao(db.DB).OrphanCleanup(ctx)
			if err != nil {
				log.Err(err).Msg("error during orphan cleanup")
			}

		case "task":
			log.Info().Msg("=== Running task cleanup ===")
			err = dao.GetTaskInfoDao(db.DB).Cleanup(ctx)
			if err != nil {
				log.Err(err).Msg("error during task cleanup")
			}

		case "snapshot":
			if config.Get().Features.Snapshots.Enabled {
				log.Info().Msg("=== Running snapshot cleanup ===")
				snapshotRetainDaysLimit := config.Get().Options.SnapshotRetainDaysLimit
				err = enqueueSnapshotsCleanup(ctx, snapshotRetainDaysLimit)
				if err != nil {
					log.Error().Err(err).Msg("error queueing delete snapshot tasks for snapshot cleanup")
				}
			} else {
				log.Warn().Msg("Snapshotting disabled")
			}

		case "upload":
			log.Info().Msg("=== Running upload cleanup ===")
			err = uploadCleanup(ctx, db.DB)
			if err != nil {
				log.Error().Err(err).Msgf("error starting upload cleanup tasks")
			}

		case "pulp-orphan":
			log.Info().Msg("=== Running pulp orphan cleanup ===")
			batchSize := 5
			if pulpOrphanBatchSize > 0 {
				batchSize = pulpOrphanBatchSize
			}
			if !config.PulpConfigured() {
				log.Error().Msg("cannot run orphan cleanup if pulp is not configured")
			}
			err = pulpOrphanCleanup(ctx, db.DB, batchSize)
			if err != nil {
				log.Error().Err(err).Msg("error starting pulp orphan cleanup tasks")
			}
		}
	}
	return nil
}

func enqueueSnapshotsCleanup(ctx context.Context, olderThanDays int) error {
	q, err := queue.NewPgQueue(ctx, db.GetUrl())
	if err != nil {
		return fmt.Errorf("error getting new task queue: %w", err)
	}
	defer q.Close()
	c := client.NewTaskClient(&q)
	daoReg := dao.GetDaoRegistry(db.DB)
	repoConfigs, err := daoReg.RepositoryConfig.ListReposWithOutdatedSnapshots(ctx, olderThanDays)
	if err != nil {
		return fmt.Errorf("error getting repository configurations: %v", err)
	}
	for _, repo := range repoConfigs {
		err := enqueueSnapshotCleanupForRepoConfig(ctx, c, daoReg, olderThanDays, repo)
		if err != nil {
			log.Err(err).Msgf("error cleaning snapshot for repository %v (%v)", repo.Name, repo.UUID)
		}
	}
	return nil
}

func enqueueSnapshotCleanupForRepoConfig(ctx context.Context, taskClient client.TaskClient, daoReg *dao.DaoRegistry, olderThanDays int, repo models.RepositoryConfiguration) error {
	// Fetch snapshots for repo and find those which are to be deleted
	snaps, err := daoReg.Snapshot.FetchForRepoConfigUUID(ctx, repo.UUID, true)
	if err != nil {
		return fmt.Errorf("error fetching snapshots for repository %v", repo.Name)
	}
	if len(snaps) < 2 {
		log.Warn().Msgf("Skipping snapshot for repository %v", repo.Name)
		return nil
	}

	slices.SortFunc(snaps, func(s1, s2 models.Snapshot) int {
		return s1.CreatedAt.Compare(s2.CreatedAt)
	})
	toBeDeletedSnapUUIDs := make([]string, 0, len(snaps))
	for i, snap := range snaps {
		if i == len(snaps)-1 && len(toBeDeletedSnapUUIDs) == len(snaps)-1 {
			break
		}
		if snap.CreatedAt.Before(time.Now().Add(-time.Duration(olderThanDays) * 24 * time.Hour)) {
			toBeDeletedSnapUUIDs = append(toBeDeletedSnapUUIDs, snap.UUID)
		}
	}
	if len(toBeDeletedSnapUUIDs) == 0 {
		return fmt.Errorf("no outdated snapshot found for repository %v", repo.Name)
	}

	// Check for a running delete task
	inProgressTasks, err := daoReg.TaskInfo.FetchActiveTasks(ctx, repo.OrgID, repo.UUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask)
	if err != nil {
		return fmt.Errorf("error fetching delete repository snapshots task for repository %v", repo.Name)
	}
	if len(inProgressTasks) >= 1 {
		return fmt.Errorf("error, delete is already in progress for repository %v", repo.Name)
	}

	// Soft delete to-be-deleted snapshots
	for _, s := range toBeDeletedSnapUUIDs {
		err := daoReg.Snapshot.SoftDelete(ctx, s)
		if err != nil {
			return fmt.Errorf("could not soft delete snapshot: %w", err)
		}
	}

	// Enqueue new delete task
	t := queue.Task{
		Typename: config.DeleteSnapshotsTask,
		Payload: payloads.DeleteSnapshotsPayload{
			RepoUUID:       repo.UUID,
			SnapshotsUUIDs: toBeDeletedSnapUUIDs,
		},
		OrgId:      repo.OrgID,
		AccountId:  repo.AccountID,
		ObjectUUID: &repo.RepositoryUUID,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
	}

	_, err = taskClient.Enqueue(t)
	if err != nil {
		return fmt.Errorf("error enqueueing delete snapshots task for repository %v", repo.Name)
	}
	return nil
}

func pulpOrphanCleanup(ctx context.Context, db *gorm.DB, batchSize int) error {
	var err error
	daoReg := dao.GetDaoRegistry(db)

	domains, err := daoReg.Domain.List(ctx)
	if err != nil {
		log.Panic().Err(err).Msg("orphan cleanup error: error listing orgs")
	}

	for i := 0; i < len(domains); i += batchSize {
		end := i + batchSize
		if end > len(domains) {
			end = len(domains)
		}
		batch := domains[i:end]
		wg := sync.WaitGroup{}
		for _, domain := range batch {
			org := domain.OrgId
			domainName := domain.DomainName

			logger := log.Logger.With().Str("org_id", org).Str("pulp_domain_name", domainName).Logger()

			wg.Add(1)
			go func() {
				defer wg.Done()

				pulpClient := pulp_client.GetPulpClientWithDomain(domainName)
				cleanupTask, err := pulpClient.OrphanCleanup(ctx)
				if err != nil {
					logger.Error().Err(err).Msgf("error starting orphan cleanup")
					return
				}
				logger.Info().Str("task_href", cleanupTask).Msgf("running orphan cleanup for org: %v", org)

				_, err = pulp_client.GetGlobalPulpClient().PollTask(ctx, cleanupTask)
				if err != nil {
					logger.Error().Err(err).Msgf("error polling pulp task for orphan cleanup")
					return
				}
			}()
		}
		wg.Wait()
	}
	return nil
}

func uploadCleanup(ctx context.Context, db *gorm.DB) error {
	var err error
	daoReg := dao.GetDaoRegistry(db)

	uploads, err := daoReg.Uploads.ListUploadsForCleanup(ctx)
	if err != nil {
		return fmt.Errorf("error listing uploads for cleanup: %w", err)
	}

	var cleanupCounter int
	for _, upload := range uploads {
		orgID := upload.OrgID

		domainName, err := daoReg.Domain.Fetch(ctx, orgID)
		if err != nil {
			log.Error().Err(err).Msgf("error fetching domain name for org %v", orgID)
			continue
		}
		logger := log.Logger.With().Str("org_id", orgID).Str("pulp_domain_name", domainName).Logger()
		pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

		uploadHref := "/api/pulp/" + domainName + "/api/v3/uploads/" + upload.UploadUUID + "/"
		_, err = pulpClient.DeleteUpload(ctx, uploadHref)
		if err != nil {
			logger.Error().Err(err).Msgf("error deleting pulp upload with uuid: %v", upload.UploadUUID)
			continue
		}

		err = daoReg.Uploads.DeleteUpload(ctx, upload.UploadUUID)
		if err != nil {
			logger.Error().Err(err).Msgf("error deleting upload")
			continue
		}
		cleanupCounter++
	}
	log.Info().Msgf("Cleaned up %v uploads", cleanupCounter)
	return nil
}
