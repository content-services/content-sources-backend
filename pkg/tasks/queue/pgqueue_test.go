package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type QueueSuite struct {
	suite.Suite
	queue PgQueue
	tx    pgx.Tx
}

func (s *QueueSuite) TearDownTest() {
	err := s.tx.Rollback(context.Background())
	if err != nil {
		require.NoError(s.T(), err)
	}
}

func (s *QueueSuite) SetupTest() {
	queue, err := NewPgQueue(db.GetUrl())
	require.NoError(s.T(), err)

	tx, err := queue.Pool.Begin(context.Background())
	require.NoError(s.T(), err)

	queue.Conn = tx
	s.tx = tx
	s.queue = queue

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
	Typename:       testTaskType,
	Payload:        testTaskPayload{Msg: "payload"},
	Dependencies:   nil,
	OrgId:          "12345",
	RepositoryUUID: uuid.NewString(),
}

func (s *QueueSuite) TestEnqueue() {
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	info, err := s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), StatusPending, info.Status)
	assert.NotNil(s.T(), info.Queued)
	assert.Nil(s.T(), info.Started)
	assert.Nil(s.T(), info.Finished)
	assert.Equal(s.T(), testTask.OrgId, info.OrgId)
	assert.Equal(s.T(), testTask.RepositoryUUID, info.RepositoryUUID.String())
}

func (s *QueueSuite) TestUpdatePayload() {
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	taskInfo, err := s.queue.Status(id)
	require.NoError(s.T(), err)

	_, err = s.queue.UpdatePayload(context.Background(), taskInfo, testTaskPayload{Msg: "Updated"})
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
	assert.Equal(s.T(), StatusRunning, info.Status)
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
	assert.Equal(s.T(), StatusCompleted, info.Status)

	// Test finishing task with error
	id, err = s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Finish(id, fmt.Errorf("something went wrong"))
	require.NoError(s.T(), err)

	info, err = s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.NotNil(s.T(), info.Finished)
	assert.Equal(s.T(), StatusFailed, info.Status)
	assert.Equal(s.T(), "something went wrong", *info.Error)
}

func (s *QueueSuite) TestRequeue() {
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	// Test cannot requeue pending task
	err = s.queue.Requeue(id)
	require.ErrorIs(s.T(), err, ErrNotRunning)

	// Test can requeue running task
	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Requeue(id)
	require.NoError(s.T(), err)

	info, err := s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), StatusPending, info.Status)

	// Test cannot requeue finished task
	_, err = s.queue.Dequeue(context.Background(), []string{testTaskType})
	require.NoError(s.T(), err)

	err = s.queue.Finish(id, nil)
	require.NoError(s.T(), err)

	err = s.queue.Requeue(id)
	assert.ErrorIs(s.T(), err, ErrNotRunning)
}

func (s *QueueSuite) TestCancel() {
	id, err := s.queue.Enqueue(&testTask)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, id)

	err = s.queue.Cancel(id)
	require.NoError(s.T(), err)

	info, err := s.queue.Status(id)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), StatusCanceled, info.Status)
	assert.Nil(s.T(), info.Finished)

	// Test cannot finish canceled task
	err = s.queue.Finish(id, nil)
	assert.ErrorIs(s.T(), err, ErrCanceled)
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

	token, err := s.queue.IdFromToken(info.Token)
	assert.NoError(s.T(), err)
	assert.NotEqual(s.T(), uuid.Nil, token)

	// Test no token found
	_, err = s.queue.IdFromToken(uuid.New())
	assert.ErrorIs(s.T(), err, ErrNotExist)
}
