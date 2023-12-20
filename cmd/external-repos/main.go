package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/openlyinc/pointy"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func main() {
	args := os.Args
	config.Load()
	config.ConfigureLogging()

	err := db.Connect()
	if err != nil {
		log.Panic().Err(err).Msg("Failed to connect to database")
	}

	if len(args) < 2 {
		log.Fatal().Msg("Requires arguments: download, import, introspect, snapshot, nightly-jobs")
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
		err = saveToDB(db.DB)
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
		introspectUrls(urls, forceIntrospect)
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
			waitForPulp()
			err := enqueueSnapshotRepos(&urls)
			if err != nil {
				log.Warn().Msgf("Error enqueuing snapshot tasks: %v", err)
			}
		} else {
			log.Warn().Msg("Snapshotting disabled")
		}
	} else if args[1] == "nightly-jobs" {
		err = enqueueIntrospectAllRepos()
		if err != nil {
			log.Error().Err(err).Msg("error queueing introspection tasks")
		}
		if config.Get().Features.Snapshots.Enabled {
			err = enqueueSnapshotRepos(nil)
			if err != nil {
				log.Error().Err(err).Msg("error queueing snapshot tasks")
			}
		}
	}
}

func saveToDB(db *gorm.DB) error {
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
	err = dao.RepositoryConfig.SavePublicRepos(urls)
	if err != nil {
		return err
	}

	rh := external_repos.NewRedHatRepos(dao)
	err = rh.LoadAndSave()
	return err
}

func waitForPulp() {
	for {
		client := pulp_client.GetPulpClientWithDomain(context.Background(), pulp_client.DefaultDomain)
		_, err := client.GetRpmRemoteList()
		if err == nil {
			return
		}
		log.Warn().Err(err).Msg("Pulp isn't up yet, waiting 5s.")
		time.Sleep(5 * time.Second)
	}
}

func introspectUrls(urls []string, force bool) {
	repos, err := dao.GetDaoRegistry(db.DB).Repository.ListForIntrospection(&urls, force)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not lookup repos to introspect")
	}
	for _, repo := range repos {
		count, introError, error := external_repos.IntrospectUrl(context.Background(), repo.URL)
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

func enqueueIntrospectAllRepos() error {
	q, err := queue.NewPgQueue(db.GetUrl())
	if err != nil {
		return fmt.Errorf("error getting new task queue: %w", err)
	}
	c := client.NewTaskClient(&q)

	repoDao := dao.GetRepositoryDao(db.DB)
	err = repoDao.OrphanCleanup()
	if err != nil {
		log.Err(err).Msg("error during orphan cleanup")
	}
	err = dao.GetTaskInfoDao(db.DB).Cleanup()
	if err != nil {
		log.Err(err).Msg("error during task cleanup")
	}

	repos, err := repoDao.ListForIntrospection(nil, false)
	if err != nil {
		return fmt.Errorf("error getting repositories: %w", err)
	}
	for _, repo := range repos {
		t := queue.Task{
			Typename: payloads.Introspect,
			Payload: payloads.IntrospectPayload{
				Url: repo.URL,
			},
			RepositoryUUID: repo.UUID,
		}
		_, err = c.Enqueue(t)
		if err != nil {
			log.Err(err).Msgf("error enqueueing introspecting for repository %v", repo.URL)
		}
	}

	return nil
}

func enqueueSnapshotRepos(urls *[]string) error {
	q, err := queue.NewPgQueue(db.GetUrl())
	if err != nil {
		return fmt.Errorf("error getting new task queue: %w", err)
	}
	c := client.NewTaskClient(&q)

	repoConfigDao := dao.GetRepositoryConfigDao(db.DB, pulp_client.GetPulpClientWithDomain(context.Background(), ""))
	var filter *dao.ListRepoFilter
	if urls != nil {
		filter = &dao.ListRepoFilter{
			URLs:       urls,
			RedhatOnly: pointy.Pointer(true),
		}
	}
	repoConfigs, err := repoConfigDao.InternalOnly_ListReposToSnapshot(filter)

	if err != nil {
		return fmt.Errorf("error getting repository configurations: %w", err)
	}

	for _, repo := range repoConfigs {
		t := queue.Task{
			Typename:       config.RepositorySnapshotTask,
			Payload:        payloads.SnapshotPayload{},
			OrgId:          repo.OrgID,
			AccountId:      repo.AccountID,
			RepositoryUUID: repo.RepositoryUUID,
		}
		taskUuid, err := c.Enqueue(t)
		if err == nil {
			if err := repoConfigDao.UpdateLastSnapshotTask(taskUuid.String(), repo.OrgID, repo.RepositoryUUID); err != nil {
				log.Error().Err(err).Msgf("error UpdatingLastSnapshotTask task during nightly job")
			}
		} else {
			log.Err(err).Msgf("error enqueueing snapshot for repository %v", repo.Name)
		}
	}
	return nil
}
