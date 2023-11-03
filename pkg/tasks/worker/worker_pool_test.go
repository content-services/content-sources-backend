package worker

import (
	"context"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	uuid2 "github.com/google/uuid"
	"github.com/stretchr/testify/mock"
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
	mockQueue := queue.NewMockQueue(t)
	return NewTaskWorkerPool(mockQueue, nil), mockQueue
}

func (s *WorkerSuite) TestStartStopWorkers() {
	defer goleak.VerifyNone(s.T())

	workerPool, mockQueue := getObjectsForTest(s.T())
	s.T().Setenv("TASKING_WORKER_COUNT", "3")

	ctx, cancelFunc := context.WithCancel(context.Background())
	mockQueue.On("Dequeue", mock.Anything, []string(nil)).Return(&models.TaskInfo{}, nil)
	mockQueue.On("ListenForCancel", mock.Anything, uuid2.Nil, mock.Anything).Return(nil, nil)

	workerPool.StartWorkers(ctx)
	time.Sleep(time.Millisecond * 2)
	workerPool.Stop()
	cancelFunc()
}
