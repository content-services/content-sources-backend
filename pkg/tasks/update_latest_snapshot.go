package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/helpers"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
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

	communityDomainName, err := daoReg.Domain.Fetch(ctxWithLogger, config.CommunityOrg)
	if err != nil {
		return err
	}

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	t := UpdateLatestSnapshot{
		daoReg:              daoReg,
		ctx:                 ctxWithLogger,
		orgID:               task.OrgId,
		payload:             &opts,
		pulpClient:          pulpClient,
		domainName:          domainName,
		rhDomainName:        rhDomainName,
		communityDomainName: communityDomainName,
		queue:               queue,
		task:                task,
	}

	return t.Run()
}

type UpdateLatestSnapshot struct {
	daoReg              *dao.DaoRegistry
	ctx                 context.Context
	orgID               string
	payload             *UpdateLatestSnapshotPayload
	pulpClient          pulp_client.PulpClient
	domainName          string
	rhDomainName        string
	communityDomainName string
	queue               *queue.Queue
	task                *models.TaskInfo
}

func (t *UpdateLatestSnapshot) Run() error {
	repo, err := t.daoReg.RepositoryConfig.FetchWithoutOrgID(t.ctx, t.payload.RepositoryConfigUUID, false)
	if err != nil {
		return err
	}

	var templates []api.TemplateResponse
	if t.orgID == config.RedHatOrg || t.orgID == config.CommunityOrg || repo.Partner {
		templates, err = t.daoReg.Template.InternalOnlyGetTemplatesForRepoConfig(t.ctx, t.payload.RepositoryConfigUUID, true)
		if err != nil {
			return err
		}
	} else {
		filterData := api.TemplateFilterData{UseLatest: true, RepositoryUUIDs: []string{t.payload.RepositoryConfigUUID}}
		resp, _, err := t.daoReg.Template.List(t.ctx, t.orgID, false, api.PaginationData{Limit: -1}, filterData)
		if err != nil {
			return err
		}
		templates = resp.Data
	}

	if repo.Partner {
		hasPublishedSnapshot, err := t.daoReg.Snapshot.HasPublishedSnapshot(t.ctx, repo.UUID)
		if err != nil {
			return fmt.Errorf("error checking published snapshots for repository %s: %w", repo.UUID, err)
		}
		if !hasPublishedSnapshot {
			return t.enqueueTemplateRemovalTasks(repo.UUID, templates)
		}
	}

	snap, err := t.daoReg.Snapshot.FetchLatestSnapshotForDistribution(t.ctx, repo.UUID)
	if err != nil {
		return err
	}

	for _, template := range templates {
		if err := t.configurePulpClientForRepo(repo); err != nil {
			return err
		}

		err = t.updateLatestSnapshot(repo, template, snap)
		if err != nil {
			daoErr := t.daoReg.Template.UpdateLastError(t.ctx, template.OrgID, template.UUID, err.Error())
			if daoErr != nil {
				return daoErr
			}
			return err
		}

		event.SendTemplateEvent(template.OrgID, event.TemplateUpdated, []event.TemplateEvent{event.MapTemplateResponse(template)})
	}
	return nil
}

func (t *UpdateLatestSnapshot) enqueueTemplateRemovalTasks(repoUUID string, templates []api.TemplateResponse) error {
	if t.queue == nil || t.task == nil {
		return fmt.Errorf("cannot enqueue template cleanup tasks without queue and parent task metadata")
	}

	for _, template := range templates {
		child := queue.Task{
			Typename: config.UpdateTemplateContentTask,
			Payload: payloads.UpdateTemplateContentPayload{
				TemplateUUID:              template.UUID,
				TriggeredByRepositoryUUID: utils.Ptr(repoUUID),
			},
			Dependencies: []uuid.UUID{t.task.Id},
			OrgId:        template.OrgID,
			AccountId:    t.task.AccountId,
			ObjectUUID:   utils.Ptr(template.UUID),
			ObjectType:   utils.Ptr(string(config.ObjectTypeTemplate)),
			RequestID:    t.task.RequestID,
		}
		if _, err := (*t.queue).Enqueue(&child); err != nil {
			return fmt.Errorf("error enqueueing template cleanup for template %s: %w", template.UUID, err)
		}
	}

	return nil
}

// configurePulpClientForRepo points the client at the domain that owns the repo's Pulp content.
// For custom/partner uploads that is always the repo owner's org, which may differ from task.OrgId.
func (t *UpdateLatestSnapshot) configurePulpClientForRepo(repo api.RepositoryResponse) error {
	domain, err := repositoryDomain(t.ctx, t.daoReg, repo, t.orgID, t.domainName, t.rhDomainName, t.communityDomainName)
	if err != nil {
		return err
	}
	t.pulpClient = t.pulpClient.WithDomain(domain)
	return nil
}

func (t *UpdateLatestSnapshot) updateLatestSnapshot(repo api.RepositoryResponse, template api.TemplateResponse, snap models.Snapshot) error {
	distPath, distName, err := getDistPathAndName(repo, template.UUID)
	if err != nil {
		return err
	}

	pdh := helpers.NewPulpDistributionHelper(t.ctx, t.pulpClient)
	_, _, err = pdh.CreateOrUpdateTemplateDistribution(repo, snap, template.OrgID, distName, distPath)
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
