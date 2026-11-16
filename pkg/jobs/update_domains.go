package jobs

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/rs/zerolog/log"
)

// UpdateDomains updates all pulp domains
func UpdateDomains(_ []string) {
	err := db.Connect()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}

	daoReg := dao.GetDaoRegistry(db.DB)
	updateDomainsRun(context.Background(), daoReg.Domain, pulp_client.GetGlobalPulpClient())
}

// updateDomainsWithDeps is a testable function that accepts dependencies
func updateDomainsRun(ctx context.Context, domainDao dao.DomainDao, pulpClient pulp_client.PulpGlobalClient) {
	domains, err := domainDao.List(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to list domains")
	}

	log.Info().Msgf("Updating %d domains", len(domains))

	successCount := 0
	errorCount := 0

	for _, domain := range domains {
		log.Debug().Str("domain_name", domain.DomainName).Str("org_id", domain.OrgId).Msg("Updating domain")

		err := pulpClient.UpdateDomainIfNeeded(ctx, domain.DomainName)
		if err != nil {
			log.Error().Err(err).Str("domain_name", domain.DomainName).Str("org_id", domain.OrgId).Msg("Failed to update domain")
			errorCount++
		} else {
			log.Debug().Str("domain_name", domain.DomainName).Str("org_id", domain.OrgId).Msg("Successfully updated domain")
			successCount++
		}
	}

	log.Info().Msgf("Updated %d/%d domains successfully, %d errors", successCount, len(domains), errorCount)
}
