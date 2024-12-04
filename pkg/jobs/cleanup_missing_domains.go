package jobs

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
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
		}
		if pulpHref == "" {
			err := daoReg.Domain.Delete(ctx, domain.OrgId, domain.DomainName)
			if err != nil {
				log.Error().Err(err).Msg("failed to delete domain")
			}
		}
	}
}
