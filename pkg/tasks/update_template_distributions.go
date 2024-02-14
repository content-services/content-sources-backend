package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/exp/slices"
)

func UpdateTemplateDistributionsHandler(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error {
	opts := payloads.UpdateTemplateDistributionsPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for UpdateTemplateDistributions")
	}

	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	ctxWithLogger := logger.WithContext(ctx)

	daoReg := dao.GetDaoRegistry(db.DB)
	domainName, err := daoReg.Domain.Fetch(task.OrgId)
	if err != nil {
		return err
	}

	rhDomainName, err := daoReg.Domain.Fetch(config.RedHatOrg)
	if err != nil {
		return err
	}

	pulpClient := pulp_client.GetPulpClientWithDomain(ctxWithLogger, domainName)

	t := UpdateTemplateDistributions{
		orgId:          task.OrgId,
		domainName:     domainName,
		rhDomainName:   rhDomainName,
		repositoryUUID: task.RepositoryUUID,
		daoReg:         daoReg,
		pulpClient:     pulpClient,
		task:           task,
		payload:        &opts,
		queue:          queue,
		ctx:            ctx,
		logger:         logger,
	}

	return t.Run()
}

type UpdateTemplateDistributions struct {
	orgId          string
	domainName     string
	rhDomainName   string
	repositoryUUID uuid.UUID
	daoReg         *dao.DaoRegistry
	pulpClient     pulp_client.PulpClient
	payload        *payloads.UpdateTemplateDistributionsPayload
	task           *models.TaskInfo
	queue          *queue.Queue
	ctx            context.Context
	logger         *zerolog.Logger
}

func (t *UpdateTemplateDistributions) Run() error {
	if t.payload.RepoConfigUUIDs == nil {
		return nil
	}

	repoConfigDistributionHref := map[string]string{} // mapping to associate each repo config to a distribution href

	reposAdded, reposRemoved, reposUnchanged, allRepos, err := t.daoReg.Template.GetRepoChanges(t.payload.TemplateUUID, t.payload.RepoConfigUUIDs)
	if err != nil {
		return err
	}

	l := api.ListSnapshotByDateRequest{Date: t.payload.TemplateDate, RepositoryUUIDS: allRepos}
	snapshots, err := t.daoReg.Snapshot.FetchSnapshotsModelByDateAndRepository(t.orgId, l)
	if err != nil {
		return err
	}

	if reposAdded != nil {
		err := t.handleReposAdded(reposAdded, snapshots, repoConfigDistributionHref)
		if err != nil {
			return err
		}
	}

	if reposRemoved != nil {
		err := t.handleReposRemoved(reposRemoved)
		if err != nil {
			return err
		}
		keepRepoConfigUUIDs := append(reposUnchanged, reposAdded...)
		err = t.daoReg.Template.DeleteTemplateRepoConfigs(t.payload.TemplateUUID, keepRepoConfigUUIDs)
		if err != nil {
			return err
		}
	}

	if reposUnchanged != nil {
		err := t.handleReposUnchanged(reposUnchanged, snapshots, repoConfigDistributionHref)
		if err != nil {
			return err
		}
	}

	err = t.daoReg.Template.UpdateDistributionHrefs(t.payload.TemplateUUID, t.payload.RepoConfigUUIDs, repoConfigDistributionHref)
	if err != nil {
		return err
	}

	return nil
}

