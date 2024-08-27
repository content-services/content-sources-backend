package client

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ClientSuite struct {
	suite.Suite
}

func TestClientSuite(t *testing.T) {
	suite.Run(t, &ClientSuite{})
}

func (s *ClientSuite) TestEnqueue() {
	mockQueue := queue.NewMockQueue(s.T())
	expectedUuid := uuid.New()
	task := queue.Task{
		Typename: "test",
	}
	mockQueue.On("Enqueue", &task).Return(expectedUuid, nil)

	tc := NewTaskClient(mockQueue)

	actualUuid, err := tc.Enqueue(task)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), expectedUuid, actualUuid)
}

func (s *ClientSuite) TestSendCancelNotification() {
	mockQueue := queue.NewMockQueue(s.T())
	expectedUuid1 := uuid.New()
	expectedUuid2 := uuid.New()
	cancellableTask := &models.TaskInfo{
		Typename: "snapshot",
	}
	uncancellableTask := &models.TaskInfo{
		Typename: "test",
	}

	// Test cancel succeeds for cancellable task type
	mockQueue.On("Status", expectedUuid1).Return(cancellableTask, nil)
	mockQueue.On("SendCancelNotification", context.Background(), expectedUuid1).Return(nil)
	tc := NewTaskClient(mockQueue)
	taskInfo, err := mockQueue.Status(expectedUuid1)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), taskInfo)
	err = tc.SendCancelNotification(context.Background(), expectedUuid1.String())
	assert.NoError(s.T(), err)

	// Test cancel errors for un-cancellable task type
	mockQueue.On("Status", expectedUuid2).Return(uncancellableTask, nil)
	taskInfo, err = mockQueue.Status(expectedUuid2)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), taskInfo)
	err = tc.SendCancelNotification(context.Background(), expectedUuid2.String())
	assert.Error(s.T(), err)
}
