package jobs

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/rs/zerolog/log"
)

func CleanupMissingDomains() {
	err := db.Connect()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}

	daoReg := dao.GetDaoRegistry(db.DB)
	ctx := context.Background()

	domains, err := daoReg.Domain.List(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to list domains")
	}
	for _, domain := range domains {
		pulpHref, err := pulp_client.GetGlobalPulpClient().LookupDomain(ctx, domain.DomainName)
		if err != nil {
			log.Error().Err(err).Msg("failed to lookup pulp domain")
		} else {
			if pulpHref == "" {
				var snapCount int64 = 0
				result := db.DB.Model(models.Snapshot{}).
					Joins("inner join repository_configurations on repository_configurations.uuid = snapshots.repository_configuration_uuid").
					Where("org_id = ?", domain.OrgId).Count(&snapCount)
				if result.Error != nil {
					log.Fatal().Err(err).Msg("failed to fetch snapshots for org")
				}
				if snapCount > 0 {
					log.Error().Err(err).Msg("skipping domain deletion, snapshots exist in this org")
				} else {
					err := daoReg.Domain.Delete(ctx, domain.OrgId, domain.DomainName)
					if err != nil {
						log.Error().Err(err).Msg("failed to delete domain")
					}
				}
			}
		}
	}
}
