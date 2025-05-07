package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/helpers"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
)

type DeleteSnapshots struct {
	orgID          string
	rhDomainName   string
	domainName     string
	ctx            context.Context
	payload        *payloads.DeleteSnapshotsPayload
	task           *models.TaskInfo
	daoReg         *dao.DaoRegistry
	pulpClient     *pulp_client.PulpClient
	pulpDistHelper *helpers.PulpDistributionHelper
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
	if !config.PulpConfigured() {
		return errors.New("no pulp client configured, can't proceed with snapshot deletion")
	} else if config.PulpConfigured() && ds.pulpClient != nil {
		err := ds.configurePulpClient()
		if err != nil {
			return err
		}
	}

	for _, snapUUID := range ds.payload.SnapshotsUUIDs {
		templateUpdateMap := make(map[string]models.Snapshot)
		snap, err := ds.daoReg.Snapshot.FetchModel(ds.ctx, snapUUID, true)
		if err != nil {
			return err
		}
		repo, err := ds.daoReg.RepositoryConfig.Fetch(ds.ctx, ds.orgID, snap.RepositoryConfigurationUUID)
		if err != nil {
			return err
		}

		err = ds.updateTemplatesUsingSnap(&templateUpdateMap, snap)
		if err != nil {
			return err
		}
		err = ds.daoReg.Template.DeleteTemplateSnapshot(ds.ctx, snap.UUID)
		if err != nil {
			return err
		}
		if repo.LastSnapshotUUID == snapUUID {
			err = ds.updateRepoConfig(repo.UUID, snapUUID)
			if err != nil {
				return err
			}
		}

		err = ds.deleteOrUpdatePulpContent(snap, repo, templateUpdateMap)
		if err != nil {
			return err
		}

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

	ds.pulpDistHelper = helpers.NewPulpDistributionHelper(ds.ctx, *ds.pulpClient)

	return nil
}

func (ds *DeleteSnapshots) deleteOrUpdatePulpContent(snap models.Snapshot, repo api.RepositoryResponse, templateUpdateMap map[string]models.Snapshot) error {
	for templateUUID, snaps := range templateUpdateMap {
		distPath, distName, err := getDistPathAndName(repo, templateUUID)
		if err != nil {
			return err
		}

		_, _, err = ds.pulpDistHelper.CreateOrUpdateDistribution(repo, snaps.PublicationHref, distName, distPath)
		if err != nil {
			return err
		}
	}

	latestPathIdent := fmt.Sprintf("%v/%v", repo.UUID, "latest")
	latestDistro, err := ds.getPulpClient().FindDistributionByPath(ds.ctx, latestPathIdent)
	if err != nil {
		return err
	}
	if latestDistro != nil {
		latestSnap, err := ds.daoReg.Snapshot.FetchLatestSnapshotModel(ds.ctx, repo.UUID)
		if err != nil {
			return err
		}
		_, _, err = ds.pulpDistHelper.CreateOrUpdateDistribution(repo, latestSnap.PublicationHref, repo.UUID, latestPathIdent)
		if err != nil {
			return err
		}
	}

	deleteDistributionHref, err := ds.getPulpClient().DeleteRpmDistribution(ds.ctx, snap.DistributionHref)
	if err != nil {
		return err
	}
	if deleteDistributionHref != nil {
		_, err = ds.getPulpClient().PollTask(ds.ctx, *deleteDistributionHref)
		if err != nil {
			return err
		}
	}

	deleteVersionHref, err := ds.getPulpClient().DeleteRpmRepositoryVersion(ds.ctx, snap.VersionHref)
	if err != nil {
		return err
	}
	if deleteVersionHref != nil {
		_, err = ds.getPulpClient().PollTask(ds.ctx, *deleteVersionHref)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ds *DeleteSnapshots) updateTemplatesUsingSnap(templateUpdateMap *map[string]models.Snapshot, snap models.Snapshot) error {
	repoUUIDs := []string{snap.RepositoryConfigurationUUID}
	templates, count, err := ds.daoReg.Template.List(ds.ctx, ds.orgID, false, api.PaginationData{Limit: -1}, api.TemplateFilterData{
		SnapshotUUIDs: []string{snap.UUID},
	})
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}

	for _, template := range templates.Data {
		date := template.Date
		if template.UseLatest {
			date = time.Now()
		}
		snaps, err := ds.daoReg.Snapshot.FetchSnapshotsModelByDateAndRepository(ds.ctx, ds.orgID, api.ListSnapshotByDateRequest{
			RepositoryUUIDS: repoUUIDs,
			Date:            date,
		})
		if err != nil {
			return err
		}
		if len(snaps) != 1 {
			return errors.New("no other snapshot was found")
		}

		err = ds.daoReg.Template.UpdateSnapshots(ds.ctx, template.UUID, repoUUIDs, snaps)
		if err != nil {
			return err
		}

		(*templateUpdateMap)[template.UUID] = snaps[0]
	}

	return nil
}

func (ds *DeleteSnapshots) updateRepoConfig(repoUUID, snapUUID string) error {
	latestSnap, err := ds.daoReg.Snapshot.FetchLatestSnapshotModel(ds.ctx, repoUUID)
	if err != nil {
		return err
	}
	if latestSnap.UUID == snapUUID {
		return errors.New("no other snapshot was found")
	}

	err = ds.daoReg.RepositoryConfig.UpdateLastSnapshot(ds.ctx, ds.orgID, repoUUID, latestSnap.UUID)
	if err != nil {
		return err
	}
	err = ds.daoReg.RepositoryConfig.UpdateLastSnapshotTask(ds.ctx, "", ds.orgID, repoUUID)
	if err != nil {
		return err
	}

	return nil
}
