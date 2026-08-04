package helpers

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/utils"
	zest "github.com/content-services/zest/release/v2026"
)

func GetLatestRepoDistPath(repoUUID string) string {
	return fmt.Sprintf("%v/%v", repoUUID, "latest")
}

// ShouldUpdateLatestDistributionOnCreate reports whether creating a snapshot should update
// the repository's Pulp "latest" distribution. Partner repos skip this: their "latest" always
// tracks the newest published snapshot (or is removed when none exist).
func ShouldUpdateLatestDistributionOnCreate(repo api.RepositoryResponse) bool {
	return !repo.Partner
}

// CreateOrUpdateLatestDistribution points the repo's "latest" distribution at publicationHref.
func (pdh *PulpDistributionHelper) CreateOrUpdateLatestDistribution(repo api.RepositoryResponse, publicationHref string) (string, string, error) {
	return pdh.CreateOrUpdateDistribution(repo, publicationHref, repo.UUID, GetLatestRepoDistPath(repo.UUID))
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
	contentGuardHref, err := pdh.FetchContentGuard(repo.OrgID, repo.FeatureName)
	if err != nil {
		return nil, err
	}
	return pdh.createDistribution(publicationHref, distName, distPath, contentGuardHref)
}

func (pdh *PulpDistributionHelper) createDistribution(publicationHref, distName, distPath string, contentGuardHref *string) (*zest.TaskResponse, error) {
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

func (pdh *PulpDistributionHelper) DistributionUpdateNeeded(repo api.RepositoryResponse,
	existingDist zest.RpmRpmDistributionResponse,
	publicationHref string) (bool, error) {
	contentGuardHref, err := pdh.FetchContentGuard(repo.OrgID, repo.FeatureName)
	if err != nil {
		return true, err
	}
	return distributionUpdateNeeded(existingDist, publicationHref, contentGuardHref), nil
}

func distributionUpdateNeeded(existingDist zest.RpmRpmDistributionResponse, publicationHref string, contentGuardHref *string) bool {
	if !utils.OptionalStringsEqual(existingDist.ContentGuard.Get(), contentGuardHref) {
		return true
	}
	if existingDist.Publication.Get() == nil || *existingDist.Publication.Get() != publicationHref {
		return true
	}
	return false
}

func (pdh *PulpDistributionHelper) UpdateDistribution(repo api.RepositoryResponse, distHref, publicationHref, distName, distPath string) (*zest.TaskResponse, error) {
	contentGuardHref, err := pdh.FetchContentGuard(repo.OrgID, repo.FeatureName)
	if err != nil {
		return nil, err
	}
	return pdh.updateDistribution(distHref, publicationHref, distName, distPath, contentGuardHref)
}

func (pdh *PulpDistributionHelper) updateDistribution(distHref, publicationHref, distName, distPath string, contentGuardHref *string) (*zest.TaskResponse, error) {
	distTaskHref, err := pdh.pulpClient.UpdateRpmDistribution(pdh.ctx, distHref, publicationHref, distName, distPath, contentGuardHref)
	if err != nil {
		return nil, err
	}

	distTask, err := pdh.pulpClient.PollTask(pdh.ctx, distTaskHref)
	if err != nil {
		return nil, err
	}

	return distTask, nil
}

func (pdh *PulpDistributionHelper) CreateOrUpdateDistribution(repo api.RepositoryResponse, publicationHref, distName, distPath string) (string, string, error) {
	contentGuardHref, err := pdh.FetchContentGuard(repo.OrgID, repo.FeatureName)
	if err != nil {
		return "", "", err
	}
	return pdh.createOrUpdateDistribution(publicationHref, distName, distPath, contentGuardHref)
}

// CreateOrUpdateUnguardedDistribution creates or reconciles a distribution with no content guard.
func (pdh *PulpDistributionHelper) CreateOrUpdateUnguardedDistribution(publicationHref, distName, distPath string) (string, string, error) {
	return pdh.createOrUpdateDistribution(publicationHref, distName, distPath, nil)
}

func (pdh *PulpDistributionHelper) CreateOrUpdateTemplateDistribution(repo api.RepositoryResponse, snapshot models.Snapshot, templateOrgID, distName, distPath string) (string, string, error) {
	if repo.Partner && repo.OrgID != templateOrgID {
		if !snapshot.Published {
			return "", "", fmt.Errorf("refusing to create unguarded distribution for foreign partner repository %s using unpublished snapshot %s", repo.UUID, snapshot.UUID)
		}
		return pdh.CreateOrUpdateUnguardedDistribution(snapshot.PublicationHref, distName, distPath)
	}
	return pdh.CreateOrUpdateDistribution(repo, snapshot.PublicationHref, distName, distPath)
}

func (pdh *PulpDistributionHelper) createOrUpdateDistribution(publicationHref, distName, distPath string, contentGuardHref *string) (string, string, error) {
	distTask := &zest.TaskResponse{}
	var distTaskHref string

	resp, err := pdh.pulpClient.FindDistributionByPath(pdh.ctx, distPath)
	if err != nil {
		return "", "", err
	}

	if resp == nil {
		distTask, err = pdh.createDistribution(publicationHref, distName, distPath, contentGuardHref)
		if distTask != nil && distTask.PulpHref != nil {
			distTaskHref = *distTask.PulpHref
		}
		if err != nil {
			return "", distTaskHref, err
		}
		distHrefPtr := pulp_client.SelectRpmDistributionHref(distTask)
		if distHrefPtr == nil {
			return "", distTaskHref, fmt.Errorf("could not find a distribution href in task: %v", distTask.PulpHref)
		}
		return *distHrefPtr, distTaskHref, err
	}

	if !distributionUpdateNeeded(*resp, publicationHref, contentGuardHref) {
		return "", "", nil
	}

	distTask, err = pdh.updateDistribution(*resp.PulpHref, publicationHref, distName, distPath, contentGuardHref)
	if distTask != nil && distTask.PulpHref != nil {
		distTaskHref = *distTask.PulpHref
	}
	if err != nil {
		return "", "", err
	}

	return *resp.PulpHref, distTaskHref, err
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
		names := utils.ParseFeatures(feature)
		if len(names) > 0 && !utils.AllStringsIn(names, config.SubscriptionFeaturesIgnored) {
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
