package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	zest "github.com/content-services/zest/release/v2024"
)

type DeleteRepositorySnapshotsPayload struct {
	RepoConfigUUID string
}

type DeleteRepositorySnapshots struct {
	daoReg     *dao.DaoRegistry
	pulpClient *pulp_client.PulpClient
	payload    *DeleteRepositorySnapshotsPayload
	task       *models.TaskInfo
	ctx        context.Context
}

// This org may or may not have a domain created in pulp, so make sure the domain exists and if not, return a nil pulpClient
func lookupOptionalPulpClient(ctx context.Context, globalClient pulp_client.PulpGlobalClient, task *models.TaskInfo, daoReg *dao.DaoRegistry) (*pulp_client.PulpClient, error) {
	if !config.PulpConfigured() {
		return nil, nil
	}
	domainName, err := daoReg.Domain.FetchOrCreateDomain(ctx, task.OrgId)
	if err != nil {
		return nil, err
	}

	// If we are deleting a repo from an org that's never done snapshotting,
	// it will not have a domain created
	domainFound, err := globalClient.LookupDomain(ctx, domainName)
	if err != nil {
		return nil, err
	}
	if domainFound != "" {
		client := pulp_client.GetPulpClientWithDomain(domainName)
		return &client, nil
	}
	return nil, nil
}

func DeleteSnapshotHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
	opts := DeleteRepositorySnapshotsPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for " + config.DeleteRepositorySnapshotsTask)
	}
	daoReg := dao.GetDaoRegistry(db.DB)
	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	ctxWithLogger := logger.WithContext(ctx)
	globalPulpClient := pulp_client.GetGlobalPulpClient()

	pulpClient, err := lookupOptionalPulpClient(ctxWithLogger, globalPulpClient, task, daoReg)
	if err != nil {
		return err
	}
	ds := DeleteRepositorySnapshots{
		daoReg:     daoReg,
		pulpClient: pulpClient,
		payload:    &opts,
		task:       task,
		ctx:        ctx,
	}
	return ds.Run()
}

func (d *DeleteRepositorySnapshots) Run() error {
	var err error

	// If pulp client is deleted, the org never had a domain created
	if config.PulpConfigured() && d.pulpClient != nil {
		snaps, _ := d.fetchSnapshots()
		for _, snap := range snaps {
			_, err := d.deleteRpmDistribution(snap)
			if err != nil {
				return err
			}
			err = d.deleteSnapshot(snap.UUID)
			if err != nil {
				return err
			}
		}
		_, _, err = d.deleteRpmRepoAndRemote()
		if err != nil {
			return err
		}
	}
	err = d.deleteRepoConfig()

	if err != nil {
		return err
	}
	return nil
}

func (d *DeleteRepositorySnapshots) getPulpClient() pulp_client.PulpClient {
	return *d.pulpClient
}

func (d *DeleteRepositorySnapshots) fetchSnapshots() ([]models.Snapshot, error) {
	return d.daoReg.Snapshot.FetchForRepoConfigUUID(d.ctx, d.payload.RepoConfigUUID)
}

func (d *DeleteRepositorySnapshots) deleteRpmDistribution(snap models.Snapshot) (*zest.TaskResponse, error) {
	deleteDistributionHref, err := d.getPulpClient().DeleteRpmDistribution(d.ctx, snap.DistributionHref)
	if err != nil {
		return nil, err
	}
	task, err := d.getPulpClient().PollTask(d.ctx, deleteDistributionHref)
	if err != nil {
		return task, err
	}
	return task, nil
}

func (d *DeleteRepositorySnapshots) deleteRpmRepoAndRemote() (taskRepo, taskRemote *zest.TaskResponse, err error) {
	remoteResp, err := d.getPulpClient().GetRpmRemoteByName(d.ctx, d.payload.RepoConfigUUID)
	if err != nil {
		return nil, nil, err
	}
	if remoteResp != nil {
		remoteHref := remoteResp.PulpHref
		deleteRemoteHref, err := d.getPulpClient().DeleteRpmRemote(d.ctx, *remoteHref)
		if err != nil {
			return taskRepo, nil, err
		}
		taskRemote, err = d.getPulpClient().PollTask(d.ctx, deleteRemoteHref)
		if err != nil {
			return taskRepo, taskRemote, err
		}
	}

	repoResp, err := d.getPulpClient().GetRpmRepositoryByName(d.ctx, d.payload.RepoConfigUUID)
	if err != nil {
		return nil, nil, err
	}
	if repoResp != nil {
		repoHref := repoResp.PulpHref
		deleteRepoHref, err := d.getPulpClient().DeleteRpmRepository(d.ctx, *repoHref)
		if err != nil {
			return nil, nil, err
		}
		taskRepo, err = d.getPulpClient().PollTask(d.ctx, deleteRepoHref)
		if err != nil {
			return taskRepo, nil, err
		}
	}
	return taskRepo, taskRemote, nil
}

func (d *DeleteRepositorySnapshots) deleteSnapshot(snapUUID string) error {
	err := d.daoReg.Snapshot.Delete(d.ctx, snapUUID)
	if err != nil {
		return err
	}
	return nil
}

func (d *DeleteRepositorySnapshots) deleteRepoConfig() error {
	err := d.daoReg.RepositoryConfig.Delete(d.ctx, d.task.OrgId, d.payload.RepoConfigUUID)
	if err != nil {
		return err
	}
	return nil
}
