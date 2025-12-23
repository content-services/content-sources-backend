package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type QueueSuite struct {
	suite.Suite
	queue PgQueue
	tx    *pgx.Tx
}

func (s *QueueSuite) TearDownTest() {
	err := (*s.tx).Rollback(context.Background())
	if err != nil {
		require.NoError(s.T(), err)
	}
}

func (s *QueueSuite) SetupTest() {
	pgxQueue, err := NewPgxPool(context.Background(), db.GetUrl())
	require.NoError(s.T(), err)
	pgxConn, err := pgxQueue.Acquire(context.Background())
	require.NoError(s.T(), err)
	tx, err := pgxConn.Begin(context.Background())
	require.NoError(s.T(), err)

	config.RequeueableTasks = append(config.RequeueableTasks, testTaskType)

	pgQueue := PgQueue{
		Pool:      &FakePgxPoolWrapper{tx: &tx, conn: pgxConn},
		dequeuers: newDequeuers(),
	}

	s.tx = &tx
	s.queue = pgQueue

	err = s.queue.RemoveAllTasks()
	require.NoError(s.T(), err)
}

func TestQueueSuite(t *testing.T) {
	q := QueueSuite{}
	suite.Run(t, &q)
}

type testTaskPayload struct {
	Msg string
}

const testTaskType = "test"

var testTask = Task{
	Typename:     testTaskType,
	Payload:      testTaskPayload{Msg: "payload"},
	Dependencies: nil,
	OrgId:        "12345",
	ObjectUUID:   utils.Ptr(uuid.NewString()),
	ObjectType:   utils.Ptr("Mytype"),
}

func (s *QueueSuite) TestEnqueue() {
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	info, err := s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusPending, info.Status)
	assert.NotNil(s.T(), info.Queued)
	assert.Nil(s.T(), info.Started)
	assert.Nil(s.T(), info.Finished)
	assert.Equal(s.T(), testTask.OrgId, info.OrgId)
	assert.Equal(s.T(), *testTask.ObjectUUID, info.ObjectUUID.String())
	assert.Equal(s.T(), *testTask.ObjectType, *info.ObjectType)
}

func (s *QueueSuite) TestUpdatePayload() {
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	taskInfo, err := s.queue.Status(id)
	require.NoError(s.T(), err)

	_, err = s.queue.UpdatePayload(taskInfo, testTaskPayload{Msg: "Updated"})
	require.NoError(s.T(), err)

	taskInfo, err = s.queue.Status(id)
	require.NoError(s.T(), err)

	payload := testTaskPayload{}
	err = json.Unmarshal(taskInfo.Payload, &payload)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), payload.Msg, "Updated")
}

func (s *QueueSuite) TestDequeue() {
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	testTaskCopy := testTask
	testTaskCopy.Typename = "missed type"
	id, err = s.queue.Enqueue(&testTaskCopy)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	info, err := s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusRunning, info.Status)
	assert.NotNil(s.T(), info.Started)
	assert.Equal(s.T(), info.Typename, testTask.Typename)
}

func (s *QueueSuite) TestFinish() {
	// Test finishing task with success
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Finish(id, nil)
	require.NoError(s.T(), err)

	info, err := s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.NotNil(s.T(), info.Finished)
	assert.Equal(s.T(), config.TaskStatusCompleted, info.Status)

	// Test finishing task with error and dependency
	id, err = s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	testTask2 := testTask
	testTask2.Dependencies = []uuid.UUID{id}
	id2, err := s.queue.Enqueue(&testTask2)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id2)

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Finish(id, fmt.Errorf("something went wrong"))
	require.NoError(s.T(), err)

	info, err = s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.NotNil(s.T(), info.Finished)
	assert.Equal(s.T(), config.TaskStatusFailed, info.Status)
	assert.Equal(s.T(), "something went wrong", *info.Error)

	info, err = s.queue.Status(id2)
	require.NoError(s.T(), err)
	assert.Nil(s.T(), info.Started)
	assert.Nil(s.T(), info.Finished)
	assert.Equal(s.T(), config.TaskStatusCanceled, info.Status)

	// Test finishing task with very large error
	id, err = s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	errorMsg := ""
	for i := 0; i < 10000; i++ {
		errorMsg = errorMsg + "a"
	}
	err = s.queue.Finish(id, errors.New(errorMsg))
	require.NoError(s.T(), err)

	info, err = s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.NotNil(s.T(), info.Finished)
	assert.Equal(s.T(), config.TaskStatusFailed, info.Status)

	assert.Equal(s.T(), 4000, len(*info.Error))

	// Test finish where error has non-UTF8 chars
	id, err = s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Finish(id, fmt.Errorf("something went \xc5wrong"))
	require.NoError(s.T(), err)

	info, err = s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.NotNil(s.T(), info.Finished)
	assert.Equal(s.T(), config.TaskStatusFailed, info.Status)
	assert.Equal(s.T(), "something went wrong", *info.Error)
}

