package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	zest "github.com/content-services/zest/release/v2026"
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
	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	if !config.PulpConfigured() {
		logger.Debug().Msg("pulp not configured, skipping pulp client setup")
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
	logger.Debug().
		Str("org_id", task.OrgId).
		Str("domain_name", domainName).
		Msg("no pulp domain found for org, org has never snapshotted, skipping pulp cleanup")
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
			return fmt.Errorf("failed to find latest distribution by path %v: %w", latestPathIdent, err)
		}

		if latestDistro != nil {
			_, err := d.deleteRpmDistribution(*latestDistro.PulpHref)
			if err != nil {
				return fmt.Errorf("failed to delete latest distribution %v: %w", *latestDistro.PulpHref, err)
			}
		}

		err = d.deleteTemplateRepoDistributions()
		if err != nil {
			return err
		}

		var snapErrs []error
		for _, snap := range snaps {
			_, err = d.deleteRpmDistribution(snap.DistributionHref)
			if err != nil {
				snapErrs = append(snapErrs, err)
				continue
			}
			if err = d.deleteTemplateSnapshot(snap.UUID); err != nil {
				snapErrs = append(snapErrs, err)
				continue
			}
			if err = d.deleteSnapshot(snap.UUID); err != nil {
				snapErrs = append(snapErrs, err)
			}
		}
		if err = errors.Join(snapErrs...); err != nil {
			return err
		}
		// only runs if all snaps deletes succeed
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
	logger := LogForTask(d.task.Id.String(), d.task.Typename, d.task.RequestID)
	snaps, err := d.daoReg.Snapshot.FetchForRepoConfigUUID(d.ctx, d.payload.RepoConfigUUID, true)
	if err != nil {
		logger.Error().Err(err).Msg("Error fetching snapshots")
		return nil, err
	}
	logger.Debug().
		Int("count", len(snaps)).
		Msg("Successfully fetched snapshots")
	return snaps, err
}

func (d *DeleteRepositorySnapshots) deleteRpmDistribution(snapDistributionHref string) (*zest.TaskResponse, error) {
	logger := LogForTask(d.task.Id.String(), d.task.Typename, d.task.RequestID)
	deleteDistributionHref, err := d.getPulpClient().DeleteRpmDistribution(d.ctx, snapDistributionHref)
	if err != nil {
		return nil, fmt.Errorf("failed to delete rpm distribution %v: %w", snapDistributionHref, err)
	}
	if deleteDistributionHref == nil {
		logger.Debug().
			Str("distribution_href", snapDistributionHref).
			Msg("no task href returned for distribution deletion, distribution may have already been deleted")
		return nil, nil
	}
	task, err := d.getPulpClient().PollTask(d.ctx, *deleteDistributionHref)
	if err != nil {
		return task, fmt.Errorf("error polling distribution deletion task for %v: %w", snapDistributionHref, err)
	}
	return task, nil
}

func (d *DeleteRepositorySnapshots) deleteRpmRepoAndRemote() (taskRepo, taskRemote *zest.TaskResponse, err error) {
	logger := LogForTask(d.task.Id.String(), d.task.Typename, d.task.RequestID)

	remoteResp, err := d.getPulpClient().GetRpmRemoteByName(d.ctx, d.payload.RepoConfigUUID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to look up rpm remote: %w", err)
	}
	if remoteResp == nil {
		logger.Debug().
			Str("repo_config_uuid", d.payload.RepoConfigUUID).
			Msg("no rpm remote found, skipping remote deletion")
	}
	if remoteResp != nil {
		remoteHref := remoteResp.PulpHref
		deleteRemoteHref, err := d.getPulpClient().DeleteRpmRemote(d.ctx, *remoteHref)
		if err != nil {
			return taskRepo, nil, fmt.Errorf("failed to delete rpm remote %v: %w", *remoteHref, err)
		}
		taskRemote, err = d.getPulpClient().PollTask(d.ctx, deleteRemoteHref)
		if err != nil {
			return taskRepo, taskRemote, fmt.Errorf("error polling rpm remote deletion task for %v: %w", *remoteHref, err)
		}
	}

	repoResp, err := d.getPulpClient().GetRpmRepositoryByName(d.ctx, d.payload.RepoConfigUUID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to look up rpm repository: %w", err)
	}
	if repoResp == nil {
		logger.Debug().
			Str("repo_config_uuid", d.payload.RepoConfigUUID).
			Msg("no rpm repository found, skipping repository deletion")
	}
	if repoResp != nil {
		repoHref := repoResp.PulpHref
		deleteRepoHref, err := d.getPulpClient().DeleteRpmRepository(d.ctx, *repoHref)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to delete rpm repository %v: %w", *repoHref, err)
		}
		taskRepo, err = d.getPulpClient().PollTask(d.ctx, deleteRepoHref)
		if err != nil {
			return taskRepo, nil, fmt.Errorf("error polling rpm repository deletion task for %v: %w", *repoHref, err)
		}
	}
	return taskRepo, taskRemote, nil
}

