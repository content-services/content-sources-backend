package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/tasks/worker"
	"github.com/content-services/content-sources-backend/pkg/utils"
	uuid2 "github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UpdateLatestSnapshotSuite struct {
	Suite
	dao        *dao.DaoRegistry
	queue      queue.PgQueue
	taskClient client.TaskClient
	cpClient   candlepin_client.CandlepinClient
}

func TestUpdateLatestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(UpdateLatestSnapshotSuite))
}

func (s *UpdateLatestSnapshotSuite) SetupTest() {
	s.Suite.SetupTest()

	wkrQueue, err := queue.NewPgQueue(db.GetUrl())
	require.NoError(s.T(), err)
	s.queue = wkrQueue

	s.taskClient = client.NewTaskClient(&s.queue)
	s.cpClient = candlepin_client.NewCandlepinClient()
	require.NoError(s.T(), err)

	wrk := worker.NewTaskWorkerPool(&wkrQueue, m.NewMetrics(prometheus.NewRegistry()))
	wrk.RegisterHandler(config.RepositorySnapshotTask, tasks.SnapshotHandler)
	wrk.RegisterHandler(config.DeleteRepositorySnapshotsTask, tasks.DeleteSnapshotHandler)
	wrk.RegisterHandler(config.UpdateLatestSnapshotTask, tasks.UpdateLatestSnapshotHandler)
	wrk.HeartbeatListener()

	wkrCtx := context.Background()
	go (wrk).StartWorkers(wkrCtx)
	go func() {
		<-wkrCtx.Done()
		wrk.Stop()
	}()

	// Force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"

	// Force content guard setup
	config.Get().Clients.Pulp.CustomRepoContentGuards = true
	config.Get().Clients.Pulp.GuardSubjectDn = "warlin.door"
}

func (s *UpdateLatestSnapshotSuite) TestUpdateLatestSnapshot() {
	config.Get().Features.Snapshots.Enabled = true
	err := config.ConfigureTang()
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), config.Tang)

	s.dao = dao.GetDaoRegistry(db.DB)
	ctx := context.Background()
	orgID := uuid2.NewString()

	domainName, err := s.dao.Domain.FetchOrCreateDomain(ctx, orgID)
	assert.NoError(s.T(), err)

	repo := s.createAndSyncRepository(orgID, "https://rverdile.fedorapeople.org/dummy-repos/comps/repo1/")

	// Create template with use latest
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr("includes rpm unsigned"),
		RepositoryUUIDS: []string{repo.UUID},
		OrgID:           utils.Ptr(repo.OrgID),
		UseLatest:       utils.Ptr(true),
		Arch:            utils.Ptr(config.X8664),
		Version:         utils.Ptr(config.El8),
	}
	tempResp1, err := s.dao.Template.Create(ctx, reqTemplate)
	assert.NoError(s.T(), err)
	s.updateTemplateContentAndWait(orgID, tempResp1.UUID, []string{repo.UUID})

	// Verify the correct snapshot content is being served by the template
	rpmPath := fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v/Packages/s", config.Get().Clients.Pulp.Server, domainName, tempResp1.UUID, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)

	// Create template with date
	reqTemplate = api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr("includes rpm unsigned"),
		RepositoryUUIDS: []string{repo.UUID},
		OrgID:           utils.Ptr(repo.OrgID),
		Arch:            utils.Ptr(config.X8664),
		Version:         utils.Ptr(config.El8),
		Date:            utils.Ptr(time.Now()),
	}
	tempResp2, err := s.dao.Template.Create(ctx, reqTemplate)
	assert.NoError(s.T(), err)
	s.updateTemplateContentAndWait(orgID, tempResp2.UUID, []string{repo.UUID})

	// Verify the correct snapshot content is being served by the template
	rpmPath = fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v/Packages/s", config.Get().Clients.Pulp.Server, domainName, tempResp2.UUID, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)

	// Change the URL of the repo, create a new snapshot, and update template snapshots
	repoNewURL := "https://rverdile.fedorapeople.org/dummy-repos/comps/repo2/"
	_, err = s.dao.RepositoryConfig.Update(ctx, orgID, repo.UUID, api.RepositoryUpdateRequest{URL: &repoNewURL})
	assert.NoError(s.T(), err)

	repo, err = s.dao.RepositoryConfig.Fetch(ctx, orgID, repo.UUID)
	assert.NoError(s.T(), err)

	repoUUID, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)
	s.snapshotAndWait(s.taskClient, repo, repoUUID, orgID)
	s.updateLatestSnapshotAndWait(orgID, repo.UUID)

	// Verify that template1 serves the new snapshot and template2 serves the original snapshot
	rpmPath = fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v/Packages/b", config.Get().Clients.Pulp.Server, domainName, tempResp1.UUID, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)

	rpmPath = fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v/Packages/s", config.Get().Clients.Pulp.Server, domainName, tempResp2.UUID, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)
}

func (s *UpdateLatestSnapshotSuite) updateTemplateContentAndWait(orgId string, tempUUID string, repoConfigUUIDS []string) payloads.UpdateTemplateContentPayload {
	var err error
	payload := payloads.UpdateTemplateContentPayload{
		TemplateUUID:    tempUUID,
		RepoConfigUUIDs: repoConfigUUIDS,
	}
	task := queue.Task{
		Typename: config.UpdateTemplateContentTask,
		Payload:  payload,
		OrgId:    orgId,
	}

	taskUUID, err := s.taskClient.Enqueue(task)
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUUID)

	taskInfo, err := s.queue.Status(taskUUID)
	assert.NoError(s.T(), err)

	err = json.Unmarshal(taskInfo.Payload, &payload)
	assert.NoError(s.T(), err)

	return payload
}

func (s *UpdateLatestSnapshotSuite) updateLatestSnapshotAndWait(orgId string, repoConfigUUID string) tasks.UpdateLatestSnapshotPayload {
	var err error
	payload := tasks.UpdateLatestSnapshotPayload{
		RepositoryConfigUUID: repoConfigUUID,
	}
	task := queue.Task{
		Typename: config.UpdateLatestSnapshotTask,
		Payload:  payload,
		OrgId:    orgId,
	}

	taskUUID, err := s.taskClient.Enqueue(task)
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUUID)

	taskInfo, err := s.queue.Status(taskUUID)
	assert.NoError(s.T(), err)

	err = json.Unmarshal(taskInfo.Payload, &payload)
	assert.NoError(s.T(), err)

	return payload
}
