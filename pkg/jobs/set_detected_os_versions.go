package jobs

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/rs/zerolog/log"
)

func SetDetectedOSVersions(_ []string) {
	err := db.Connect()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}

	err = config.ConfigureTang()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure tang")
	}

	daoReg := dao.GetDaoRegistry(db.DB)
	ctx := context.Background()

	pageData := api.PaginationData{
		Limit: -1,
	}

	repos, _, err := daoReg.RepositoryConfig.List(ctx, config.RedHatOrg, pageData, api.FilterData{Origin: config.OriginRedHat})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to list repos")
	}

	for _, repo := range repos.Data {
		log.Info().Msgf("Updating snapshots for repo: (%v, %v)", repo.Name, repo.UUID)

		snapshots, _, err := daoReg.Snapshot.List(ctx, config.RedHatOrg, repo.UUID, pageData, api.FilterData{})
		if err != nil {
			log.Error().Err(err).Msg("failed to connect to list snapshots")
		}

		for _, snapshot := range snapshots.Data {
			OSVersion, err := daoReg.Snapshot.SetDetectedOSVersion(ctx, snapshot.UUID)
			if err != nil {
				log.Fatal().Err(err).Msgf("failed to set detected os version for snapshot: %v", snapshot.UUID)
			}
			if OSVersion != "" {
				log.Info().Str("os_version", OSVersion).Str("snapshot_uuid", snapshot.UUID).Msg("Successfully detected and set OS version for snapshot")
			}
		}
	}
}
