package integration

import (
	"context"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
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

// This is a delete integration tests without any snapshotting
type DeleteTest struct {
	Suite
	dao   *dao.DaoRegistry
	queue queue.PgQueue
	ctx   context.Context
}

func (s *DeleteTest) SetupTest() {
	s.Suite.SetupTest()
	s.ctx = context.Background()
	wkrQueue, err := queue.NewPgQueue(db.GetUrl())
	require.NoError(s.T(), err)
	s.queue = wkrQueue

	wrk := worker.NewTaskWorkerPool(&wkrQueue, m.NewMetrics(prometheus.NewRegistry()))
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

func TestDeleteTest(t *testing.T) {
	suite.Run(t, new(DeleteTest))
}

func (s *DeleteTest) TestSnapshot() {
	s.dao = dao.GetDaoRegistry(db.DB)

	// Setup the repository
	accountId := uuid2.NewString()
	repo, err := s.dao.RepositoryConfig.Create(s.ctx, api.RepositoryRequest{
		Name:      pointy.Pointer(uuid2.NewString()),
		URL:       pointy.Pointer("https://fixtures.pulpproject.org/rpm-unsigned/"),
		AccountID: pointy.Pointer(accountId),
		OrgID:     pointy.Pointer(accountId),
		Snapshot:  pointy.Pointer(false),
	})
	assert.NoError(s.T(), err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)

	// Start the task
	taskClient := client.NewTaskClient(&s.queue)

	// Delete the repository
	taskUuid, err := taskClient.Enqueue(queue.Task{
		Typename:       config.DeleteRepositorySnapshotsTask,
		Payload:        tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repo.UUID},
		OrgId:          repo.OrgID,
		RepositoryUUID: pointy.String(repoUuid.String()),
	})
	assert.NoError(s.T(), err)
	s.WaitOnTask(taskUuid)

	results, _, err := s.dao.RepositoryConfig.List(s.ctx, accountId, api.PaginationData{}, api.FilterData{
		Name: repo.Name,
	})
	assert.NoError(s.T(), err)
	assert.Empty(s.T(), results.Data)
}

func (s *DeleteTest) WaitOnTask(taskUuid uuid2.UUID) {
	// Poll until the task is complete
	taskInfo, err := s.queue.Status(taskUuid)
	assert.NoError(s.T(), err)
	for {
		if taskInfo.Status == config.TaskStatusRunning || taskInfo.Status == config.TaskStatusPending {
			log.Logger.Error().Msg("SLEEPING")
			time.Sleep(1 * time.Second)
		} else {
			break
		}
		taskInfo, err = s.queue.Status(taskUuid)
		assert.NoError(s.T(), err)
	}
	if taskInfo.Error != nil {
		assert.Nil(s.T(), *taskInfo.Error)
	}

	assert.Equal(s.T(), config.TaskStatusCompleted, taskInfo.Status)
}
