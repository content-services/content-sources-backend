package jobs

import (
	"context"
	"sync"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/tasks/helpers"
	"github.com/rs/zerolog/log"
)

func CreateLatestDistributions(_ []string) {
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
		pulpClient := pulp_client.GetPulpClientWithDomain(domain.DomainName)
		distHelper := helpers.NewPulpDistributionHelper(ctx, pulpClient)

		originFilter := config.OriginExternal + "," + config.OriginUpload
		if domain.OrgId == config.RedHatOrg {
			originFilter += "," + config.OriginRedHat
		} else if domain.OrgId == config.CommunityOrg {
			originFilter += "," + config.OriginCommunity
		}

		pageData := api.PaginationData{Limit: -1}
		filterData := api.FilterData{Origin: originFilter}
		repos, _, err := daoReg.RepositoryConfig.List(ctx, domain.OrgId, pageData, filterData)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to list repos")
		}

		batchSize := 5
		for i := 0; i < batchSize; i += batchSize {
			end := i + batchSize
			if end > len(repos.Data) {
				end = len(repos.Data)
			}
			batch := repos.Data[i:end]
			wg := sync.WaitGroup{}
			for _, repo := range batch {
				lastSnapshot := repo.LastSnapshot
				if lastSnapshot == nil {
					continue
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err = distHelper.FindOrCreateDistribution(
						repo,
						lastSnapshot.PublicationHref,
						repo.UUID,
						helpers.GetLatestRepoDistPath(repo.UUID))
					if err != nil {
						log.Error().Str("repo_uuid", repo.UUID).Str("org_id", domain.OrgId).Err(err).Msg("failed to create distribution")
					}
				}()
			}
			wg.Wait()
		}
	}
}
