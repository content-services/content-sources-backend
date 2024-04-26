package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
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

type SnapshotSuite struct {
	Suite
	dao   *dao.DaoRegistry
	queue queue.PgQueue
	ctx   context.Context
}

func (s *SnapshotSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ctx = context.Background() // Test Context
	wkrQueue, err := queue.NewPgQueue(db.GetUrl())
	require.NoError(s.T(), err)
	s.queue = wkrQueue

	wrk := worker.NewTaskWorkerPool(&wkrQueue, m.NewMetrics(prometheus.NewRegistry()))
	wrk.RegisterHandler(config.RepositorySnapshotTask, tasks.SnapshotHandler)
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

func TestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(SnapshotSuite))
}

func (s *SnapshotSuite) TestSnapshot() {
	s.dao = dao.GetDaoRegistry(db.DB)

	// Setup the repository
	accountId := uuid2.NewString()
	repo, err := s.dao.RepositoryConfig.Create(s.ctx, api.RepositoryRequest{
		Name:      pointy.String(uuid2.NewString()),
		URL:       pointy.String("https://fixtures.pulpproject.org/rpm-unsigned/"),
		AccountID: pointy.String(accountId),
		OrgID:     pointy.String(accountId),
	})
	assert.NoError(s.T(), err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)

	// Start the task
	taskClient := client.NewTaskClient(&s.queue)
	s.snapshotAndWait(taskClient, repo, repoUuid, accountId)

	// Verify the snapshot was created
	snaps, _, err := s.dao.Snapshot.List(s.ctx, repo.OrgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), snaps)
	time.Sleep(5 * time.Second)

	// Fetch the repomd.xml to verify its being served
	distPath := fmt.Sprintf("%s/pulp/content/%s/repodata/repomd.xml",
		config.Get().Clients.Pulp.Server,
		snaps.Data[0].RepositoryPath)
	err = s.getRequest(distPath, identity.Identity{OrgID: accountId, Internal: identity.Internal{OrgID: accountId}}, 200)
	assert.NoError(s.T(), err)

	err = s.getRequest(distPath, identity.Identity{X509: &identity.X509{SubjectDN: "warlin.door"}}, 200)
	assert.NoError(s.T(), err)

	// But can't be served without a valid org id or common dn
	_ = s.getRequest(distPath, identity.Identity{}, 403)

	// Update the url
	newUrl := "https://fixtures.pulpproject.org/rpm-with-sha-512/"
	urlUpdated, err := s.dao.RepositoryConfig.Update(s.ctx, accountId, repo.UUID, api.RepositoryRequest{URL: &newUrl})
	assert.NoError(s.T(), err)
	repo, err = s.dao.RepositoryConfig.Fetch(s.ctx, accountId, repo.UUID)
	assert.NoError(s.T(), err)
	repoUuid, err = uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)
	assert.True(s.T(), urlUpdated)
	assert.NoError(s.T(), err)

	s.snapshotAndWait(taskClient, repo, repoUuid, accountId)

	domainName, err := s.dao.Domain.FetchOrCreateDomain(s.ctx, accountId)
	assert.NoError(s.T(), err)

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)
	remote, err := pulpClient.GetRpmRemoteByName(s.ctx, repo.UUID)

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), repo.URL, remote.Url)

	// Delete the repository
	taskUuid, err := taskClient.Enqueue(queue.Task{
		Typename:       config.DeleteRepositorySnapshotsTask,
		Payload:        tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repo.UUID},
		OrgId:          repo.OrgID,
		RepositoryUUID: pointy.String(repoUuid.String()),
	})
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUuid)

	// Verify the snapshot was deleted
	snaps, _, err = s.dao.Snapshot.List(s.ctx, repo.OrgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.Error(s.T(), err)
	assert.Empty(s.T(), snaps.Data)
	time.Sleep(5 * time.Second)

	// Fetch the repomd.xml to verify it's not being served
	err = s.getRequest(distPath, identity.Identity{OrgID: accountId, Internal: identity.Internal{OrgID: accountId}}, 404)
	assert.NoError(s.T(), err)
}

