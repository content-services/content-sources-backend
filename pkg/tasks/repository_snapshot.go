package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/content-services/content-sources-backend/pkg/utils"
	zest "github.com/content-services/zest/release/v2024"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func SnapshotHandler(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error {
	opts := payloads.SnapshotPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for Snapshot")
	}
	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	daoReg := dao.GetDaoRegistry(db.DB)
	domainName, err := daoReg.Domain.FetchOrCreateDomain(ctx, task.OrgId)
	if err != nil {
		return err
	}
	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	sr := SnapshotRepository{
		orgId:          task.OrgId,
		domainName:     domainName,
		repositoryUUID: task.ObjectUUID,
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

type SnapshotRepository struct {
	orgId          string
	domainName     string
	repositoryUUID uuid.UUID
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
	var remoteHref string
	var repoHref string
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

	helper := SnapshotHelper{
		pulpClient: sr.pulpClient,
		ctx:        sr.ctx,
		payload:    sr,
		logger:     sr.logger,
		orgId:      sr.orgId,
		repo:       repoConfig,
		daoReg:     sr.daoReg,
		domainName: sr.domainName,
	}

	defer func() {
		if errors.Is(err, context.Canceled) {
			cleanupErr := helper.Cleanup()
			if cleanupErr != nil {
				sr.logger.Err(cleanupErr).Msg("error cleaning up canceled snapshot helper")
			}
			cleanupErr = sr.cleanupOnCancel()
			if cleanupErr != nil {
				sr.logger.Err(cleanupErr).Msg("error cleaning up canceled snapshot")
			}
			cleanupErr = sr.UpdatePayload()
			if cleanupErr != nil {
				sr.logger.Err(cleanupErr).Msg("error cleaning up payload")
			}
			sr.logger.Warn().Msg("task was cancelled")
		}
	}()

	if repoConfig.Origin != config.OriginUpload {
		remoteHref, err = sr.findOrCreateRemote(repoConfig)
		if err != nil {
			return err
		}
	}

	repoHref, err = sr.findOrCreatePulpRepo(repoConfig.UUID, remoteHref)
	if err != nil {
		return err
	}

	var versionHref *string
	if repoConfig.Origin == config.OriginUpload {
		// Lookup the repositories version zero
		repo, err := sr.pulpClient.GetRpmRepositoryByName(sr.ctx, repoConfig.UUID)
		if err != nil {
			return fmt.Errorf("Could not lookup version for upload repo %w", err)
		}
		versionHref = repo.LatestVersionHref
	} else {
		versionHref, err = sr.syncRepository(repoHref, remoteHref)
		if err != nil {
			return err
		}
	}

	if versionHref == nil {
		// Nothing updated, but maybe the previous version was orphaned?
		versionHref, err = sr.GetOrphanedLatestVersion(repoConfig.UUID)
		if err != nil {
			return err
		}
	}
	if versionHref == nil {
		// There really isn't a new repo version available
		// TODO: figure out how to better indicate this to the user
		return nil
	}

	return helper.Run(*versionHref)
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
		caCert = utils.Ptr(string(ca))
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
		sr.payload.SyncTaskHref = nil
		if sr.payload.PublicationTaskHref != nil {
			_, err := pulpClient.CancelTask(ctxWithLogger, *sr.payload.PublicationTaskHref)
			if err != nil {
				return err
			}
			sr.payload.PublicationTaskHref = nil
		}
		versionHref := pulp_client.SelectVersionHref(&task)
		if versionHref != nil {
			_, err = pulpClient.DeleteRpmRepositoryVersion(ctxWithLogger, *versionHref)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func ContentSummaryToContentCounts(summary *zest.ContentSummaryResponse) (models.ContentCountsType, models.ContentCountsType, models.ContentCountsType) {
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

func (sr *SnapshotRepository) SavePublicationTaskHref(href string) error {
	if href == "" {
		sr.payload.PublicationTaskHref = nil
	} else {
		sr.payload.PublicationTaskHref = &href
	}
	return sr.UpdatePayload()
}

func (sr *SnapshotRepository) GetPublicationTaskHref() *string {
	return sr.payload.PublicationTaskHref
}

func (sr *SnapshotRepository) SaveDistributionTaskHref(href string) error {
	if href == "" {
		sr.payload.DistributionTaskHref = nil
	} else {
		sr.payload.DistributionTaskHref = &href
	}
	return sr.UpdatePayload()
}

func (sr *SnapshotRepository) GetDistributionTaskHref() *string {
	return sr.payload.DistributionTaskHref
}

func (sr *SnapshotRepository) SaveSnapshotIdent(id string) error {
	if id == "" {
		sr.payload.SnapshotIdent = nil
	} else {
		sr.payload.SnapshotIdent = &id
	}
	return sr.UpdatePayload()
}

func (sr *SnapshotRepository) GetSnapshotIdent() *string {
	return sr.payload.SnapshotIdent
}

func (sr *SnapshotRepository) SaveSnapshotUUID(uuid string) error {
	if uuid == "" {
		sr.payload.SnapshotUUID = nil
	} else {
		sr.payload.SnapshotUUID = &uuid
	}
	return sr.UpdatePayload()
}

func (sr *SnapshotRepository) GetSnapshotUUID() *string {
	return sr.payload.SnapshotUUID
}
