package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TaskInfoModelSuite struct {
	*ModelsSuite
}

func TestTaskInfoModelSuite(t *testing.T) {
	m := ModelsSuite{}
	taskInfoModelSuite := TaskInfoModelSuite{&m}
	suite.Run(t, &taskInfoModelSuite)
}

func (s *TaskInfoModelSuite) TestTaskInfo() {
	t := s.T()
	tx := s.tx

	// PostgreSQL has microsecond precision
	queued := time.Now().Truncate(time.Microsecond)
	started := time.Now().Add(10).Truncate(time.Microsecond)
	finished := time.Now().Add(100).Truncate(time.Microsecond)
	nextRetry := time.Now().Add(1000).Truncate(time.Microsecond)

	taskErr := "task error"
	payloadData := map[string]string{"url": "https://example.com"}
	payload, err := json.Marshal(payloadData)
	assert.NoError(t, err)

	task := TaskInfo{
		Id:             uuid.New(),
		Typename:       "example type",
		Payload:        payload,
		OrgId:          "example org id",
		RepositoryUUID: uuid.New(),
		Dependencies:   make([]uuid.UUID, 0),
		Token:          uuid.New(),
		Queued:         &queued,
		Started:        &started,
		Finished:       &finished,
		Error:          &taskErr,
		Status:         "task status",
		Retries:        1,
		NextRetryTime:  &nextRetry,
	}
	insert := tx.Create(&task)
	assert.NoError(t, insert.Error)

	readTaskInfo := TaskInfo{}
	result := tx.Where("id = ?", task.Id).First(&readTaskInfo)
	assert.NoError(t, result.Error)
	assert.Equal(t, task.Typename, readTaskInfo.Typename)

	var readTaskPayload map[string]string
	payloadErr := json.Unmarshal(readTaskInfo.Payload, &readTaskPayload)
	assert.NoError(t, payloadErr)
	assert.Equal(t, payloadData, readTaskPayload)

	assert.Equal(t, task.OrgId, readTaskInfo.OrgId)
	assert.Equal(t, task.RepositoryUUID, readTaskInfo.RepositoryUUID)
	assert.Len(t, task.Dependencies, 0)
	assert.Equal(t, task.Queued, readTaskInfo.Queued)
	assert.Equal(t, task.Token, readTaskInfo.Token)
	assert.Equal(t, task.Started, readTaskInfo.Started)
	assert.Equal(t, task.Finished, readTaskInfo.Finished)
	assert.Equal(t, task.Error, readTaskInfo.Error)
	assert.Equal(t, task.Status, readTaskInfo.Status)
	assert.Equal(t, task.Retries, readTaskInfo.Retries)
	assert.Equal(t, task.NextRetryTime, readTaskInfo.NextRetryTime)
}
