package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	zest "github.com/content-services/zest/release/v2024"
	"github.com/google/uuid"
	"github.com/openlyinc/pointy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func SnapshotHandler(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error {
	logRequestId("first", log.Logger, ctx)

	opts := payloads.SnapshotPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for Snapshot")
	}
	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	logRequestId("2nd", log.Logger, ctx)
	logRequestId("3rd", log.Logger, ctx)
	daoReg := dao.GetDaoRegistry(db.DB)
	domainName, err := daoReg.Domain.FetchOrCreateDomain(ctx, task.OrgId)
	if err != nil {
		return err
	}
	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	sr := SnapshotRepository{
		orgId:          task.OrgId,
		domainName:     domainName,
		repositoryUUID: task.RepositoryUUID,
		daoReg:         daoReg,
		pulpClient:     pulpClient,
		task:           task,
		payload:        &opts,
		queue:          queue,
		ctx:            ctx,
		logger:         logger,
	}
	return sr.Run()
}

func logRequestId(msg string, lg zerolog.Logger, ctx context.Context) {
	rId, ok := ctx.Value(config.ContextRequestIDKey{}).(string)
	if ok {
		lg.Error().Msgf("MY REQUEST (%v) %v", msg, rId)
	} else {
		lg.Error().Msgf("NO REQUEST ID (%v)", msg)
	}
}

type SnapshotRepository struct {
	orgId          string
	domainName     string
	repositoryUUID uuid.UUID
	snapshotUUID   string
	daoReg         *dao.DaoRegistry
	pulpClient     pulp_client.PulpClient
	payload        *payloads.SnapshotPayload
	task           *models.TaskInfo
	queue          *queue.Queue
	ctx            context.Context
	logger         *zerolog.Logger
}

