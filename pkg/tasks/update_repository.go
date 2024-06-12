package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
)

type UpdateRepositoryPayload struct {
	RepositoryConfigUUID string
}

type UpdateRepository struct {
	orgID      string
	domainName string
	ctx        context.Context
	payload    *UpdateRepositoryPayload
	task       *models.TaskInfo
	daoReg     *dao.DaoRegistry
	repoConfig api.RepositoryResponse
	cpClient   candlepin_client.CandlepinClient
	pulpClient pulp_client.PulpClient
}

func UpdateRepositoryHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
	if config.Get().Clients.Candlepin.Server == "" {
		return nil
	}

	opts := UpdateRepositoryPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for " + config.UpdateRepositoryTask)
	}

	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	ctxWithLogger := logger.WithContext(ctx)

	daoReg := dao.GetDaoRegistry(db.DB)
	domainName, err := daoReg.Domain.Fetch(ctxWithLogger, task.OrgId)
	if err != nil {
		return err
	}

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)
	cpClient := candlepin_client.NewCandlepinClient()
	repo, err := daoReg.RepositoryConfig.Fetch(ctx, task.OrgId, opts.RepositoryConfigUUID)
	if err != nil {
		return fmt.Errorf("could not fetch repository config %w", err)
	}

	ur := UpdateRepository{
		daoReg:     daoReg,
		payload:    &opts,
		task:       task,
		ctx:        ctx,
		domainName: domainName,
		cpClient:   cpClient,
		orgID:      task.OrgId,
		repoConfig: repo,
		pulpClient: pulpClient,
	}
	return ur.Run()
}

func (ur *UpdateRepository) Run() error {
	content, err := ur.cpClient.FetchContent(ur.ctx, ur.orgID, ur.repoConfig.UUID)
	if err != nil {
		return err
	}
	if content == nil {
		// Content may have not been created yet, if not we don't need to do anything
		return nil
	}

	if err := ur.UpdateCPContent(*content); err != nil {
		if err != nil {
			return fmt.Errorf("could not udpate GPG Key: %w", err)
		}
	}
	if err := ur.UpdateContentOverrides(); err != nil {
		if err != nil {
			return fmt.Errorf("could not update environment overrides: %w", err)
		}
	}
	return nil
}

// UpdateCPContent updates the content in candlepin (name & GPG Key)
func (ur *UpdateRepository) UpdateCPContent(content caliri.ContentDTO) error {
	expected := GenContentDto(ur.repoConfig)
	err := ur.cpClient.UpdateContent(ur.ctx, ur.orgID, ur.repoConfig.UUID, expected)
	if err != nil {
		return fmt.Errorf("could not update repository for gpg key update: %w", err)
	}
	return nil
}

func (ur *UpdateRepository) UpdateContentOverrides() error {
	templates, _, err := ur.daoReg.Template.List(ur.ctx, ur.orgID, api.PaginationData{Limit: -1}, api.TemplateFilterData{
		RepositoryUUIDs: []string{ur.repoConfig.UUID},
	})
	if err != nil {
		return fmt.Errorf("could not list templates for repo config %w", err)
	}
	for _, template := range templates.Data {
		existingOverrides, err := ur.cpClient.FetchContentOverridesForRepo(ur.ctx, template.UUID, ur.repoConfig.Label)
		if err != nil {
			return fmt.Errorf("could not list content for template & repo config %w", err)
		}
		path, err := ur.pulpClient.GetContentPath(ur.ctx)
		if err != nil {
			return err
		}
		expected, err := ContentOverridesForRepo(ur.orgID, ur.domainName, template.UUID, path, ur.repoConfig)
		if err != nil {
			return fmt.Errorf("could not generate overrides %w", err)
		}
		unneeded := UnneededOverrides(existingOverrides, expected)
		if len(unneeded) > 0 {
			err = ur.cpClient.RemoveContentOverrides(ur.ctx, candlepin_client.GetEnvironmentID(template.UUID), unneeded)
			if err != nil {
				return fmt.Errorf("could not remove overrides %w", err)
			}
		}
		if len(expected) > 0 {
			err = ur.cpClient.UpdateContentOverrides(ur.ctx, candlepin_client.GetEnvironmentID(template.UUID), expected)
			if err != nil {
				return fmt.Errorf("could not update overrides %w", err)
			}
		}
	}
	return nil
}
