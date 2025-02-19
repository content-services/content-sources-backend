package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
)

type DeleteTemplatesPayload struct {
	TemplateUUID    string
	RepoConfigUUIDs []string
}

type DeleteTemplates struct {
	orgID        string
	rhDomainName string
	domainName   string
	ctx          context.Context
	payload      *DeleteTemplatesPayload
	task         *models.TaskInfo
	daoReg       *dao.DaoRegistry
	pulpClient   pulp_client.PulpClient
	cpClient     candlepin_client.CandlepinClient
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

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)
	cpClient := candlepin_client.NewCandlepinClient()

	dt := DeleteTemplates{
		daoReg:       daoReg,
		payload:      &opts,
		task:         task,
		ctx:          ctx,
		rhDomainName: rhDomainName,
		domainName:   domainName,
		cpClient:     cpClient,
		pulpClient:   pulpClient,
		orgID:        task.OrgId,
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
			return err
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
	for _, repoConfigUUID := range d.payload.RepoConfigUUIDs {
		repo, err := d.daoReg.RepositoryConfig.Fetch(d.ctx, d.orgID, repoConfigUUID)
		if err != nil {
			var daoErr *ce.DaoError
			if errors.As(err, &daoErr) && daoErr.NotFound {
				continue
			}
			return err
		}
		if repo.LastSnapshot == nil {
			continue
		}

		// Configure client for org
		if repo.OrgID == config.RedHatOrg {
			d.pulpClient = d.pulpClient.WithDomain(d.rhDomainName)
		} else {
			d.pulpClient = d.pulpClient.WithDomain(d.domainName)
		}

		distHref, err := d.daoReg.Template.GetDistributionHref(d.ctx, d.payload.TemplateUUID, repoConfigUUID)
		if err != nil {
			return err
		}
		taskHref, err := d.pulpClient.DeleteRpmDistribution(d.ctx, distHref)
		if err != nil {
			return err
		}

		if taskHref != nil {
			_, err = d.pulpClient.PollTask(d.ctx, *taskHref)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *DeleteTemplates) deleteTemplate() error {
	err := d.daoReg.Template.Delete(d.ctx, d.task.OrgId, d.payload.TemplateUUID)
	if err != nil {
		return err
	}
	return nil
}
