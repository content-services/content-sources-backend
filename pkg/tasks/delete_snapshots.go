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
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
)

type DeleteSnapshots struct {
	orgID        string
	rhDomainName string
	domainName   string
	ctx          context.Context
	payload      *payloads.DeleteSnapshotsPayload
	task         *models.TaskInfo
	daoReg       *dao.DaoRegistry
	pulpClient   *pulp_client.PulpClient
}

func DeleteSnapshotsHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
	opts := payloads.DeleteSnapshotsPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for %s", config.DeleteSnapshotsTask)
	}

	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	ctxWithLogger := logger.WithContext(ctx)

	daoReg := dao.GetDaoRegistry(db.DB)
	domainName, err := daoReg.Domain.Fetch(ctxWithLogger, task.OrgId)
	if err != nil {
		return err
	}

	rhDomainName, err := daoReg.Domain.Fetch(ctxWithLogger, config.RedHatOrg)
	if err != nil {
		return err
	}

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	ds := DeleteSnapshots{
		orgID:        task.OrgId,
		rhDomainName: rhDomainName,
		domainName:   domainName,
		ctx:          ctx,
		payload:      &opts,
		task:         task,
		daoReg:       daoReg,
		pulpClient:   &pulpClient,
	}
	return ds.Run()
}

func (ds *DeleteSnapshots) Run() error {
	if config.PulpConfigured() && ds.pulpClient != nil {
		err := ds.configurePulpClient()
		if err != nil {
			return err
		}
	}

	for _, snapUUID := range ds.payload.SnapshotsUUIDs {
		snap, err := ds.daoReg.Snapshot.FetchUnscoped(ds.ctx, snapUUID)
		if err != nil {
			return err
		}

		if config.PulpConfigured() && ds.pulpClient != nil {
			err = ds.deletePulpContent(snap)
			if err != nil {
				return err
			}
		}

		// #TODO: Update Template Snapshots

		err = ds.daoReg.Snapshot.Delete(ds.ctx, snapUUID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ds *DeleteSnapshots) getPulpClient() pulp_client.PulpClient {
	return *ds.pulpClient
}

func (ds *DeleteSnapshots) configurePulpClient() error {
	repo, err := ds.daoReg.RepositoryConfig.Fetch(ds.ctx, ds.orgID, ds.payload.RepoUUID)
	if err != nil {
		return err
	}
	if repo.OrgID == config.RedHatOrg && ds.rhDomainName != "" {
		ds.pulpClient = utils.Ptr(ds.getPulpClient().WithDomain(ds.rhDomainName))
	} else if ds.domainName != "" {
		ds.pulpClient = utils.Ptr(ds.getPulpClient().WithDomain(ds.domainName))
	}

	return nil
}

func (ds *DeleteSnapshots) deletePulpContent(snap models.Snapshot) error {
	deleteDistributionHref, err := ds.getPulpClient().DeleteRpmDistribution(ds.ctx, snap.DistributionHref)
	if err != nil {
		return err
	}
	_, err = ds.getPulpClient().PollTask(ds.ctx, deleteDistributionHref)
	if err != nil {
		return err
	}

	err = ds.getPulpClient().DeleteRpmPublication(ds.ctx, snap.PublicationHref)
	if err != nil {
		return err
	}

	_, err = ds.getPulpClient().DeleteRpmRepositoryVersion(ds.ctx, snap.VersionHref)
	if err != nil {
		return err
	}

	return nil
}
