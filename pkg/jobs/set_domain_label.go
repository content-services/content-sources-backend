package jobs

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/rs/zerolog/log"
)

func SetDomainLabel() {
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
		if err != nil || len(pulpHref) == 0 {
			log.Error().Err(err).Msgf("failed to lookup pulp domain, for domain with name: %s", domain.DomainName)
			continue
		}

		err = pulp_client.GetGlobalPulpClient().SetDomainLabel(ctx, pulpHref, "contentsources", "true")
		if err != nil {
			log.Error().Err(err).Msgf("failed to set domain label, for domain with name: %s", domain.DomainName)
		}
	}
}
