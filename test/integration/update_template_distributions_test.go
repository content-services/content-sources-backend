package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/tasks/worker"
	uuid2 "github.com/google/uuid"
	"github.com/openlyinc/pointy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UpdateTemplateDistributionsSuite struct {
	Suite
	dao        *dao.DaoRegistry
	queue      queue.PgQueue
	taskClient client.TaskClient
}

func (s *UpdateTemplateDistributionsSuite) SetupTest() {
	s.Suite.SetupTest()

	wkrQueue, err := queue.NewPgQueue(db.GetUrl())
	require.NoError(s.T(), err)
	s.queue = wkrQueue

	s.taskClient = client.NewTaskClient(&s.queue)

	wrk := worker.NewTaskWorkerPool(&wkrQueue, m.NewMetrics(prometheus.NewRegistry()))
	wrk.RegisterHandler(config.RepositorySnapshotTask, tasks.SnapshotHandler)
	wrk.RegisterHandler(config.UpdateTemplateDistributionsTask, tasks.UpdateTemplateDistributionsHandler)
	wrk.RegisterHandler(config.DeleteRepositorySnapshotsTask, tasks.DeleteSnapshotHandler)
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

func TestUpdateTemplateDistributionsSuite(t *testing.T) {
	suite.Run(t, new(UpdateTemplateDistributionsSuite))
}

func (s *UpdateTemplateDistributionsSuite) TestUpdateTemplateDistributions() {
	s.dao = dao.GetDaoRegistry(db.DB)

	orgID := uuid2.NewString()
	repo1 := s.createAndSyncRepository(orgID, "https://fixtures.pulpproject.org/rpm-unsigned/")
	repo2 := s.createAndSyncRepository(orgID, "https://rverdile.fedorapeople.org/dummy-repos/comps/repo1/")

	domainName, err := s.dao.Domain.Fetch(orgID)
	assert.NoError(s.T(), err)

	reqTemplate := api.TemplateRequest{
		Name:            pointy.Pointer("test template"),
		Description:     pointy.Pointer("includes rpm unsigned"),
		RepositoryUUIDS: []string{repo1.UUID},
		OrgID:           pointy.Pointer(repo1.OrgID),
	}
	tempResp, err := s.dao.Template.Create(reqTemplate)
	assert.NoError(s.T(), err)

	s.updateTemplatesAndWait(orgID, tempResp.UUID, []string{repo1.UUID})
	distPath := fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v", config.Get().Clients.Pulp.Server, domainName, tempResp.UUID, repo1.UUID)
	err = s.getRequest(distPath, identity.Identity{OrgID: repo1.OrgID, Internal: identity.Internal{OrgID: repo1.OrgID}}, 200)
	assert.NoError(s.T(), err)

	updateReq := api.TemplateUpdateRequest{
		RepositoryUUIDS: []string{repo1.UUID, repo2.UUID},
		OrgID:           &orgID,
	}
	_, err = s.dao.Template.Update(orgID, tempResp.UUID, updateReq)
	assert.NoError(s.T(), err)

	s.updateTemplatesAndWait(orgID, tempResp.UUID, []string{repo1.UUID, repo2.UUID})
	distPath = fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v", config.Get().Clients.Pulp.Server, domainName, tempResp.UUID, repo1.UUID)
	err = s.getRequest(distPath, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 200)
	assert.NoError(s.T(), err)
	distPath = fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v", config.Get().Clients.Pulp.Server, domainName, tempResp.UUID, repo2.UUID)
	err = s.getRequest(distPath, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 200)
	assert.NoError(s.T(), err)

	updateReq = api.TemplateUpdateRequest{
		RepositoryUUIDS: []string{repo1.UUID},
		OrgID:           &orgID,
	}
	_, err = s.dao.Template.Update(orgID, tempResp.UUID, updateReq)
	assert.NoError(s.T(), err)

	s.updateTemplatesAndWait(orgID, tempResp.UUID, []string{repo1.UUID})
	distPath = fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v", config.Get().Clients.Pulp.Server, domainName, tempResp.UUID, repo1.UUID)
	err = s.getRequest(distPath, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 200)
	assert.NoError(s.T(), err)
	distPath = fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v", config.Get().Clients.Pulp.Server, domainName, tempResp.UUID, repo2.UUID)
	err = s.getRequest(distPath, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 404)
	assert.NoError(s.T(), err)
}

func (s *UpdateTemplateDistributionsSuite) updateTemplatesAndWait(orgId string, tempUUID string, repoConfigUUIDS []string) {
	var err error
	payload := payloads.UpdateTemplateDistributionsPayload{
		TemplateUUID:    tempUUID,
		RepoConfigUUIDs: repoConfigUUIDS,
	}
	task := queue.Task{
		Typename: config.UpdateTemplateDistributionsTask,
		Payload:  payload,
		OrgId:    orgId,
	}

	taskUUID, err := s.taskClient.Enqueue(task)
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUUID)
}

