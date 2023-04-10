package worker

import (
	"context"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type WorkerSuite struct {
	suite.Suite
}

func TestWorkerSuite(t *testing.T) {
	suite.Run(t, &WorkerSuite{})
}

func getObjectsForTest(t *testing.T) (TaskWorkerPool, *queue.MockQueue) {
	config := Config{
		NumWorkers:        3,
		Heartbeat:         time.Minute,
		HeartbeatInterval: time.Millisecond * 12,
	}
	mockQueue := queue.NewMockQueue(t)
	return NewTaskWorkerPool(config, mockQueue, nil), mockQueue
}

func (s *WorkerSuite) TestStartStopWorkers() {
	defer goleak.VerifyNone(s.T())

	workerPool, mockQueue := getObjectsForTest(s.T())

	mockQueue.On("Dequeue", context.Background(), []string(nil)).Times(3).Return(nil, nil)
	mockQueue.On("RefreshHeartbeat", uuid.Nil).Times(3)

	workerPool.StartWorkers(context.Background())
	time.Sleep(time.Millisecond * 5)
	workerPool.Stop()
}