type loggingTransport struct{}

func (s loggingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	bytes, _ := httputil.DumpRequestOut(r, true)

	resp, err := http.DefaultTransport.RoundTrip(r)
	// err is returned after dumping the response

	respBytes, _ := httputil.DumpResponse(resp, true)
	bytes = append(bytes, respBytes...)

	fmt.Printf("%s\n", bytes)

	return resp, err
}

func (s *SnapshotSuite) getRequest(url string, id identity.Identity, expectedCode int) error {
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

func (s *SnapshotSuite) TestSnapshotCancel() {
	s.dao = dao.GetDaoRegistry(db.DB)

	// Setup the repository
	accountId := uuid2.NewString()
	repo, err := s.dao.RepositoryConfig.Create(s.ctx, api.RepositoryRequest{
		Name:      pointy.String(uuid2.NewString()),
		URL:       pointy.String("https://fixtures.pulpproject.org/rpm-unsigned/"),
		AccountID: pointy.String(accountId),
		OrgID:     pointy.String(accountId),
	})
	assert.NoError(s.T(), err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)

	taskClient := client.NewTaskClient(&s.queue)
	taskUuid, err := taskClient.Enqueue(queue.Task{Typename: config.RepositorySnapshotTask, Payload: payloads.SnapshotPayload{}, OrgId: repo.OrgID,
		RepositoryUUID: pointy.String(repoUuid.String())})
	assert.NoError(s.T(), err)
	time.Sleep(time.Millisecond * 500)
	s.cancelAndWait(taskClient, taskUuid, repo)
}

func (s *SnapshotSuite) snapshotAndWait(taskClient client.TaskClient, repo api.RepositoryResponse, repoUuid uuid2.UUID, orgId string) {
	var err error
	taskUuid, err := taskClient.Enqueue(queue.Task{Typename: config.RepositorySnapshotTask, Payload: payloads.SnapshotPayload{}, OrgId: repo.OrgID,
		RepositoryUUID: pointy.String(repoUuid.String())})
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUuid)

	// Verify the snapshot was created
	snaps, _, err := s.dao.Snapshot.List(s.ctx, repo.OrgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
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

func (s *SnapshotSuite) cancelAndWait(taskClient client.TaskClient, taskUUID uuid2.UUID, repo api.RepositoryResponse) {
	var err error
	err = taskClient.SendCancelNotification(context.Background(), taskUUID.String())
	assert.NoError(s.T(), err)

	s.WaitOnCanceledTask(taskUUID)

	// Verify the snapshot was not created
	snaps, _, err := s.dao.Snapshot.List(s.ctx, repo.OrgID, repo.UUID, api.PaginationData{}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), api.SnapshotCollectionResponse{Data: []api.SnapshotResponse{}}, snaps)
}

func (s *SnapshotSuite) WaitOnTask(taskUUID uuid2.UUID) {
	taskInfo := s.waitOnTask(taskUUID)
	if taskInfo.Error != nil {
		// if there is an error, throw and assertion so the error gets printed
		assert.Empty(s.T(), *taskInfo.Error)
	}
	assert.Equal(s.T(), config.TaskStatusCompleted, taskInfo.Status)
}

func (s *SnapshotSuite) WaitOnCanceledTask(taskUUID uuid2.UUID) {
	taskInfo := s.waitOnTask(taskUUID)
	require.NotNil(s.T(), taskInfo.Error)
	assert.NotEmpty(s.T(), *taskInfo.Error)
	assert.Equal(s.T(), config.TaskStatusCanceled, taskInfo.Status)
}

func (s *SnapshotSuite) waitOnTask(taskUUID uuid2.UUID) *models.TaskInfo {
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
