package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	zest "github.com/content-services/zest/release/v3"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

func SnapshotHandler(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error {
	opts := payloads.SnapshotPayload{}

	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for Snapshot")
	}
	pulpClient := pulp_client.GetPulpClient()
	sr := SnapshotRepository{
		orgId:          task.OrgId,
		repositoryUUID: task.RepositoryUUID,
		daoReg:         dao.GetDaoRegistry(db.DB),
		pulpClient:     pulpClient,
		task:           task,
		payload:        &opts,
		queue:          queue,
		ctx:            ctx,
	}
	return sr.Run()
}

type SnapshotRepository struct {
	orgId          string
	repositoryUUID uuid.UUID
	daoReg         *dao.DaoRegistry
	pulpClient     pulp_client.PulpClient
	payload        *payloads.SnapshotPayload
	task           *models.TaskInfo
	queue          *queue.Queue
	ctx            context.Context
}

// SnapshotRepository creates a snapshot of a given repository config
func (sr *SnapshotRepository) Run() error {
	var remoteHref string
	var repoHref string
	var publicationHref string

	repoConfig, repo, err := sr.lookupRepoObjects()
	if err != nil {
		return err
	}

	repoConfigUuid := repoConfig.UUID

	remoteHref, err = sr.findOrCreateRemote(repoConfig)
	if err != nil {
		return err
	}

	repoHref, err = sr.findOrCreatePulpRepo(repoConfigUuid, remoteHref)
	if err != nil {
		return err
	}

	versionHref, err := sr.syncRepository(repoHref)
	if err != nil {
		return err
	}
	if versionHref == nil {
		// Nothing updated, no snapshot needed
		// TODO: figure out how to better indicate this to the user
		return nil
	}

	publicationHref, err = sr.findOrCreatePublication(versionHref)
	if err != nil {
		return err
	}

	if sr.payload.SnapshotIdent == nil {
		ident := uuid.NewString()
		sr.payload.SnapshotIdent = &ident
	}
	distHref, distPath, err := sr.createDistribution(publicationHref, repoConfig.UUID, *sr.payload.SnapshotIdent)
	if err != nil {
		return err
	}
	version, err := sr.pulpClient.GetRpmRepositoryVersion(*versionHref)
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
		OrgId:            sr.orgId,
		RepositoryUUID:   repo.UUID,
		ContentCounts:    ContentSummaryToContentCounts(version.ContentSummary),
	}
	log.Logger.Debug().Msgf("Snapshot created at: %v", distPath)
	err = sr.daoReg.Snapshot.Create(&snap)
	if err != nil {
		return err
	}
	return nil
}

func (sr *SnapshotRepository) createDistribution(publicationHref string, repoConfigUUID string, snapshotId string) (string, string, error) {
	distPath := fmt.Sprintf("%v/%v", repoConfigUUID, snapshotId)

	foundDist, err := sr.pulpClient.FindDistributionByPath(distPath)
	if err != nil && foundDist != nil {
		return *foundDist.PulpHref, distPath, nil
	} else if err != nil {
		log.Error().Err(err).Msgf("Error looking up distribution by path %v", distPath)
	}

	if sr.payload.DistributionTaskHref == nil {
		distTaskHref, err := sr.pulpClient.CreateRpmDistribution(publicationHref, snapshotId, distPath)
		if err != nil {
			return "", "", err
		}
		sr.payload.DistributionTaskHref = distTaskHref
	}

	distTask, err := sr.pulpClient.PollTask(*sr.payload.DistributionTaskHref)
	if err != nil {
		return "", "", err
	}
	distHref := pulp_client.SelectRpmDistributionHref(distTask)
	if distHref == nil {
		return "", "", fmt.Errorf("Could not find a distribution href in task: %v", distTask.PulpHref)
	}
	return *distHref, distPath, nil
}

func (sr *SnapshotRepository) findOrCreatePublication(versionHref *string) (string, error) {
	var publicationHref *string
	// Publication
	publication, err := sr.pulpClient.FindRpmPublicationByVersion(*versionHref)
	if err != nil {
		return "", err
	}
	if publication == nil || publication.PulpHref == nil {
		if sr.payload.PublicationTaskHref == nil {
			publicationTaskHref, err := sr.pulpClient.CreateRpmPublication(*versionHref)
			if err != nil {
				return "", err
			}
			sr.payload.PublicationTaskHref = publicationTaskHref
			err = sr.UpdatePayload()
			if err != nil {
				return "", err
			}
		} else {
			log.Debug().Str("pulp_task_id", *sr.payload.PublicationTaskHref).Msg("Resuming Publication task")
		}

		publicationTask, err := sr.pulpClient.PollTask(*sr.payload.PublicationTaskHref)
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

func (sr *SnapshotRepository) UpdatePayload() error {
	var err error
	a := *sr.payload
	sr.task, err = (*sr.queue).UpdatePayload(sr.task, a)
	if err != nil {
		return err
	}
	return nil
}

func (sr *SnapshotRepository) syncRepository(repoHref string) (*string, error) {
	if sr.payload.SyncTaskHref == nil {
		syncTaskHref, err := sr.pulpClient.SyncRpmRepository(repoHref, nil)
		if err != nil {
			return nil, err
		}
		sr.payload.SyncTaskHref = &syncTaskHref
		err = sr.UpdatePayload()
		if err != nil {
			return nil, err
		}
	} else {
		log.Debug().Str("pulp_task_id", *sr.payload.SyncTaskHref).Msg("Resuming Sync task")
	}

	syncTask, err := sr.pulpClient.PollTask(*sr.payload.SyncTaskHref)
	if err != nil {
		return nil, err
	}

	versionHref := pulp_client.SelectVersionHref(syncTask)
	return versionHref, nil
}

func (sr *SnapshotRepository) findOrCreatePulpRepo(repoConfigUUID string, remoteHref string) (string, error) {
	repoResp, err := sr.pulpClient.GetRpmRepositoryByName(repoConfigUUID)
	if err != nil {
		return "", err
	}
	if repoResp == nil {
		repoResp, err = sr.pulpClient.CreateRpmRepository(repoConfigUUID, &remoteHref)
		if err != nil {
			return "", err
		}
	}
	return *repoResp.PulpHref, nil
}

func (sr *SnapshotRepository) findOrCreateRemote(repoConfig api.RepositoryResponse) (string, error) {
	remoteResp, err := sr.pulpClient.GetRpmRemoteByName(repoConfig.UUID)
	if err != nil {
		return "", err
	}
	if remoteResp == nil {
		remoteResp, err = sr.pulpClient.CreateRpmRemote(repoConfig.UUID, repoConfig.URL)
		if err != nil {
			return "", err
		}
	} else if remoteResp.Url != repoConfig.URL && remoteResp.PulpHref != nil {
		_, err = sr.pulpClient.UpdateRpmRemoteUrl(*remoteResp.PulpHref, repoConfig.URL)
		if err != nil {
			return "", err
		}
	}
	return *remoteResp.PulpHref, nil
}

func (sr *SnapshotRepository) lookupRepoObjects() (api.RepositoryResponse, dao.Repository, error) {
	repoConfig, err := sr.daoReg.RepositoryConfig.FetchByRepoUuid(sr.orgId, sr.repositoryUUID.String())
	if err != nil {
		return api.RepositoryResponse{}, dao.Repository{}, err
	}

	repo, err := sr.daoReg.Repository.FetchForUrl(repoConfig.URL)
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
