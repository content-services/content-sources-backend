package helpers

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	zest "github.com/content-services/zest/release/v2024"
)

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

func (pdh *PulpDistributionHelper) CreateDistribution(orgID, publicationHref, distName, distPath string) (*zest.TaskResponse, error) {
	// Create content guard
	var contentGuardHref *string
	if orgID != config.RedHatOrg && config.Get().Clients.Pulp.CustomRepoContentGuards {
		href, err := pdh.pulpClient.CreateOrUpdateGuardsForOrg(pdh.ctx, orgID)
		if err != nil {
			return nil, fmt.Errorf("could not fetch/create/update content guard: %w", err)
		}
		contentGuardHref = &href
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

func (pdh *PulpDistributionHelper) CreateOrUpdateDistribution(orgId, distName, distPath, publicationHref string) error {
	resp, err := pdh.pulpClient.FindDistributionByPath(pdh.ctx, distPath)
	if err != nil {
		return err
	}

	if resp == nil {
		_, err := pdh.CreateDistribution(orgId, publicationHref, distName, distPath)
		if err != nil {
			return err
		}
	} else {
		contentGuardHref, err := pdh.FetchContentGuard(orgId)
		if err != nil {
			return err
		}
		taskHref, err := pdh.pulpClient.UpdateRpmDistribution(pdh.ctx, *resp.PulpHref, publicationHref, distName, distPath, contentGuardHref)
		if err != nil {
			return err
		}

		_, err = pdh.pulpClient.PollTask(pdh.ctx, taskHref)
		if err != nil {
			return err
		}
	}
	return nil
}

func (pdh *PulpDistributionHelper) FetchContentGuard(orgId string) (*string, error) {
	if orgId != config.RedHatOrg && config.Get().Clients.Pulp.CustomRepoContentGuards {
		href, err := pdh.pulpClient.CreateOrUpdateGuardsForOrg(pdh.ctx, orgId)
		if err != nil {
			return nil, fmt.Errorf("could not fetch/create/update content guard: %w", err)
		}
		return &href, nil
	}
	return nil, nil
}
