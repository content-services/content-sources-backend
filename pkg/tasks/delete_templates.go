package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/clients/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
)

type DeleteTemplatesPayload struct {
	TemplateUUID    string
	RepoConfigUUIDs []string
}

type DeleteTemplates struct {
	orgID               string
	rhDomainName        string
	domainName          string
	communityDomainName string
	ctx                 context.Context
	payload             *DeleteTemplatesPayload
	task                *models.TaskInfo
	daoReg              *dao.DaoRegistry
	pulpClient          pulp_client.PulpClient
	cpClient            candlepin_client.CandlepinClient
}

func DeleteTemplateHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
	opts := DeleteTemplatesPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for " + config.DeleteTemplatesTask)
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

	communityDomainName, err := daoReg.Domain.Fetch(ctxWithLogger, config.CommunityOrg)
	if err != nil {
		return err
	}

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)
	cpClient := candlepin_client.NewCandlepinClient()

	dt := DeleteTemplates{
		daoReg:              daoReg,
		payload:             &opts,
		task:                task,
		ctx:                 ctx,
		rhDomainName:        rhDomainName,
		domainName:          domainName,
		communityDomainName: communityDomainName,
		cpClient:            cpClient,
		pulpClient:          pulpClient,
		orgID:               task.OrgId,
	}
	return dt.Run()
}

func (d *DeleteTemplates) Run() error {
	var err error

	if config.PulpConfigured() {
		err = d.deleteDistributions()
		if err != nil {
			return err
		}
	}

	if config.CandlepinConfigured() {
		err = d.cpClient.DeleteEnvironment(d.ctx, d.payload.TemplateUUID)
		if err != nil {
			return fmt.Errorf("failed to delete candlepin environment: %w", err)
		}
	}

	err = d.deleteTemplate()
	if err != nil {
		return err
	}
	return nil
}

// deleteDistributions deletes all the pulp distributions for the repositories in the given template
func (d *DeleteTemplates) deleteDistributions() error {
	logger := LogForTask(d.task.Id.String(), d.task.Typename, d.task.RequestID)
	var errs []error

	for _, repoConfigUUID := range d.payload.RepoConfigUUIDs {
		repo, err := d.daoReg.RepositoryConfig.Fetch(d.ctx, d.orgID, repoConfigUUID)
		if err != nil {
			var daoErr *ce.DaoError
			if errors.As(err, &daoErr) && daoErr.NotFound {
				logger.Warn().
					Str("repo_config_uuid", repoConfigUUID).
					Str("template_uuid", d.payload.TemplateUUID).
					Msg("repo config not found, skipping distribution deletion")
				continue
			}
			errs = append(errs, fmt.Errorf("failed to fetch repo config %v: %w", repoConfigUUID, err))
			continue
		}
		if repo.LastSnapshot == nil {
			logger.Debug().
				Str("repo_config_uuid", repoConfigUUID).
				Msg("repo config has no snapshot, skipping distribution deletion")
			continue
		}

		// Configure client for org
		switch repo.OrgID {
		case config.RedHatOrg:
			d.pulpClient = d.pulpClient.WithDomain(d.rhDomainName)
		case config.CommunityOrg:
			d.pulpClient = d.pulpClient.WithDomain(d.communityDomainName)
		default:
			d.pulpClient = d.pulpClient.WithDomain(d.domainName)
		}

		distHref, err := d.daoReg.Template.GetDistributionHref(d.ctx, d.payload.TemplateUUID, repoConfigUUID)
		if err != nil {
			errs = append(errs, fmt.Errorf("error getting distribution href for repo %v: %w", repoConfigUUID, err))
			continue
		}

		if distHref == nil {
			logger.Warn().
				Str("template_uuid", d.payload.TemplateUUID).
				Msg("distribution href is null")
			continue
		}

		taskHref, err := d.pulpClient.DeleteRpmDistribution(d.ctx, *distHref)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to delete rpm distribution %v for repo %v: %w", *distHref, repoConfigUUID, err))
			continue
		}

		if taskHref != nil {
			if _, err = d.pulpClient.PollTask(d.ctx, *taskHref); err != nil {
				errs = append(errs, fmt.Errorf("error polling distribution deletion task for repo %v: %w", repoConfigUUID, err))
			}
		}
	}
	return errors.Join(errs...)
}

func (d *DeleteTemplates) deleteTemplate() error {
	logger := LogForTask(d.task.Id.String(), d.task.Typename, d.task.RequestID)
	err := d.daoReg.Template.Delete(d.ctx, d.task.OrgId, d.payload.TemplateUUID)
	if err != nil {
		var daoErr *ce.DaoError
		if errors.As(err, &daoErr) && daoErr.NotFound {
			logger.Warn().
				Str("template_uuid", d.payload.TemplateUUID).
				Msg("template not found during deletion, already deleted")
			return nil
		}
		return fmt.Errorf("failed to delete template: %w", err)
	}
	return nil
}
