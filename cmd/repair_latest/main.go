package main

import (
	"context"
	"fmt"
	"os"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
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

	if len(args) < 2 || args[1] != "--force" {
		log.Fatal().Msg("Requires arguments: --force")
	}

	daoReg := dao.GetDaoRegistry(db.DB)
	repoConfigs := []models.RepositoryConfiguration{}
	result := db.DB.Where("org_id = ?", config.RedHatOrg).Preload("Repository").Preload("LastSnapshot").Find(&repoConfigs)
	if result.Error != nil {
		log.Panic().Err(err).Msg("Failed to list repo configs")
	}
	domainName, err := daoReg.Domain.Fetch(ctx, config.RedHatOrg)
	if err != nil {
		log.Panic().Err(err).Msg("Failed to lookup domain name")
	}
	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)
	tasks := []string{}
	for _, repoConfig := range repoConfigs {
		repairTasks, err := repairRepo(ctx, daoReg, pulpClient, repoConfig)
		if err != nil {
			log.Logger.Error().Err(err).Msgf("could not repair repo %v (%v)", repoConfig.Name, repoConfig.UUID)
		}
		tasks = append(tasks, repairTasks...)
	}
	for i, task := range tasks {
		log.Logger.Debug().Msgf("Polling task %v of %v", i, len(tasks))
		_, err = pulpClient.PollTask(ctx, task)
		if err != nil {
			log.Logger.Error().Err(err).Msg("failed polling task")
		}
	}
}

func repairRepo(ctx context.Context, daoReg *dao.DaoRegistry, pulpClient pulp_client.PulpClient, repoConfig models.RepositoryConfiguration) ([]string, error) {
	tasks := []string{}
	repoResp, err := pulpClient.GetRpmRepositoryByName(ctx, repoConfig.UUID)
	if err != nil {
		return tasks, fmt.Errorf("error fetching repository by name: %w", err)
	}
	if repoResp == nil {
		return tasks, fmt.Errorf("requested Repository is not found")
	}
	if repoResp.LatestVersionHref != nil {
		task, err := pulpClient.RepairRpmRepositoryVersion(ctx, *repoResp.LatestVersionHref)
		if err != nil {
			return tasks, fmt.Errorf("trror starting repair: %w", err)
		}
		tasks = append(tasks, task)

		// If the repo doesn't have a last snapshot, or the latestVersion doesn't match, try to delete it, if it's not version zero
		if repoConfig.LastSnapshot == nil || *repoResp.LatestVersionHref != repoConfig.LastSnapshot.VersionHref {
			versionResp, err := pulpClient.GetRpmRepositoryVersion(ctx, *repoResp.LatestVersionHref)
			if err != nil {
				return tasks, fmt.Errorf("couldn't get repo version: %w", err)
			}
			if versionResp.Number != nil && *versionResp.Number > int64(0) {
				task, err := pulpClient.DeleteRpmRepositoryVersion(ctx, *repoResp.LatestVersionHref)
				if err != nil {
					return tasks, fmt.Errorf("couldn't delete repo version: %w", err)
				}
				tasks = append(tasks, task)
			}
		}
	}

	return tasks, nil
}
