package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/helpers"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
)

type UpdateLatestSnapshotPayload struct {
	RepositoryConfigUUID string
}

// UpdateLatestSnapshotHandler for the given repo config UUID, fetches all templates (with use_latest=true) containing that repository.
// For each template, updates the pulp distribution to serve the latest snapshot for that repository.
func UpdateLatestSnapshotHandler(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error {
	opts := UpdateLatestSnapshotPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for UpdateLatestSnapshotPayload")
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

	t := UpdateLatestSnapshot{
		daoReg:       daoReg,
		ctx:          ctxWithLogger,
		orgID:        task.OrgId,
		payload:      &opts,
		pulpClient:   pulpClient,
		domainName:   domainName,
		rhDomainName: rhDomainName,
	}

	return t.Run()
}

type UpdateLatestSnapshot struct {
	daoReg       *dao.DaoRegistry
	ctx          context.Context
	orgID        string
	payload      *UpdateLatestSnapshotPayload
	pulpClient   pulp_client.PulpClient
	domainName   string
	rhDomainName string
}

func (t *UpdateLatestSnapshot) Run() error {
	var err error
	filterData := api.TemplateFilterData{UseLatest: true, RepositoryUUIDs: []string{t.payload.RepositoryConfigUUID}}
	templates, _, err := t.daoReg.Template.List(t.ctx, t.orgID, api.PaginationData{Limit: -1}, filterData)
	if err != nil {
		return err
	}

	repo, err := t.daoReg.RepositoryConfig.Fetch(t.ctx, t.orgID, t.payload.RepositoryConfigUUID)
	if err != nil {
		return err
	}

	snap, err := t.daoReg.Snapshot.FetchLatestSnapshotModel(t.ctx, repo.UUID)
	if err != nil {
		return err
	}

	for _, template := range templates.Data {
		if repo.OrgID == config.RedHatOrg {
			t.pulpClient = t.pulpClient.WithDomain(t.rhDomainName)
		} else {
			t.pulpClient = t.pulpClient.WithDomain(t.domainName)
		}

		err = t.updateLatestSnapshot(repo, template, snap)
		if err != nil {
			daoErr := t.daoReg.Template.UpdateLastError(t.ctx, template.OrgID, template.UUID, err.Error())
			if daoErr != nil {
				return daoErr
			}
			return err
		}
	}
	return nil
}

func (t *UpdateLatestSnapshot) updateLatestSnapshot(repo api.RepositoryResponse, template api.TemplateResponse, snap models.Snapshot) error {
	distPath, distName, err := getDistPathAndName(repo, template.UUID)
	if err != nil {
		return err
	}

	_, _, err = helpers.NewPulpDistributionHelper(t.ctx, t.pulpClient).CreateOrUpdateDistribution(t.orgID, distName, distPath, snap.PublicationHref)
	if err != nil {
		return err
	}

	distResp, err := t.pulpClient.FindDistributionByPath(t.ctx, distPath)
	if err != nil {
		return err
	}

	repoConfigDistributionHref := map[string]string{}
	repoConfigDistributionHref[repo.UUID] = *distResp.PulpHref
	err = t.daoReg.Template.UpdateDistributionHrefs(t.ctx, template.UUID, []string{repo.UUID}, []models.Snapshot{snap}, repoConfigDistributionHref)
	if err != nil {
		return err
	}
	err = t.daoReg.Template.UpdateSnapshots(t.ctx, template.UUID, []string{repo.UUID}, []models.Snapshot{snap})
	if err != nil {
		return err
	}

	return nil
}
