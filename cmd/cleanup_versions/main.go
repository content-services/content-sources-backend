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

	if err != nil {
		log.Panic().Err(err).Msg("Failed to connect to database")
	}

	if len(args) < 2 || args[1] != "--force" {
		log.Fatal().Msg("Requires arguments: --force")
	}

	snapshots := []models.Snapshot{}
	result := db.DB.Find(&snapshots)

	if result.Error != nil {
		log.Logger.Fatal().Err(result.Error).Msgf("Cannot load repos")
	}
	ctx := context.Background()

	domainCleanupTasks := make(map[string]string)

	for _, snap := range snapshots {
		domainName, err := GetDomainName(snap)
		domainCleanupTasks[domainName] = ""
		if err != nil {
			log.Logger.Error().Err(err).Msgf("Could not get domain name for snap: %v", snap.UUID)
		}
		err = DeleteSnapshot(ctx, domainName, snap)
		if err != nil {
			log.Logger.Error().Err(err).Msgf("Could not delete snapshot: %v", snap.UUID)
		}
	}

	// Spawn all the cleanup tasks
	for name := range domainCleanupTasks {
		taskUrl, err := pulp_client.GetPulpClientWithDomain(ctx, name).OrphanCleanup()
		if err == nil {
			domainCleanupTasks[name] = taskUrl
		} else {
			log.Logger.Error().Err(err).Msgf("Orphan deletion for domain failed: %v", name)
		}
	}

	// Now poll for their completion
	for name, taskUrl := range domainCleanupTasks {
		if taskUrl != "" {
			_, err = pulp_client.GetPulpClientWithDomain(ctx, name).PollTask(taskUrl)
			if err != nil {
				log.Logger.Error().Err(err).Msgf("Orphan deletion for domain failed: %v", name)
			}
		}
	}
}

func GetDomainName(snap models.Snapshot) (string, error) {
	domainDao := dao.GetDomainDao(db.DB)
	repoConfig := models.RepositoryConfiguration{}
	result := db.DB.Where("uuid = ?", snap.RepositoryConfigurationUUID).First(&repoConfig)

	if result.Error != nil {
		return "", fmt.Errorf("error looking up repo configuration: %w", result.Error)
	}
	domainName, err := domainDao.Fetch(repoConfig.OrgID)
	if err != nil {
		return "", err
	}
	if domainName == "" {
		return "", fmt.Errorf("Could not find domain for org %v", repoConfig.OrgID)
	}
	return domainName, nil
}

func DeleteSnapshot(ctx context.Context, domainName string, snap models.Snapshot) error {
	pulp := pulp_client.GetPulpClientWithDomain(ctx, domainName)
	versionDelHref, err := pulp.DeleteRpmRepositoryVersion(snap.VersionHref)
	if err != nil {
		return fmt.Errorf("error starting version deletion: %w", err)
	}

	distDelHref, err := pulp.DeleteRpmDistribution(snap.DistributionHref)
	if err != nil {
		return fmt.Errorf("error starting version deletion: %w", err)
	}
	if versionDelHref != "" {
		_, err = pulp.PollTask(versionDelHref)
		if err != nil {
			return fmt.Errorf("version deletion failed for: %v", versionDelHref)
		}
	}
	if distDelHref != "" {
		_, err = pulp.PollTask(distDelHref)
		if err != nil {
			return fmt.Errorf("distribution deletion failed for: %v", versionDelHref)
		}
	}

	result := db.DB.Delete(&snap)
	if result.Error != nil {
		return fmt.Errorf("error deleting snapshot from db: %w", err)
	}
	log.Logger.Debug().Msgf("Deleting snpashot: %v", snap.UUID)
	return nil
}