func (s *UpdateTemplateDistributionsSuite) snapshotAndWait(taskClient client.TaskClient, repo api.RepositoryResponse, repoUuid uuid2.UUID, orgId string) {
	var err error
	taskUuid, err := taskClient.Enqueue(queue.Task{Typename: config.RepositorySnapshotTask, Payload: payloads.SnapshotPayload{}, OrgId: repo.OrgID,
		RepositoryUUID: pointy.String(repoUuid.String())})
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUuid)

	// Verify the snapshot was created
	snaps, _, err := s.dao.Snapshot.List(repo.OrgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), snaps)
	time.Sleep(5 * time.Second)

	// Fetch the repomd.xml to verify its being served
	distPath := fmt.Sprintf("%s/pulp/content/%s/repodata/repomd.xml",
		config.Get().Clients.Pulp.Server,
		snaps.Data[0].RepositoryPath)
	err = s.getRequest(distPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)
}

func (s *UpdateTemplateDistributionsSuite) WaitOnTask(taskUUID uuid2.UUID) {
	taskInfo := s.waitOnTask(taskUUID)
	if taskInfo.Error != nil {
		// if there is an error, throw and assertion so the error gets printed
		assert.Empty(s.T(), *taskInfo.Error)
	}
	assert.Equal(s.T(), config.TaskStatusCompleted, taskInfo.Status)
}

func (s *UpdateTemplateDistributionsSuite) WaitOnCanceledTask(taskUUID uuid2.UUID) {
	taskInfo := s.waitOnTask(taskUUID)
	require.NotNil(s.T(), taskInfo.Error)
	assert.NotEmpty(s.T(), *taskInfo.Error)
	assert.Equal(s.T(), config.TaskStatusCanceled, taskInfo.Status)
}

func (s *UpdateTemplateDistributionsSuite) waitOnTask(taskUUID uuid2.UUID) *models.TaskInfo {
	// Poll until the task is complete
	taskInfo, err := s.queue.Status(taskUUID)
	assert.NoError(s.T(), err)
	for {
		if taskInfo.Status == config.TaskStatusRunning || taskInfo.Status == config.TaskStatusPending {
			log.Logger.Error().Msg("SLEEPING")
			time.Sleep(1 * time.Second)
		} else {
			break
		}
		taskInfo, err = s.queue.Status(taskUUID)
		assert.NoError(s.T(), err)
	}
	return taskInfo
}

func (s *UpdateTemplateDistributionsSuite) getRequest(url string, id identity.Identity, expectedCode int) error {
	client := http.Client{Transport: loggingTransport{}}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	js, err := json.Marshal(identity.XRHID{Identity: id})
	if err != nil {
		return err
	}
	req.Header = http.Header{}
	req.Header.Add(api.IdentityHeader, base64.StdEncoding.EncodeToString(js))
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	assert.Equal(s.T(), expectedCode, res.StatusCode)

	return nil
}

func (s *UpdateTemplateDistributionsSuite) createAndSyncRepository(orgID string, url string) api.RepositoryResponse {
	// Setup the repository
	repo, err := s.dao.RepositoryConfig.Create(api.RepositoryRequest{
		Name:      pointy.String(uuid2.NewString()),
		URL:       pointy.String(url),
		AccountID: pointy.String(orgID),
		OrgID:     pointy.String(orgID),
	})
	assert.NoError(s.T(), err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)

	// Start the task
	s.snapshotAndWait(s.taskClient, repo, repoUuid, orgID)
	return repo
}
