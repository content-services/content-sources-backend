package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func main() {
	args := os.Args
	config.Load()
	config.ConfigureLogging()

	err := db.Connect()
	ctx := context.Background()
	if err != nil {
		log.Panic().Err(err).Msg("Failed to connect to database")
	}

	dao.SetupGormTableOrFail(db.DB)

	if len(args) < 2 {
		log.Fatal().Msg("Requires arguments: download, import, introspect, snapshot, nightly-jobs [INTERVAL], pulp-orphan-cleanup [BATCH_SIZE]")
	}
	if args[1] == "download" {
		if len(args) < 3 {
			log.Fatal().Msg("Usage:  ./external-repos download /path/to/jsons/")
		}
		scanForExternalRepos(args[2])
	} else if args[1] == "import" {
		config.Load()
		err := db.Connect()
		if err != nil {
			log.Panic().Err(err).Msg("Failed to save repositories")
		}
		err = saveToDB(ctx, db.DB)
		if err != nil {
			log.Panic().Err(err).Msg("Failed to save repositories")
		}
		log.Debug().Msg("Successfully loaded external repositories.")
	} else if args[1] == "introspect" {
		if len(args) < 3 {
			log.Error().Msg("Usage:  ./external_repos introspect [--force] URL [URL2]...")
			os.Exit(1)
		}
		var urls []string
		forceIntrospect := false
		for i := 2; i < len(args); i++ {
			if args[i] != "--force" {
				urls = append(urls, args[i])
			} else {
				forceIntrospect = true
			}
		}
		introspectUrls(ctx, urls, forceIntrospect)
	} else if args[1] == "snapshot" {
		if len(args) < 3 {
			log.Error().Msg("Usage:  ./external_repos snapshot URL [URL2]...")
			os.Exit(1)
		}
		var urls []string
		for i := 2; i < len(args); i++ {
			urls = append(urls, args[i])
		}
		if config.Get().Features.Snapshots.Enabled {
			waitForPulp(ctx)
			err := enqueueSnapshotRepos(ctx, &urls, nil)
			if err != nil {
				log.Warn().Msgf("Error enqueuing snapshot tasks: %v", err)
			}
		} else {
			log.Warn().Msg("Snapshotting disabled")
		}
	} else if args[1] == "nightly-jobs" {
		err = enqueueIntrospectAllRepos(ctx)
		if err != nil {
			log.Error().Err(err).Msg("error queueing introspection tasks")
		}
		if config.Get().Features.Snapshots.Enabled {
			var interval *int
			if len(args) > 2 {
				parsed, err := strconv.ParseInt(args[2], 10, 0)
				if err != nil {
					log.Logger.Fatal().Err(err).Msgf("could not parse integer interval %v", args[2])
				}
				interval = utils.Ptr(int(parsed))
			}
			waitForPulp(ctx)
			err = enqueueSnapshotRepos(ctx, nil, interval)
			if err != nil {
				log.Error().Err(err).Msg("error queueing snapshot tasks")
			}
		}
	} else if args[1] == "pulp-orphan-cleanup" {
		batchSize := 5
		if len(args) > 2 {
			parsed, err := strconv.ParseInt(args[2], 10, 0)
			if err != nil {
				log.Logger.Fatal().Err(err).Msgf("could not parse integer interval %v", args[2])
			}
			batchSize = int(parsed)
		}
		if !config.PulpConfigured() {
			log.Error().Msg("cannot run orphan cleanup if pulp is not configured")
		}
		err := pulpOrphanCleanup(ctx, db.DB, batchSize)
		if err != nil {
			log.Error().Err(err).Msg("error starting pulp orphan cleanup tasks")
		}
	}
}

func saveToDB(ctx context.Context, db *gorm.DB) error {
	dao := dao.GetDaoRegistry(db)
	var (
		err      error
		extRepos []external_repos.ExternalRepository
		urls     []string
	)
	extRepos, err = external_repos.LoadFromFile()

	if err != nil {
		return err
	}
	urls = external_repos.GetBaseURLs(extRepos)
	err = dao.RepositoryConfig.SavePublicRepos(ctx, urls)
	if err != nil {
		return err
	}

	rh := external_repos.NewRedHatRepos(dao)
	err = rh.LoadAndSave(ctx)
	return err
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

func introspectUrls(ctx context.Context, urls []string, force bool) {
	repos, err := dao.GetDaoRegistry(db.DB).Repository.ListForIntrospection(ctx, &urls, force)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not lookup repos to introspect")
	}
	for _, repo := range repos {
		count, introError, error := external_repos.IntrospectUrl(ctx, repo.URL)
		if introError != nil {
			log.Warn().Msgf("Introspection Error: %v", introError)
		}
		if error != nil {
			log.Panic().Err(error).Msg("Failed to introspect repository due to fatal errors")
		}
		log.Debug().Msgf("Inserted %d packages for %v", count, repo.URL)
	}
}

func scanForExternalRepos(path string) {
	urls, err := external_repos.IBUrlsFromDir(path)
	if err != nil {
		log.Panic().Err(err).Msg("Failed to import repositories")
	}
	sort.Strings(urls)
	err = external_repos.SaveToFile(urls)
	if err != nil {
		log.Panic().Err(err).Msg("Failed to import repositories")
	}
	log.Info().Msg("Saved External Repositories")
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

func enqueueSnapshotRepos(ctx context.Context, urls *[]string, interval *int) error {
	q, err := queue.NewPgQueue(ctx, db.GetUrl())
	if err != nil {
		return fmt.Errorf("error getting new task queue: %w", err)
	}
	defer q.Close()
	c := client.NewTaskClient(&q)

	repoConfigDao := dao.GetRepositoryConfigDao(db.DB, pulp_client.GetPulpClientWithDomain(""))
	filter := &dao.ListRepoFilter{
		URLs:            urls,
		RedhatOnly:      utils.Ptr(urls != nil),
		MinimumInterval: interval,
	}
	repoConfigs, err := repoConfigDao.InternalOnly_ListReposToSnapshot(ctx, filter)

	if err != nil {
		return fmt.Errorf("error getting repository configurations: %w", err)
	}

	for _, repo := range repoConfigs {
		t := queue.Task{
			Typename:   config.RepositorySnapshotTask,
			Payload:    payloads.SnapshotPayload{},
			OrgId:      repo.OrgID,
			AccountId:  repo.AccountID,
			ObjectUUID: &repo.RepositoryUUID,
			ObjectType: utils.Ptr(config.ObjectTypeRepository),
		}
		taskUuid, err := c.Enqueue(t)
		if err == nil {
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
