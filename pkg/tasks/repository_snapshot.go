package tasks

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	zest "github.com/content-services/zest/release/v3"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// SnapshotOptions Options for the snapshot task.
//
//	These may change with integration with the tasking system
type SnapshotOptions struct {
	OrgId          string
	RepoConfigUuid string
	DaoRegistry    *dao.DaoRegistry
	PulpClient     pulp_client.PulpClient
	snapshotIdent  *string // Optional, autogenerated
}

func SnapshotRepository(opt SnapshotOptions) error {
	var remoteHref string
	var repoHref string
	var publicationHref string
	pulpClient := opt.PulpClient

	repoConfig, repo, err := lookupRepoObjects(opt.OrgId, opt.RepoConfigUuid, opt.DaoRegistry)
	if err != nil {
		return err
	}

	remoteHref, err = findOrCreateRemote(pulpClient, repoConfig)
	if err != nil {
		return err
	}

	repoHref, err = findOrCreatePulpRepo(pulpClient, repoConfig, remoteHref)
	if err != nil {
		return err
	}

	versionHref, err := syncRepository(pulpClient, repoHref)
	if err != nil {
		return err
	}
	if versionHref == nil {
		// Nothing updated, no snapshot needed
		// TODO: figure out how to better indicate this to the user
		return nil
	}

	publicationHref, err = findOrCreatePublication(pulpClient, versionHref)
	if err != nil {
		return err
	}

	if opt.snapshotIdent == nil {
		ident := uuid.NewString()
		opt.snapshotIdent = &ident
	}
	distHref, distPath, err := createDistribution(pulpClient, publicationHref, repoConfig.UUID, *opt.snapshotIdent)
	if err != nil {
		return err
	}

	version, err := pulpClient.GetRpmRepositoryVersion(*versionHref)
	if err != nil {
		return err
	}

	if version.ContentSummary == nil {
		log.Logger.Error().Msgf("Found nil content Summary for version %v", *versionHref)
	}
	snap := models.Snapshot{
		VersionHref:      *versionHref,
		PublicationHref:  publicationHref,
		DistributionPath: distPath,
		DistributionHref: distHref,
		OrgId:            opt.OrgId,
		RepositoryUUID:   repo.UUID,
		ContentCounts:    ContentSummaryToContentCounts(version.ContentSummary),
	}
	err = opt.DaoRegistry.Snapshot.Create(&snap)
	if err != nil {
		return err
	}
	return nil
}

func createDistribution(pulpClient pulp_client.PulpClient, publicationHref string, repoConfigUUID string, snapshotId string) (string, string, error) {
	distPath := fmt.Sprintf("%v/%v", repoConfigUUID, snapshotId)
	distTaskHref, err := pulpClient.CreateRpmDistribution(publicationHref, snapshotId, distPath)
	if err != nil {
		return "", "", err
	}
	distTask, err := pulpClient.PollTask(*distTaskHref)
	if err != nil {
		return "", "", err
	}
	distHref := pulp_client.SelectRpmDistributionHref(distTask)
	if distHref == nil {
		return "", "", fmt.Errorf("Could not find a distribution href in task: %v", distTask.PulpHref)
	}
	return *distHref, distPath, nil
}

func findOrCreatePublication(pulpClient pulp_client.PulpClient, versionHref *string) (string, error) {
	var publicationHref *string
	// Publication
	publication, err := pulpClient.FindRpmPublicationByVersion(*versionHref)
	if err != nil {
		return "", err
	}
	if publication == nil || publication.PulpHref == nil {
		// TODO: check for existing publication task href and poll that if found
		publicationTaskHref, err := pulpClient.CreateRpmPublication(*versionHref)
		if err != nil {
			return "", err
		}
		// TODO: Save publicationTaskHref onto task
		publicationTask, err := pulpClient.PollTask(*publicationTaskHref)
		if err != nil {
			return "", err
		}
		publicationHref = pulp_client.SelectPublicationHref(publicationTask)
		if publicationHref == nil {
			return "", fmt.Errorf("Could not find a publication href in task: %v", publicationTask.PulpHref)
		}
	} else {
		publicationHref = publication.PulpHref
	}
	return *publicationHref, nil
}

func syncRepository(pulpClient pulp_client.PulpClient, repoHref string) (*string, error) {
	// TODO: check for existing sync href and poll that if found
	syncTaskHref, err := pulpClient.SyncRpmRepository(repoHref, nil)
	if err != nil {
		return nil, err
	}
	// TODO: save sync href to task data
	syncTask, err := pulpClient.PollTask(syncTaskHref)
	if err != nil {
		return nil, err
	}

	versionHref := pulp_client.SelectVersionHref(syncTask)
	return versionHref, nil
}

func findOrCreatePulpRepo(pulpClient pulp_client.PulpClient, repoConfig api.RepositoryResponse, remoteHref string) (string, error) {
	repoResp, err := pulpClient.GetRpmRepositoryByName(repoConfig.UUID)
	if err != nil {
		return "", err
	}
	if repoResp == nil {
		repoResp, err = pulpClient.CreateRpmRepository(repoConfig.UUID, &remoteHref)
		if err != nil {
			return "", err
		}
	}
	return *repoResp.PulpHref, nil
}

func findOrCreateRemote(pulpClient pulp_client.PulpClient, repoConfig api.RepositoryResponse) (string, error) {
	remoteResp, err := pulpClient.GetRpmRemoteByName(repoConfig.UUID)
	if err != nil {
		return "", err
	}
	if remoteResp == nil {
		remoteResp, err = pulpClient.CreateRpmRemote(repoConfig.UUID, repoConfig.URL)
		if err != nil {
			return "", err
		}
	}
	return *remoteResp.PulpHref, nil
}

func lookupRepoObjects(orgId string, repoConfigUuid string, daoReg *dao.DaoRegistry) (api.RepositoryResponse, dao.Repository, error) {
	repoConfig, err := daoReg.RepositoryConfig.Fetch(orgId, repoConfigUuid)
	if err != nil {
		return api.RepositoryResponse{}, dao.Repository{}, err
	}

	repo, err := daoReg.Repository.FetchForUrl(repoConfig.URL)
	if err != nil {
		return api.RepositoryResponse{}, dao.Repository{}, err
	}
	return repoConfig, repo, nil
}

func ContentSummaryToContentCounts(summary *zest.RepositoryVersionResponseContentSummary) models.ContentCounts {
	counts := models.ContentCounts{}
	if summary != nil {
		for contentType, item := range summary.Present {
			num, ok := item["count"].(float64)
			if ok {
				counts[contentType] = int64(num)
			}
		}
	}
	return counts
}
