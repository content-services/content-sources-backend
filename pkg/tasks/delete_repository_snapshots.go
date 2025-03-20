package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	zest "github.com/content-services/zest/release/v2024"
)

type DeleteRepositorySnapshotsPayload struct {
	RepoConfigUUID string
}

type DeleteRepositorySnapshots struct {
	daoReg     *dao.DaoRegistry
	pulpClient *pulp_client.PulpClient
	cpClient   candlepin_client.CandlepinClient
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

func DeleteRepositorySnapshotsHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
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

	cpClient := candlepin_client.NewCandlepinClient()

	ds := DeleteRepositorySnapshots{
		daoReg:     daoReg,
		pulpClient: pulpClient,
		cpClient:   cpClient,
		payload:    &opts,
		task:       task,
		ctx:        ctx,
	}
	return ds.Run()
}

func (d *DeleteRepositorySnapshots) Run() error {
	var err error

	err = d.deleteCandlepinContent()
	if err != nil {
		return err
	}

	// If pulp client is deleted, the org never had a domain created
	if config.PulpConfigured() && d.pulpClient != nil {
		snaps, _ := d.fetchSnapshots()

		// Remove the "/latest" distribution
		latestPathIdent := fmt.Sprintf("%v/%v", d.payload.RepoConfigUUID, "latest")

		// Do not throw an error if not found
		latestDistro, err := d.getPulpClient().FindDistributionByPath(d.ctx, latestPathIdent)
		if err != nil {
			return err
		}

		if latestDistro != nil {
			_, err := d.deleteRpmDistribution(*latestDistro.PulpHref)
			if err != nil {
				return err
			}
		}

		err = d.deleteTemplateRepoDistributions()
		if err != nil {
			return err
		}

		for _, snap := range snaps {
			_, err = d.deleteRpmDistribution(snap.DistributionHref)
			if err != nil {
				return err
			}
			err = d.deleteTemplateSnapshot(snap.UUID)
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
	return d.daoReg.Snapshot.FetchForRepoConfigUUID(d.ctx, d.payload.RepoConfigUUID, false)
}

func (d *DeleteRepositorySnapshots) deleteRpmDistribution(snapDistributionHref string) (*zest.TaskResponse, error) {
	deleteDistributionHref, err := d.getPulpClient().DeleteRpmDistribution(d.ctx, snapDistributionHref)
	if err != nil {
		return nil, err
	}
	if deleteDistributionHref == nil {
		return nil, nil
	}
	task, err := d.getPulpClient().PollTask(d.ctx, *deleteDistributionHref)
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

func (d *DeleteRepositorySnapshots) candlepinRHContentId(templateOrgId string, repoConfigUuid string) (string, error) {
	rConfig, err := d.daoReg.RepositoryConfig.FetchWithoutOrgID(d.ctx, repoConfigUuid, true)
	if err != nil {
		return "", fmt.Errorf("error fetching repo config %v", err)
	}
	cpContent, err := d.cpClient.FetchContentsByLabel(d.ctx, templateOrgId, []string{rConfig.Label})
	if err != nil {
		return "", fmt.Errorf("failed to fetch content for repo %s/%s: %w", templateOrgId, rConfig.Label, err)
	}
	if len(cpContent) == 0 || cpContent[0].Id == nil {
		return "", fmt.Errorf("missing content for candlepin content for repo %s/%s", templateOrgId, rConfig.Label)
	}
	return *cpContent[0].Id, nil
}

func (d *DeleteRepositorySnapshots) deleteCandlepinContent() error {
	if !config.CandlepinConfigured() {
		return nil
	}
	if d.task.OrgId == config.RedHatOrg {
		templates, err := d.daoReg.Template.InternalOnlyGetTemplatesForRepoConfig(d.ctx, d.payload.RepoConfigUUID, false)
		if err != nil {
			return fmt.Errorf("couldn't get templates for repo config")
		}
		for _, template := range templates {
			// We have to lookup the content ID for RH content, as its based on the repo label
			contentId, err := d.candlepinRHContentId(template.OrgID, d.payload.RepoConfigUUID)
			if err != nil {
				return err
			}
			err = d.cpClient.DemoteContentFromEnvironment(d.ctx, template.UUID, []string{contentId})
			if err != nil {
				return fmt.Errorf("couldn't demote content from environment, %v", err)
			}
		}
	} else {
		err := d.cpClient.RemoveContentFromProduct(d.ctx, d.task.OrgId, d.payload.RepoConfigUUID)
		if err != nil {
			return err
		}

		err = d.cpClient.DeleteContent(d.ctx, d.task.OrgId, d.payload.RepoConfigUUID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *DeleteRepositorySnapshots) deleteTemplateRepoDistributions() (err error) {
	var templates []api.TemplateResponse
	if d.task.OrgId == config.RedHatOrg {
		templates, err = d.daoReg.Template.InternalOnlyGetTemplatesForRepoConfig(d.ctx, d.payload.RepoConfigUUID, false)
		if err != nil {
			return err
		}
	} else {
		templateResponse, _, err := d.daoReg.Template.List(d.ctx, d.task.OrgId, true, api.PaginationData{Limit: -1}, api.TemplateFilterData{RepositoryUUIDs: []string{d.payload.RepoConfigUUID}})
		if err != nil {
			return err
		}
		templates = templateResponse.Data
	}

	for _, template := range templates {
		distHref, err := d.daoReg.Template.GetDistributionHref(d.ctx, template.UUID, d.payload.RepoConfigUUID)
		if err != nil {
			return err
		}
		taskHref, err := d.getPulpClient().DeleteRpmDistribution(d.ctx, distHref)
		if err != nil {
			return err
		}

		if taskHref != nil {
			_, err = d.getPulpClient().PollTask(d.ctx, *taskHref)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *DeleteRepositorySnapshots) deleteTemplateSnapshot(snapshotUUID string) error {
	err := d.daoReg.Template.DeleteTemplateSnapshot(d.ctx, snapshotUUID)
	if err != nil {
		return err
	}
	return nil
}
