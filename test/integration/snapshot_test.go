package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
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
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SnapshotSuite struct {
	Suite
	dao   *dao.DaoRegistry
	queue queue.PgQueue
}

func (s *SnapshotSuite) SetupTest() {
	s.Suite.SetupTest()

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
}

func TestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(SnapshotSuite))
}

func (s *SnapshotSuite) TestSnapshot() {
	s.dao = dao.GetDaoRegistry(db.DB)

	// Setup the repository
	accountId := uuid2.NewString()
	repo, err := s.dao.RepositoryConfig.Create(api.RepositoryRequest{
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
	snaps, _, err := s.dao.Snapshot.List(repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), snaps)
	time.Sleep(5 * time.Second)

	// Fetch the repomd.xml to verify its being served
	distPath := fmt.Sprintf("%s/pulp/content/%s/repodata/repomd.xml",
		config.Get().Clients.Pulp.Server,
		snaps.Data[0].RepositoryPath)
	resp, err := http.Get(distPath)
	assert.NoError(s.T(), err)
	defer resp.Body.Close()
	assert.Equal(s.T(), resp.StatusCode, 200)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), body)

	// Update the url
	newUrl := "https://fixtures.pulpproject.org/rpm-with-sha-512/"
	urlUpdated, err := s.dao.RepositoryConfig.Update(accountId, repo.UUID, api.RepositoryRequest{URL: &newUrl})
	assert.NoError(s.T(), err)
	repo, err = s.dao.RepositoryConfig.Fetch(accountId, repo.UUID)
	assert.NoError(s.T(), err)
	repoUuid, err = uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)
	assert.True(s.T(), urlUpdated)
	assert.NoError(s.T(), err)

	s.snapshotAndWait(taskClient, repo, repoUuid, accountId)

	domainName, err := s.dao.Domain.FetchOrCreateDomain(accountId)
	assert.NoError(s.T(), err)

	pulpClient := pulp_client.GetPulpClientWithDomain(context.Background(), domainName)
	remote, err := pulpClient.GetRpmRemoteByName(repo.UUID)

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), repo.URL, remote.Url)

	// Delete the repository
	taskUuid, err := taskClient.Enqueue(queue.Task{
		Typename:       config.DeleteRepositorySnapshotsTask,
		Payload:        tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repo.UUID},
		OrgId:          repo.OrgID,
		RepositoryUUID: repoUuid.String(),
	})
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUuid)

	// Verify the snapshot was deleted
	snaps, _, err = s.dao.Snapshot.List(repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.Error(s.T(), err)
	assert.Empty(s.T(), snaps.Data)
	time.Sleep(5 * time.Second)

	// Fetch the repomd.xml to verify it's not being served
	resp, err = http.Get(distPath)
	assert.NoError(s.T(), err)
	defer resp.Body.Close()
	assert.Equal(s.T(), resp.StatusCode, 404)
	body, err = io.ReadAll(resp.Body)
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), body)
}

func (s *SnapshotSuite) TestSnapshotCancel() {
	s.dao = dao.GetDaoRegistry(db.DB)

	// Setup the repository
	accountId := uuid2.NewString()
	repo, err := s.dao.RepositoryConfig.Create(api.RepositoryRequest{
		Name:      pointy.String(uuid2.NewString()),
		URL:       pointy.String("https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/"),
		AccountID: pointy.String(accountId),
		OrgID:     pointy.String(accountId),
	})
	assert.NoError(s.T(), err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)

	taskClient := client.NewTaskClient(&s.queue)
	taskUuid, err := taskClient.Enqueue(queue.Task{Typename: config.RepositorySnapshotTask, Payload: payloads.SnapshotPayload{}, OrgId: repo.OrgID,
		RepositoryUUID: repoUuid.String()})
	assert.NoError(s.T(), err)
	time.Sleep(time.Millisecond * 500)
	s.cancelAndWait(taskClient, taskUuid, repo)
}

func (s *SnapshotSuite) snapshotAndWait(taskClient client.TaskClient, repo api.RepositoryResponse, repoUuid uuid2.UUID, orgId string) {
	var err error
	taskUuid, err := taskClient.Enqueue(queue.Task{Typename: config.RepositorySnapshotTask, Payload: payloads.SnapshotPayload{}, OrgId: repo.OrgID,
		RepositoryUUID: repoUuid.String()})
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUuid)

	// Verify the snapshot was created
	snaps, _, err := s.dao.Snapshot.List(repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), snaps)
	time.Sleep(5 * time.Second)

	// Fetch the repomd.xml to verify its being served
	distPath := fmt.Sprintf("%s/pulp/content/%s/repodata/repomd.xml",
		config.Get().Clients.Pulp.Server,
		snaps.Data[0].RepositoryPath)
	resp, err := http.Get(distPath)
	assert.NoError(s.T(), err)
	defer resp.Body.Close()
	assert.Equal(s.T(), resp.StatusCode, 200)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), body)
}

func (s *SnapshotSuite) cancelAndWait(taskClient client.TaskClient, taskUUID uuid2.UUID, repo api.RepositoryResponse) {
	var err error
	err = taskClient.SendCancelNotification(context.Background(), taskUUID.String())
	assert.NoError(s.T(), err)

	s.WaitOnCanceledTask(taskUUID)

	// Verify the snapshot was not created
	snaps, _, err := s.dao.Snapshot.List(repo.UUID, api.PaginationData{}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), api.SnapshotCollectionResponse{Data: []api.SnapshotResponse{}}, snaps)
}

func (s *SnapshotSuite) WaitOnTask(taskUUID uuid2.UUID) {
	taskInfo := s.waitOnTask(taskUUID)
	if taskInfo.Error != nil {
		assert.NotNil(s.T(), *taskInfo.Error)
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
