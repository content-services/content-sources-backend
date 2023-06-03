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
	wrk.RegisterHandler(tasks.Snapshot, tasks.SnapshotHandler)
	wrk.HeartbeatListener()

	wkrCtx := context.Background()
	go (wrk).StartWorkers(wkrCtx)
	go func() {
		<-wkrCtx.Done()
		wrk.Stop()
	}()
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
	taskUuid, err := taskClient.Enqueue(queue.Task{Typename: tasks.Snapshot, Payload: tasks.SnapshotPayload{}, OrgId: repo.OrgID,
		RepositoryUUID: repoUuid.String()})
	assert.NoError(s.T(), err)

	// Poll until the task is complete
	taskInfo, err := s.queue.Status(taskUuid)
	assert.NoError(s.T(), err)
	for {
		if taskInfo.Status == queue.StatusRunning || taskInfo.Status == queue.StatusPending {
			log.Logger.Error().Msg("SLEEPING")
			time.Sleep(1 * time.Second)
		} else {
			break
		}
		taskInfo, err = s.queue.Status(taskUuid)
		assert.NoError(s.T(), err)
	}
	assert.Equal(s.T(), queue.StatusCompleted, taskInfo.Status)
	assert.Empty(s.T(), taskInfo.Error)

	// Verify the snapshot was created
	snaps, err := s.dao.Snapshot.List(repo.UUID)
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), snaps)
	time.Sleep(5 * time.Second)

	// Fetch the repomd.xml to verify its being served
	distPath := fmt.Sprintf("%s/pulp/content/%s/repodata/repomd.xml",
		config.Get().Clients.Pulp.Server,
		snaps[0].DistributionPath)
	resp, err := http.Get(distPath)
	assert.NoError(s.T(), err)
	defer resp.Body.Close()
	assert.Equal(s.T(), resp.StatusCode, 200)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), body)
}
