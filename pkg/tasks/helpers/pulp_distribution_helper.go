package helpers

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2025"
	"golang.org/x/exp/slices"
)

func GetLatestRepoDistPath(repoUUID string) string {
	return fmt.Sprintf("%v/%v", repoUUID, "latest")
}

func NewPulpDistributionHelper(ctx context.Context, client pulp_client.PulpClient) *PulpDistributionHelper {
	return &PulpDistributionHelper{
		pulpClient: client,
		ctx:        ctx,
	}
}

type PulpDistributionHelper struct {
	pulpClient pulp_client.PulpClient
	ctx        context.Context
}

func (pdh *PulpDistributionHelper) CreateDistribution(repo api.RepositoryResponse, publicationHref, distName, distPath string) (*zest.TaskResponse, error) {
	// Create content guard
	var contentGuardHref *string
	if config.Get().Clients.Pulp.RepoContentGuards {
		href, err := pdh.FetchContentGuard(repo.OrgID, repo.FeatureName)
		if err != nil {
			return nil, err
		}
		contentGuardHref = href
	}

	// Create distribution
	distTask, err := pdh.pulpClient.CreateRpmDistribution(pdh.ctx, publicationHref, distName, distPath, contentGuardHref)
	if err != nil {
		return nil, err
	}

	distResp, err := pdh.pulpClient.PollTask(pdh.ctx, *distTask)
	if err != nil {
		return nil, err
	}

	return distResp, nil
}

func (pdh *PulpDistributionHelper) UpdateDistribution(repo api.RepositoryResponse, distHref, publicationHref, distName, distPath string) (*zest.TaskResponse, error) {
	var contentGuardHref *string
	if config.Get().Clients.Pulp.RepoContentGuards {
		fmt.Println("UpdateDistribution Goes into RepoContentGuards")
		href, err := pdh.FetchContentGuard(repo.OrgID, repo.FeatureName)
		fmt.Println("UpdateDistribution guard: ", href)
		if err != nil {
			return nil, err
		}
		contentGuardHref = href
	}
	distTaskHref, err := pdh.pulpClient.UpdateRpmDistribution(pdh.ctx, distHref, publicationHref, distName, distPath, contentGuardHref)
	if err != nil {
		return nil, err
	}
	var distTask *zest.TaskResponse
	if distTaskHref != "" {
		fmt.Println("Should not go into Poll")
		distTask, err = pdh.pulpClient.PollTask(pdh.ctx, distTaskHref)
		if err != nil {
			return nil, err
		}
	}
	fmt.Println("UpdateDistribution taskHref: ", distTaskHref)
	fmt.Println("UpdateDistribution task: ", distTask)
	return distTask, nil
}

func (pdh *PulpDistributionHelper) CreateOrUpdateDistribution(repo api.RepositoryResponse, publicationHref, distName, distPath string) (string, *string, error) {
	fmt.Println("CreateOrUpdateDistribution")
	distTask := &zest.TaskResponse{}
	var distTaskHref string

	resp, err := pdh.pulpClient.FindDistributionByPath(pdh.ctx, distPath)
	if err != nil {
		return "", nil, err
	}

	if resp != nil {
		fmt.Println("CreateOrUpdate path found: ", *resp.PulpHref)
	}

	if resp == nil {
		fmt.Println("Go into create dist")
		distTask, err = pdh.CreateDistribution(repo, publicationHref, distName, distPath)
		if distTask != nil && distTask.PulpHref != nil {
			distTaskHref = *distTask.PulpHref
		}
		if err != nil {
			return "", &distTaskHref, err
		}
		distHrefPtr := pulp_client.SelectRpmDistributionHref(distTask)
		if distHrefPtr == nil {
			return "", &distTaskHref, fmt.Errorf("could not find a distribution href in task: %v", distTask.PulpHref)
		}
		return *distHrefPtr, &distTaskHref, err
	}

	fmt.Println("Go into update dist")

	distTask, err = pdh.UpdateDistribution(repo, *resp.PulpHref, publicationHref, distName, distPath)

	fmt.Println("CreateOrUpdate distTask", distTask)

	if distTask != nil && distTask.PulpHref != nil {
		distTaskHref = *distTask.PulpHref
	}
	if err != nil {
		return "", nil, err
	}
	if distTask == nil {
		fmt.Println("CreateOrUpdate no taskHref to return")
		fmt.Println("CreateOrUpdate PulpHref", *resp.PulpHref)
		fmt.Println("CreateOrUpdate distTaskHref", nil)
		return *resp.PulpHref, nil, err
	}

	return *resp.PulpHref, &distTaskHref, err
}

func (pdh *PulpDistributionHelper) FindOrCreateDistribution(repo api.RepositoryResponse, publicationHref, distName, distPath string) (string, error) {
	resp, err := pdh.pulpClient.FindDistributionByPath(pdh.ctx, distPath)
	if err != nil {
		return "", err
	}
	if resp != nil && resp.PulpHref != nil {
		return *resp.PulpHref, err
	}

	distTask, err := pdh.CreateDistribution(repo, publicationHref, distName, distPath)
	if err != nil {
		return "", err
	}
	distHrefPtr := pulp_client.SelectRpmDistributionHref(distTask)
	if distHrefPtr == nil {
		return "", fmt.Errorf("could not find a distribution href in task: %v", distTask.PulpHref)
	}

	return *distTask.PulpHref, err
}

func (pdh *PulpDistributionHelper) FetchContentGuard(orgId string, feature string) (*string, error) {
	if !config.Get().Clients.Pulp.RepoContentGuards {
		return nil, nil
	}
	if orgId == config.RedHatOrg || orgId == config.CommunityOrg {
		if !slices.Contains(config.SubscriptionFeaturesIgnored, feature) {
			href, err := pdh.pulpClient.CreateOrUpdateGuardsForRhelRepo(pdh.ctx, feature)
			if err != nil {
				return nil, fmt.Errorf("could not fetch/create/update RHEL composite content guard: %w", err)
			}
			return &href, nil
		}
	} else {
		href, err := pdh.pulpClient.CreateOrUpdateGuardsForOrg(pdh.ctx, orgId)
		if err != nil {
			return nil, fmt.Errorf("could not fetch/create/update content guard: %w", err)
		}
		return &href, nil
	}
	return nil, nil
}