func (t *UpdateTemplateDistributions) handleReposAdded(reposAdded []string, snapshots []models.Snapshot, repoConfigDistributionHref map[string]string) error {
	for _, repoConfigUUID := range reposAdded {
		repo, err := t.daoReg.RepositoryConfig.Fetch(t.orgId, repoConfigUUID)
		if err != nil {
			return err
		}

		// Configure client for org
		if repo.OrgID == config.RedHatOrg {
			t.pulpClient = t.pulpClient.WithDomain(t.rhDomainName)
		} else {
			t.pulpClient = t.pulpClient.WithDomain(t.domainName)
		}

		snapIndex := slices.IndexFunc(snapshots, func(s models.Snapshot) bool {
			return s.RepositoryConfigurationUUID == repoConfigUUID
		})

		// Create content guard
		var contentGuardHref *string
		if t.orgId != config.RedHatOrg && config.Get().Clients.Pulp.CustomRepoContentGuards {
			href, err := t.pulpClient.CreateOrUpdateGuardsForOrg(t.orgId)
			if err != nil {
				return fmt.Errorf("could not fetch/create/update content guard: %w", err)
			}
			contentGuardHref = &href
		}

		distPath, distName, err := getDistPathAndName(repo, t.payload.TemplateUUID, snapshots[snapIndex].UUID)
		if err != nil {
			return err
		}

		// Create distribution
		distTask, err := t.pulpClient.CreateRpmDistribution(snapshots[snapIndex].PublicationHref, distName, distPath, contentGuardHref)
		if err != nil {
			return err
		}

		distResp, err := t.pulpClient.PollTask(*distTask)
		if err != nil {
			return err
		}

		distHrefPtr := pulp_client.SelectRpmDistributionHref(distResp)
		if distHrefPtr == nil {
			return fmt.Errorf("could not find a distribution href in task: %v", *distResp.PulpHref)
		}

		repoConfigDistributionHref[repoConfigUUID] = *distHrefPtr
	}
	return nil
}

func (t *UpdateTemplateDistributions) handleReposUnchanged(reposUnchanged []string, snapshots []models.Snapshot, repoConfigDistributionHref map[string]string) error {
	for _, repoConfigUUID := range reposUnchanged {
		repo, err := t.daoReg.RepositoryConfig.Fetch(t.orgId, repoConfigUUID)
		if err != nil {
			return err
		}

		// Configure client for org
		if repo.OrgID == config.RedHatOrg {
			t.pulpClient = t.pulpClient.WithDomain(t.rhDomainName)
		} else {
			t.pulpClient = t.pulpClient.WithDomain(t.domainName)
		}

		snapIndex := slices.IndexFunc(snapshots, func(s models.Snapshot) bool {
			return s.RepositoryConfigurationUUID == repoConfigUUID
		})

		distPath, distName, err := getDistPathAndName(repo, t.payload.TemplateUUID, snapshots[snapIndex].UUID)
		if err != nil {
			return err
		}

		distHref, err := t.daoReg.Template.GetDistributionHref(t.payload.TemplateUUID, repoConfigUUID)
		if err != nil {
			return err
		}
		taskHref, err := t.pulpClient.UpdateRpmDistribution(distHref, snapshots[snapIndex].PublicationHref, distName, distPath)
		if err != nil {
			return err
		}
		_, err = t.pulpClient.PollTask(taskHref)
		if err != nil {
			return err
		}
		distResp, err := t.pulpClient.FindDistributionByPath(distPath)
		if err != nil {
			return err
		}
		repoConfigDistributionHref[repoConfigUUID] = *distResp.PulpHref
	}
	return nil
}

func (t *UpdateTemplateDistributions) handleReposRemoved(reposRemoved []string) error {
	for _, repoConfigUUID := range reposRemoved {
		repo, err := t.daoReg.RepositoryConfig.Fetch(t.orgId, repoConfigUUID)
		if err != nil {
			return err
		}

		// Configure client for org
		if repo.OrgID == config.RedHatOrg {
			t.pulpClient = t.pulpClient.WithDomain(t.rhDomainName)
		} else {
			t.pulpClient = t.pulpClient.WithDomain(t.domainName)
		}

		distHref, err := t.daoReg.Template.GetDistributionHref(t.payload.TemplateUUID, repoConfigUUID)
		if err != nil {
			return err
		}
		taskHref, err := t.pulpClient.DeleteRpmDistribution(distHref)
		if err != nil {
			return err
		}
		_, err = t.pulpClient.PollTask(taskHref)
		if err != nil {
			return err
		}
	}
	return nil
}

func getDistPathAndName(repo api.RepositoryResponse, templateUUID string, snapshotUUID string) (distPath string, distName string, err error) {
	if repo.OrgID == config.RedHatOrg {
		path, err := getRHRepoContentPath(repo.URL)
		if err != nil {
			return "", "", err
		}
		distPath = fmt.Sprintf("templates/%v/%v", templateUUID, path)
	} else {
		distPath = fmt.Sprintf("templates/%v/%v", templateUUID, repo.UUID)
	}

	distName = templateUUID + "/" + snapshotUUID
	return distPath, distName, nil
}

func getRHRepoContentPath(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Path[1 : len(u.Path)-1], nil
}