// SnapshotRepository creates a snapshot of a given repository config
func (sr *SnapshotRepository) Run() (err error) {
	defer func() {
		if errors.Is(err, context.Canceled) {
			cleanupErr := sr.cleanupOnCancel()
			if cleanupErr != nil {
				sr.logger.Err(cleanupErr).Msg("error cleaning up canceled snapshot")
			}
		}
	}()

	var remoteHref string
	var repoHref string
	var publicationHref string
	_, err = sr.pulpClient.LookupOrCreateDomain(sr.ctx, sr.domainName)
	if err != nil {
		return err
	}
	err = sr.pulpClient.UpdateDomainIfNeeded(sr.ctx, sr.domainName)
	if err != nil {
		return err
	}
	repoConfig, err := sr.lookupRepoObjects()
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

	versionHref, err := sr.syncRepository(repoHref, remoteHref)
	if err != nil {
		return err
	}
	if versionHref == nil {
		// Nothing updated, but maybe the previous version was orphaned?
		versionHref, err = sr.GetOrphanedLatestVersion(repoConfigUuid)
		if err != nil {
			return err
		}
	}
	if versionHref == nil {
		// There really isn't a new repo version available
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
	distHref, distPath, addedContentGuard, err := sr.createDistribution(publicationHref, repoConfig.UUID, *sr.payload.SnapshotIdent)
	if err != nil {
		return err
	}
	version, err := sr.pulpClient.GetRpmRepositoryVersion(sr.ctx, *versionHref)
	if err != nil {
		return err
	}

	if version.ContentSummary == nil {
		sr.logger.Error().Msgf("Found nil content Summary for version %v", *versionHref)
	}

	current, added, removed := ContentSummaryToContentCounts(version.ContentSummary)

	snap := models.Snapshot{
		VersionHref:                 *versionHref,
		PublicationHref:             publicationHref,
		DistributionPath:            distPath,
		RepositoryPath:              filepath.Join(sr.domainName, distPath),
		DistributionHref:            distHref,
		RepositoryConfigurationUUID: repoConfigUuid,
		ContentCounts:               current,
		AddedCounts:                 added,
		RemovedCounts:               removed,
		ContentGuardAdded:           addedContentGuard,
	}
	sr.logger.Debug().Msgf("Snapshot created at: %v", distPath)
	err = sr.daoReg.Snapshot.Create(sr.ctx, &snap)
	if err != nil {
		return err
	}
	sr.snapshotUUID = snap.UUID
	return nil
}

func (sr *SnapshotRepository) createDistribution(publicationHref string, repoConfigUUID string, snapshotId string) (distHref string, distPath string, addedContentGuard bool, err error) {
	distPath = fmt.Sprintf("%v/%v", repoConfigUUID, snapshotId)

	foundDist, err := sr.pulpClient.FindDistributionByPath(sr.ctx, distPath)
	if err != nil && foundDist != nil {
		return *foundDist.PulpHref, distPath, false, nil
	} else if err != nil {
		sr.logger.Error().Err(err).Msgf("Error looking up distribution by path %v", distPath)
	}

	if sr.payload.DistributionTaskHref == nil {
		var contentGuardHref *string
		if sr.orgId != config.RedHatOrg && config.Get().Clients.Pulp.CustomRepoContentGuards {
			href, err := sr.pulpClient.CreateOrUpdateGuardsForOrg(sr.ctx, sr.orgId)
			if err != nil {
				return "", "", false, fmt.Errorf("could not fetch/create/update content guard: %w", err)
			}
			contentGuardHref = &href
			addedContentGuard = true
		}
		distTaskHref, err := sr.pulpClient.CreateRpmDistribution(sr.ctx, publicationHref, snapshotId, distPath, contentGuardHref)
		if err != nil {
			return "", "", false, err
		}
		sr.payload.DistributionTaskHref = distTaskHref
		err = sr.UpdatePayload()
		if err != nil {
			return "", "", false, err
		}
	}

	distTask, err := sr.pulpClient.PollTask(sr.ctx, *sr.payload.DistributionTaskHref)
	if err != nil {
		return "", "", false, err
	}
	distHrefPtr := pulp_client.SelectRpmDistributionHref(distTask)
	if distHrefPtr == nil {
		return "", "", false, fmt.Errorf("Could not find a distribution href in task: %v", distTask.PulpHref)
	}
	return *distHrefPtr, distPath, addedContentGuard, nil
}

func (sr *SnapshotRepository) findOrCreatePublication(versionHref *string) (string, error) {
	var publicationHref *string
	// Publication
	publication, err := sr.pulpClient.FindRpmPublicationByVersion(sr.ctx, *versionHref)
	if err != nil {
		return "", err
	}
	if publication == nil || publication.PulpHref == nil {
		if sr.payload.PublicationTaskHref == nil {
			publicationTaskHref, err := sr.pulpClient.CreateRpmPublication(sr.ctx, *versionHref)
			if err != nil {
				return "", err
			}
			sr.payload.PublicationTaskHref = publicationTaskHref
			err = sr.UpdatePayload()
			if err != nil {
				return "", err
			}
		} else {
			sr.logger.Debug().Str("pulp_task_id", *sr.payload.PublicationTaskHref).Msg("Resuming Publication task")
		}

		publicationTask, err := sr.pulpClient.PollTask(sr.ctx, *sr.payload.PublicationTaskHref)
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

func (sr *SnapshotRepository) syncRepository(repoHref string, remoteHref string) (*string, error) {
	if sr.payload.SyncTaskHref == nil {
		syncTaskHref, err := sr.pulpClient.SyncRpmRepository(sr.ctx, repoHref, &remoteHref)
		if err != nil {
			return nil, err
		}
		sr.payload.SyncTaskHref = &syncTaskHref
		err = sr.UpdatePayload()
		if err != nil {
			return nil, err
		}
	} else {
		sr.logger.Debug().Str("pulp_task_id", *sr.payload.SyncTaskHref).Msg("Resuming Sync task")
	}

	syncTask, err := sr.pulpClient.PollTask(sr.ctx, *sr.payload.SyncTaskHref)
	if err != nil {
		return nil, err
	}

	versionHref := pulp_client.SelectVersionHref(syncTask)
	return versionHref, nil
}

func (sr *SnapshotRepository) findOrCreatePulpRepo(repoConfigUUID string, remoteHref string) (string, error) {
	repoResp, err := sr.pulpClient.GetRpmRepositoryByName(sr.ctx, repoConfigUUID)
	if err != nil {
		return "", err
	}
	if repoResp == nil {
		repoResp, err = sr.pulpClient.CreateRpmRepository(sr.ctx, repoConfigUUID, &remoteHref)
		if err != nil {
			return "", err
		}
	}
	return *repoResp.PulpHref, nil
}

func urlIsRedHat(url string) bool {
	return strings.Contains(url, "cdn.redhat.com")
}

func (sr *SnapshotRepository) findOrCreateRemote(repoConfig api.RepositoryResponse) (string, error) {
	var clientCertPair *string
	var caCert *string
	if repoConfig.OrgID == config.RedHatOrg && urlIsRedHat(repoConfig.URL) {
		clientCertPair = config.Get().Certs.CdnCertPairString
		ca, err := external_repos.LoadCA()
		if err != nil {
			log.Err(err).Msg("Cannot load red hat ca file")
		}
		caCert = pointy.Pointer(string(ca))
	}

	remoteResp, err := sr.pulpClient.GetRpmRemoteByName(sr.ctx, repoConfig.UUID)
	if err != nil {
		return "", err
	}
	if remoteResp == nil {
		remoteResp, err = sr.pulpClient.CreateRpmRemote(sr.ctx, repoConfig.UUID, repoConfig.URL, clientCertPair, clientCertPair, caCert)
		if err != nil {
			return "", err
		}
	} else if remoteResp.PulpHref != nil { // blindly update the remote
		_, err = sr.pulpClient.UpdateRpmRemote(sr.ctx, *remoteResp.PulpHref, repoConfig.URL, clientCertPair, clientCertPair, caCert)
		if err != nil {
			return "", err
		}
	}
	return *remoteResp.PulpHref, nil
}

func (sr *SnapshotRepository) lookupRepoObjects() (api.RepositoryResponse, error) {
	repoConfig, err := sr.daoReg.RepositoryConfig.FetchByRepoUuid(sr.ctx, sr.orgId, sr.repositoryUUID.String())
	if err != nil {
		return api.RepositoryResponse{}, err
	}
	return repoConfig, nil
}

func (sr *SnapshotRepository) cleanupOnCancel() error {
	logger := LogForTask(sr.task.Id.String(), sr.task.Typename, sr.task.RequestID)
	// TODO In Go 1.21 we could use context.WithoutCancel() to make copy of parent ctx that isn't canceled
	ctxWithLogger := logger.WithContext(context.Background())
	pulpClient := pulp_client.GetPulpClientWithDomain(sr.domainName)
	if sr.payload.SyncTaskHref != nil {
		task, err := pulpClient.CancelTask(ctxWithLogger, *sr.payload.SyncTaskHref)
		if err != nil {
			return err
		}
		task, err = pulpClient.GetTask(ctxWithLogger, *sr.payload.SyncTaskHref)
		if err != nil {
			return err
		}
		if sr.payload.PublicationTaskHref != nil {
			_, err := pulpClient.CancelTask(ctxWithLogger, *sr.payload.PublicationTaskHref)
			if err != nil {
				return err
			}
		}
		versionHref := pulp_client.SelectVersionHref(&task)
		if versionHref != nil {
			_, err = pulpClient.DeleteRpmRepositoryVersion(ctxWithLogger, *versionHref)
			if err != nil {
				return err
			}
		}
	}
	if sr.payload.DistributionTaskHref != nil {
		task, err := pulpClient.CancelTask(ctxWithLogger, *sr.payload.DistributionTaskHref)
		if err != nil {
			return err
		}
		task, err = pulpClient.GetTask(ctxWithLogger, *sr.payload.DistributionTaskHref)
		if err != nil {
			return err
		}
		versionHref := pulp_client.SelectRpmDistributionHref(&task)
		if versionHref != nil {
			_, err = pulpClient.DeleteRpmDistribution(ctxWithLogger, *versionHref)
			if err != nil {
				return err
			}
		}
	}
	if sr.snapshotUUID != "" {
		err := sr.daoReg.Snapshot.Delete(ctxWithLogger, sr.snapshotUUID)
		if err != nil {
			return err
		}
	}
	return nil
}

func ContentSummaryToContentCounts(summary *zest.RepositoryVersionResponseContentSummary) (models.ContentCountsType, models.ContentCountsType, models.ContentCountsType) {
	presentCount := models.ContentCountsType{}
	addedCount := models.ContentCountsType{}
	removedCount := models.ContentCountsType{}
	if summary != nil {
		for contentType, item := range summary.Present {
			num, ok := item["count"].(float64)
			if ok {
				presentCount[contentType] = int64(num)
			}
		}
		for contentType, item := range summary.Added {
			num, ok := item["count"].(float64)
			if ok {
				addedCount[contentType] = int64(num)
			}
		}
		for contentType, item := range summary.Removed {
			num, ok := item["count"].(float64)
			if ok {
				removedCount[contentType] = int64(num)
			}
		}
	}
	return presentCount, addedCount, removedCount
}

// GetOrphanedLatestVersion
//
//	 If a snapshot is made, but something bad happens between the repo version being made and its publication or distribution,
//			the repo version is 'lost', as there is no snapshot referring to it.  If this happens we can grab the latest
//		    repo version from pulp, and check if any snapshot exists with that version href.  If not then this is an orphaned version
func (sr *SnapshotRepository) GetOrphanedLatestVersion(repoConfigUUID string) (*string, error) {
	repoResp, err := sr.pulpClient.GetRpmRepositoryByName(sr.ctx, repoConfigUUID)
	if err != nil {
		return nil, nil
	}
	if repoResp == nil || repoResp.LatestVersionHref == nil {
		return nil, nil
	}
	snap, err := sr.daoReg.Snapshot.FetchSnapshotByVersionHref(sr.ctx, repoConfigUUID, *repoResp.LatestVersionHref)
	if err != nil {
		return nil, err
	}
	// The latest version from pulp is NOT tracked by a snapshot, so return it
	if snap == nil {
		return repoResp.LatestVersionHref, nil
	} else { // It is tracked by a snapshot, so the repo must not have changed
		return nil, nil
	}
}
