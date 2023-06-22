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
	zest "github.com/content-services/zest/release/v3"
)

type DeleteRepositorySnapshotsPayload struct {
	RepoConfigUUID string
}

type DeleteRepositorySnapshots struct {
	daoReg     *dao.DaoRegistry
	pulpClient pulp_client.PulpClient
	payload    *DeleteRepositorySnapshotsPayload
	task       *models.TaskInfo
	ctx        context.Context
}

func DeleteSnapshotHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
	opts := DeleteRepositorySnapshotsPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for " + config.DeleteRepositorySnapshotsTask)
	}

	ds := DeleteRepositorySnapshots{
		daoReg:     dao.GetDaoRegistry(db.DB),
		pulpClient: pulp_client.GetPulpClient(),
		payload:    &opts,
		task:       task,
		ctx:        ctx,
	}
	return ds.Run()
}

func (d *DeleteRepositorySnapshots) Run() error {
	snaps, _ := d.fetchSnapshots()
	if len(snaps) == 0 {
		return nil
	}
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
	_, _, err := d.deleteRpmRepoAndRemote()
	if err != nil {
		return err
	}
	return nil
}

func (d *DeleteRepositorySnapshots) fetchSnapshots() ([]dao.Snapshot, error) {
	return d.daoReg.Snapshot.FetchForRepoUUID(d.task.OrgId, d.task.RepositoryUUID.String())
}

func (d *DeleteRepositorySnapshots) deleteRpmDistribution(snap dao.Snapshot) (*zest.TaskResponse, error) {
	deleteDistributionHref, err := d.pulpClient.DeleteRpmDistribution(snap.DistributionHref)
	if err != nil {
		return nil, err
	}
	task, err := d.pulpClient.PollTask(deleteDistributionHref)
	if err != nil {
		return task, err
	}
	return task, nil
}

func (d *DeleteRepositorySnapshots) deleteRpmRepoAndRemote() (taskRepo, taskRemote *zest.TaskResponse, err error) {
	remoteResp, err := d.pulpClient.GetRpmRemoteByName(d.payload.RepoConfigUUID)
	if err != nil {
		return nil, nil, err
	}
	remoteHref := remoteResp.PulpHref

	repoResp, err := d.pulpClient.GetRpmRepositoryByRemote(*remoteHref)
	if err != nil {
		return nil, nil, err
	}
	repoHref := repoResp.PulpHref

	deleteRepoHref, err := d.pulpClient.DeleteRpmRepository(*repoHref)
	if err != nil {
		return nil, nil, err
	}
	taskRepo, err = d.pulpClient.PollTask(deleteRepoHref)
	if err != nil {
		return taskRepo, nil, err
	}

	deleteRemoteHref, err := d.pulpClient.DeleteRpmRemote(*remoteHref)
	if err != nil {
		return taskRepo, nil, err
	}
	taskRemote, err = d.pulpClient.PollTask(deleteRemoteHref)
	if err != nil {
		return taskRepo, taskRemote, err
	}
	return taskRepo, taskRemote, nil
}

func (d *DeleteRepositorySnapshots) deleteSnapshot(snapUUID string) error {
	err := d.daoReg.Snapshot.Delete(snapUUID)
	if err != nil {
		return err
	}
	return nil
}