func (s *QueueSuite) TestRequeue() {
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	info, err := s.queue.Status(id)
	require.NoError(s.T(), err)
	originalQueueTime := info.Queued

	// Test cannot requeue pending task
	err = s.queue.Requeue(id)
	require.ErrorIs(s.T(), err, ErrNotRunning)

	// Test can requeue running task
	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Requeue(id)
	require.NoError(s.T(), err)

	info, err = s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusPending, info.Status)
	assert.True(s.T(), info.Queued.After(*originalQueueTime))

	// Test cannot requeue finished task
	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Finish(id, nil)
	require.NoError(s.T(), err)

	err = s.queue.Requeue(id)
	assert.ErrorIs(s.T(), err, ErrNotRunning)
}

func (s *QueueSuite) TestRequeueExceedRetries() {
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	for i := 0; i < MaxTaskRetries; i++ {
		_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
		require.NoError(s.T(), err)

		err = s.queue.Requeue(id)
		require.NoError(s.T(), err)
	}

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Requeue(id)
	require.Error(s.T(), err)

	info, err := s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusFailed, info.Status)
}

func (s *QueueSuite) TestRequeueFailedTasks() {
	config.Get().Tasking.RetryWaitUpperBound = 0

	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	testTask2 := testTask
	testTask2.Dependencies = []uuid.UUID{id}
	id2, err := s.queue.Enqueue(&testTask2)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id2)

	info, err := s.queue.Status(id)
	require.NoError(s.T(), err)
	originalQueueTime := info.Queued

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Finish(id, fmt.Errorf("something went wrong"))
	require.NoError(s.T(), err)

	// Test requeue failed task
	err = s.queue.RequeueFailedTasks([]string{testTaskType})
	assert.NoError(s.T(), err)

	info, err = s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusPending, info.Status)
	assert.Nil(s.T(), info.Finished)
	assert.Nil(s.T(), info.Started)
	assert.Equal(s.T(), uuid.Nil, info.Token)
	assert.True(s.T(), info.Queued.After(*originalQueueTime))

	info, err = s.queue.Status(id2)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusPending, info.Status)
	assert.Nil(s.T(), info.Finished)
	assert.Nil(s.T(), info.Started)
	assert.Equal(s.T(), uuid.Nil, info.Token)
	assert.True(s.T(), info.Queued.After(*originalQueueTime))
}

func (s *QueueSuite) TestCannotRequeueCanceledTasks() {
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	_, err = s.queue.Status(id)
	require.NoError(s.T(), err)

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Cancel(context.Background(), id)
	require.NoError(s.T(), err)

	err = s.queue.Requeue(id)
	assert.ErrorIs(s.T(), err, ErrTaskCanceled)
}

func (s *QueueSuite) TestCannotRequeueCanceledFailedTasks() {
	config.Get().Tasking.RetryWaitUpperBound = 0

	// Test when task fails right after cancellation
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	info, err := s.queue.Status(id)
	require.NoError(s.T(), err)
	originalQueueTime := info.Queued

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Cancel(context.Background(), id)
	require.NoError(s.T(), err)

	err = s.queue.Finish(id, fmt.Errorf("something went wrong"))
	require.NoError(s.T(), err)

	err = s.queue.RequeueFailedTasks([]string{testTaskType})
	assert.NoError(s.T(), err)

	info, err = s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusFailed, info.Status)
	assert.Equal(s.T(), true, info.CancelAttempted)
	assert.True(s.T(), info.Queued.Equal(*originalQueueTime))

	// Test when task fails right before cancellation
	id, err = s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	info, err = s.queue.Status(id)
	require.NoError(s.T(), err)
	originalQueueTime = info.Queued

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Finish(id, fmt.Errorf("something went wrong"))
	require.NoError(s.T(), err)

	err = s.queue.Cancel(context.Background(), id)
	require.NoError(s.T(), err)

	err = s.queue.RequeueFailedTasks([]string{testTaskType})
	assert.NoError(s.T(), err)

	info, err = s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusFailed, info.Status)
	assert.Equal(s.T(), true, info.CancelAttempted)
	assert.True(s.T(), info.Queued.Equal(*originalQueueTime))
}

