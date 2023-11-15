package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
)

func RepairLatestSnapshotHandler(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error {
	opts := payloads.RepairPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for Snapshot")
	}
	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	ctxWithLogger := logger.WithContext(ctx)

	daoReg := dao.GetDaoRegistry(db.DB)
	domainName, err := daoReg.Domain.FetchOrCreateDomain(task.OrgId)
	if err != nil {
		return err
	}
	pulpClient := pulp_client.GetPulpClientWithDomain(ctxWithLogger, domainName)

	rr := RepairRepository{
		orgId:      task.OrgId,
		pulpClient: pulpClient,
		payload:    &opts,
		queue:      queue,
		task:       task,
		dao:        daoReg,
	}
	return rr.Run()
}

type RepairRepository struct {
	dao        *dao.DaoRegistry
	orgId      string
	pulpClient pulp_client.PulpClient
	payload    *payloads.RepairPayload
	task       *models.TaskInfo
	queue      *queue.Queue
	ctx        context.Context
}

// RepairRepository repairs a specific repository version
func (rr *RepairRepository) Run() error {
	err := rr.dao.RepositoryConfig.InitializePulpClient(rr.ctx, rr.orgId)
	if err != nil {
		return fmt.Errorf("error initializing pulp client: %w", err)
	}
	repoConfig, err := rr.dao.RepositoryConfig.Fetch(rr.orgId, rr.payload.RepositoryConfigUUID)
	if err != nil {
		return fmt.Errorf("couldn't query repository: %w", err)
	}
	if repoConfig.LastSnapshot == nil {
		return fmt.Errorf("repository has no snapshots: %v", rr.payload.RepositoryConfigUUID)
	}
	snap, err := rr.dao.Snapshot.Internal_Fetch(repoConfig.LastSnapshot.UUID)
	if err != nil {
		return fmt.Errorf("error fetching snapshot: %w", err)
	}
	if snap.VersionHref == "" {
		return fmt.Errorf("Version href is nil for snapshot, cannot repair")
	}

	taskHref, err := rr.pulpClient.RepairRpmRepositoryVersion(snap.VersionHref)
	if err != nil {
		return fmt.Errorf("couldn't initiate repair process: %w", err)
	}
	rr.payload.RepairTaskHref = &taskHref
	err = rr.UpdatePayload()
	if err != nil {
		return fmt.Errorf("couldn't update payload: %w", err)
	}
	_, err = rr.pulpClient.PollTask(taskHref)
	if err != nil {
		return fmt.Errorf("failed polling task: %w", err)
	}
	return nil
}

func (rr *RepairRepository) UpdatePayload() error {
	var err error
	a := *rr.payload
	rr.task, err = (*rr.queue).UpdatePayload(rr.task, a)
	if err != nil {
		return err
	}
	return nil
}
