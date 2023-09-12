package client

import (
	"context"
	"testing"

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
	expectedUuid := uuid.New()

	mockQueue.On("SendCancelNotification", context.Background(), expectedUuid).Return(nil)
	tc := NewTaskClient(mockQueue)
	err := tc.SendCancelNotification(context.Background(), expectedUuid.String())
	assert.NoError(s.T(), err)
}