func (d *DeleteRepositorySnapshots) deleteSnapshot(snapUUID string) error {
	logger := LogForTask(d.task.Id.String(), d.task.Typename, d.task.RequestID)
	err := d.daoReg.Snapshot.Delete(d.ctx, snapUUID)
	if err != nil {
		var daoErr *ce.DaoError
		if errors.As(err, &daoErr) && daoErr.NotFound {
			logger.Warn().
				Str("snapshot_uuid", snapUUID).
				Msg("snapshot not found during deletion, already deleted")
			return nil
		}
		return fmt.Errorf("error deleting snapshot %v: %w", snapUUID, err)
	}
	return nil
}

func (d *DeleteRepositorySnapshots) deleteRepoConfig() error {
	logger := LogForTask(d.task.Id.String(), d.task.Typename, d.task.RequestID)
	err := d.daoReg.RepositoryConfig.Delete(d.ctx, d.task.OrgId, d.payload.RepoConfigUUID)
	if err != nil {
		var daoErr *ce.DaoError
		if errors.As(err, &daoErr) && daoErr.NotFound {
			logger.Warn().
				Str("repo_config_uuid", d.payload.RepoConfigUUID).
				Msg("repo config not found during deletion, already deleted")
			return nil
		}
		return fmt.Errorf("error deleting repository configuration: %w", err)
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
	logger := LogForTask(d.task.Id.String(), d.task.Typename, d.task.RequestID)
	if !config.CandlepinConfigured() {
		logger.Debug().Msg("candlepin not configured, skipping candlepin content deletion")
		return nil
	}
	if d.task.OrgId == config.RedHatOrg {
		templates, err := d.daoReg.Template.InternalOnlyGetTemplatesForRepoConfig(d.ctx, d.payload.RepoConfigUUID, false)
		if err != nil {
			return fmt.Errorf("couldn't get templates for repo config: %w", err)
		}
		for _, template := range templates {
			// We have to lookup the content ID for RH content, as its based on the repo label
			contentId, err := d.candlepinRHContentId(template.OrgID, d.payload.RepoConfigUUID)
			if err != nil {
				return err
			}
			err = d.cpClient.DemoteContentFromEnvironment(d.ctx, template.UUID, []string{contentId})
			if err != nil {
				return fmt.Errorf("couldn't demote content from environment for template %v: %w", template.UUID, err)
			}
		}
	} else {
		err := d.cpClient.RemoveContentFromProduct(d.ctx, d.task.OrgId, d.payload.RepoConfigUUID)
		if err != nil {
			return fmt.Errorf("failed to remove content from candlepin product: %w", err)
		}

		err = d.cpClient.DeleteContent(d.ctx, d.task.OrgId, d.payload.RepoConfigUUID)
		if err != nil {
			return fmt.Errorf("error deleting candlepin content: %w", err)
		}
	}

	return nil
}

func (d *DeleteRepositorySnapshots) deleteTemplateRepoDistributions() error {
	logger := LogForTask(d.task.Id.String(), d.task.Typename, d.task.RequestID)
	var errs []error

	var templates []api.TemplateResponse
	if d.task.OrgId == config.RedHatOrg {
		var err error
		templates, err = d.daoReg.Template.InternalOnlyGetTemplatesForRepoConfig(d.ctx, d.payload.RepoConfigUUID, false)
		if err != nil {
			return fmt.Errorf("failed to get templates for repo config: %w", err)
		}
	} else {
		templateResponse, _, err := d.daoReg.Template.List(d.ctx, d.task.OrgId, true, api.PaginationData{Limit: -1}, api.TemplateFilterData{RepositoryUUIDs: []string{d.payload.RepoConfigUUID}})
		if err != nil {
			return fmt.Errorf("failed to get templates for repo config: %w", err)
		}
		templates = templateResponse.Data
	}

	for _, template := range templates {
		distHref, err := d.daoReg.Template.GetDistributionHref(d.ctx, template.UUID, d.payload.RepoConfigUUID)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get distribution href for template %v: %w", template.UUID, err))
			continue
		}

		if distHref == nil {
			logger.Warn().
				Str("template_uuid", template.UUID).
				Msg("distribution href is null")
			continue
		}

		taskHref, err := d.getPulpClient().DeleteRpmDistribution(d.ctx, *distHref)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to delete rpm distribution %v for template %v: %w", *distHref, template.UUID, err))
			continue
		}

		if taskHref != nil {
			if _, err = d.getPulpClient().PollTask(d.ctx, *taskHref); err != nil {
				errs = append(errs, fmt.Errorf("error polling distribution deletion task for template %v: %w", template.UUID, err))
			}
		}
	}

	return errors.Join(errs...)
}

func (d *DeleteRepositorySnapshots) deleteTemplateSnapshot(snapshotUUID string) error {
	logger := LogForTask(d.task.Id.String(), d.task.Typename, d.task.RequestID)
	err := d.daoReg.Template.DeleteTemplateSnapshot(d.ctx, snapshotUUID)
	if err != nil {
		var daoErr *ce.DaoError
		if errors.As(err, &daoErr) && daoErr.NotFound {
			logger.Warn().
				Str("snapshot_uuid", snapshotUUID).
				Msg("template snapshot association not found during deletion, already deleted")
			return nil
		}
		return fmt.Errorf("error deleting template snapshot %v: %w", snapshotUUID, err)
	}
	return nil
}
