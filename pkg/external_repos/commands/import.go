package commands

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
	"gorm.io/gorm"
)

func ImportAction(c *cli.Context) error {
	ctx := c.Context

	err := importRepos(ctx, db.DB)
	if err != nil {
		log.Panic().Err(err).Msg("Failed to save repositories")
	}
	log.Debug().Msg("Successfully loaded external repositories.")
	return nil
}

func importRepos(ctx context.Context, db *gorm.DB) error {
	daoReg := dao.GetDaoRegistry(db)
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
	err = daoReg.RepositoryConfig.SavePublicRepos(ctx, urls)
	if err != nil {
		return err
	}

	rh := external_repos.NewRedHatRepos(daoReg)
	err = rh.LoadAndSave(ctx)
	if err != nil {
		return err
	}
	err = deleteNoLongerNeededRepos(ctx, daoReg)
	if err != nil {
		return err
	}
	return err
}

func deleteNoLongerNeededRepos(ctx context.Context, daoReg *dao.DaoRegistry) error {
	q, err := queue.NewPgQueue(ctx, db.GetUrl())
	if err != nil {
		return fmt.Errorf("error getting new task queue: %w", err)
	}
	defer q.Close()
	c := client.NewTaskClient(&q)

	urls := []string{
		"https://cdn.redhat.com/content/dist/layered/rhel8/x86_64/ansible/2/os/",
		"https://cdn.redhat.com/content/dist/layered/rhel8/aarch64/ansible/2/os/",
	}
	for _, url := range urls {
		results, _, err := daoReg.RepositoryConfig.List(ctx, config.RedHatOrg, api.PaginationData{Limit: 1},
			api.FilterData{URL: url, Origin: config.OriginRedHat})
		if err != nil {
			return fmt.Errorf("could not list repositories: %v", err)
		}
		if len(results.Data) == 1 && results.Data[0].URL == url {
			repo := results.Data[0]
			err = daoReg.RepositoryConfig.SoftDelete(ctx, config.RedHatOrg, results.Data[0].UUID)
			if err != nil {
				return fmt.Errorf("could not soft delete repository for url (%v): %v", url, err)
			}
			payload := tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repo.UUID}
			task := queue.Task{
				Typename:   config.DeleteRepositorySnapshotsTask,
				Payload:    payload,
				OrgId:      config.RedHatOrg,
				AccountId:  repo.AccountID,
				ObjectUUID: &repo.RepositoryUUID,
				ObjectType: utils.Ptr(config.ObjectTypeRepository),
			}
			_, err := c.Enqueue(task)
			if err != nil {
				return fmt.Errorf("could not enqueue task for repo deletion (%v): %v", url, err)
			}

			// Mark as not public, so orphan cleanup will clean it up later
			err = daoReg.Repository.MarkAsNotPublic(ctx, url)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
