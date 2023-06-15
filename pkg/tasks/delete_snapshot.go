package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	zest "github.com/content-services/zest/release/v3"
)

const SnapshotDelete = "snapshot-delete"

type DeleteSnapshotPayload struct {
	RepoConfigUUID string
}

type DeleteSnapshot struct {
	daoReg     *dao.DaoRegistry
	pulpClient pulp_client.PulpClient
	payload    *DeleteSnapshotPayload
	task       *models.TaskInfo
	ctx        context.Context
}

func DeleteSnapshotHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
	opts := DeleteSnapshotPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for " + SnapshotDelete)
	}

	ds := DeleteSnapshot{
		daoReg:     dao.GetDaoRegistry(db.DB),
		pulpClient: pulp_client.GetPulpClient(),
		payload:    &opts,
		task:       task,
		ctx:        ctx,
	}
	return ds.Run()
}

func (d *DeleteSnapshot) Run() error {
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

func (d *DeleteSnapshot) fetchSnapshots() ([]dao.Snapshot, error) {
	return d.daoReg.Snapshot.FetchForRepoUUID(d.task.OrgId, d.task.RepositoryUUID.String())
}

func (d *DeleteSnapshot) deleteRpmDistribution(snap dao.Snapshot) (*zest.TaskResponse, error) {
	deleteDistributionHref, err := d.pulpClient.DeleteRpmDistribution(snap.DistributionHref)
	if err != nil {
		if err.Error() == "404 Not Found" {
			return nil, nil
		}
		return nil, err
	}
	task, err := d.pulpClient.PollTask(deleteDistributionHref)
	if err != nil {
		return task, err
	}
	return task, nil
}

func (d *DeleteSnapshot) deleteRpmRepoAndRemote() (taskRepo, taskRemote *zest.TaskResponse, err error) {
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

func (d *DeleteSnapshot) deleteSnapshot(snapUUID string) error {
	err := d.daoReg.Snapshot.Delete(snapUUID)
	if err != nil {
		return err
	}
	return nil
}