func (s *QueueSuite) TestRequeueFailedTasksExceedRetries() {
	config.Get().Tasking.RetryWaitUpperBound = 0

	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Finish(id, fmt.Errorf("something went wrong"))
	require.NoError(s.T(), err)

	for i := 0; i < MaxTaskRetries; i++ {
		err = s.queue.RequeueFailedTasks([]string{testTaskType})
		assert.NoError(s.T(), err)

		_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
		require.NoError(s.T(), err)

		err = s.queue.Finish(id, fmt.Errorf("something went wrong"))
		require.NoError(s.T(), err)
	}

	err = s.queue.RequeueFailedTasks([]string{testTaskType})
	assert.NoError(s.T(), err)

	info, err := s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.TaskStatusFailed, info.Status)
}

func (s *QueueSuite) TestHeartbeats() {
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	id, err = s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	// Test pending tasks have no heartbeats
	uuids := s.queue.Heartbeats(time.Millisecond)
	assert.Len(s.T(), uuids, 0)

	// Test running tasks have heartbeats and only tasks older than 10ms are found
	id, err = s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)
	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	time.Sleep(time.Millisecond * 10)

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	uuids = s.queue.Heartbeats(time.Millisecond * 10)
	assert.Len(s.T(), uuids, 2)
}

func (s *QueueSuite) TestIdFromToken() {
	_, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)

	info, err := s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	token, isRunning, err := s.queue.IdFromToken(info.Token)
	assert.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, token)
	assert.True(s.T(), isRunning)

	// Test no token found
	_, _, err = s.queue.IdFromToken(uuid.New())
	assert.ErrorIs(s.T(), err, ErrNotExist)
}

func (s *QueueSuite) TestListenForCanceledTask() {
	pgQueue, err := NewPgQueue(context.Background(), db.GetUrl())
	require.NoError(s.T(), err)
	defer pgQueue.Close()

	taskID := uuid.New()
	receivedID := make(chan uuid.UUID, 1)

	go func() {
		id, err := pgQueue.ListenForCanceledTask(context.Background())
		if err == nil {
			receivedID <- id
		}
	}()

	time.Sleep(time.Millisecond * 200)

	err = pgQueue.sendCancelNotification(context.Background(), taskID)
	assert.NoError(s.T(), err)
	time.Sleep(time.Millisecond * 100)

	assert.Equal(s.T(), taskID, <-receivedID)
}

func (s *QueueSuite) TestCancel() {
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Cancel(context.Background(), id)
	require.NoError(s.T(), err)

	info, err := s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.Nil(s.T(), info.Finished)
	assert.Equal(s.T(), config.TaskStatusCanceled, info.Status)
	require.NotNil(s.T(), info.Error)
	assert.Equal(s.T(), "task canceled", *info.Error)
}

func (s *QueueSuite) TestPriority() {
	task1 := testTask
	task1.Priority = 0
	task1ID, err := s.queue.Enqueue(&task1)
	require.NoError(s.T(), err)

	task2 := testTask
	task2.Priority = 1
	task2ID, err := s.queue.Enqueue(&task2)
	require.NoError(s.T(), err)

	task3 := testTask
	task3.Priority = 1
	task3ID, err := s.queue.Enqueue(&task3)
	require.NoError(s.T(), err)

	task4 := testTask
	task4.Priority = 0
	task4ID, err := s.queue.Enqueue(&task4)
	require.NoError(s.T(), err)

	info, err := s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), info.Id, task2ID) // task 2 is highest priority and queued before task 3

	info, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), info.Id, task3ID) // task 3 is highest priority, but queued after task 2

	info, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), info.Id, task1ID) // task 1 is lowest priority

	info, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), info.Id, task4ID) // task 4 is lowest priority and queued after task 1
}
