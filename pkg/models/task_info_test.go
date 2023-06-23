package models

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	zest "github.com/content-services/zest/release/v3"
	"github.com/google/uuid"
	"github.com/openlyinc/pointy"
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
}

func (s *TaskInfoModelSuite) TestGetPulpData() {
	t := s.T()

	mockPulpClient := pulp_client.NewMockPulpClient(s.T())
	mockPulpClient.On("GetTask", "/example-sync/").Return(zest.TaskResponse{Name: "example sync", LoggingCid: "1"}, nil)
	mockPulpClient.On("GetTask", "/example-publication/").Return(zest.TaskResponse{Name: "example publication", LoggingCid: "2"}, nil)
	mockPulpClient.On("GetTask", "/example-distribution/").Return(zest.TaskResponse{Name: "example distribution", LoggingCid: "3"}, nil)

	payload := payloads.SnapshotPayload{
		SyncTaskHref:         pointy.String("/example-sync/"),
		PublicationTaskHref:  pointy.String("/example-publication/"),
		DistributionTaskHref: pointy.String("/example-distribution/"),
	}
	jsonPayload, err := json.Marshal(payload)
	assert.NoError(t, err)

	task := TaskInfo{
		Typename: payloads.Snapshot,
		Payload:  jsonPayload,
	}

	pulpData, parseErr := task.GetPulpData(mockPulpClient)
	assert.NoError(t, parseErr)

	expectedPulpData := api.PulpResponse{
		Sync: api.PulpTaskResponse{
			Name:       "example sync",
			LoggingCid: "1",
		},
		Publication: api.PulpTaskResponse{
			Name:       "example publication",
			LoggingCid: "2",
		},
		Distribution: api.PulpTaskResponse{
			Name:       "example distribution",
			LoggingCid: "3",
		},
	}

	assert.Equal(t, expectedPulpData, pulpData)
}

func (s *TaskInfoModelSuite) TestGetPulpDataIncomplete() {
	t := s.T()

	mockPulpClient := pulp_client.NewMockPulpClient(s.T())
	mockPulpClient.On("GetTask", "/example-sync/").Return(zest.TaskResponse{Name: "example sync", LoggingCid: "1"}, nil)

	payload := payloads.SnapshotPayload{
		SyncTaskHref: pointy.String("/example-sync/"),
	}
	jsonPayload, err := json.Marshal(payload)
	assert.NoError(t, err)

	task := TaskInfo{
		Typename: payloads.Snapshot,
		Payload:  jsonPayload,
	}

	pulpData, parseErr := task.GetPulpData(mockPulpClient)
	assert.NoError(t, parseErr)

	expectedPulpData := api.PulpResponse{
		Sync: api.PulpTaskResponse{
			Name:       "example sync",
			LoggingCid: "1",
		},
	}

	assert.Equal(t, expectedPulpData, pulpData)
}

func (s *TaskInfoModelSuite) TestGetPulpDataPulpError() {
	t := s.T()

	mockPulpClient := pulp_client.NewMockPulpClient(s.T())
	mockPulpClient.On("GetTask", "/example-sync/").Return(zest.TaskResponse{}, errors.New("a pulp error"))

	payload := payloads.SnapshotPayload{
		SyncTaskHref: pointy.String("/example-sync/"),
	}
	jsonPayload, err := json.Marshal(payload)
	assert.NoError(t, err)

	task := TaskInfo{
		Typename: payloads.Snapshot,
		Payload:  jsonPayload,
	}

	_, parseErr := task.GetPulpData(mockPulpClient)
	assert.Error(t, parseErr)
}

func (s *TaskInfoModelSuite) TestGetPulpDataWrongType() {
	t := s.T()

	mockPulpClient := pulp_client.NewMockPulpClient(s.T())

	payload := payloads.SnapshotPayload{
		SyncTaskHref: pointy.String("/example-sync/"),
	}
	jsonPayload, err := json.Marshal(payload)
	assert.NoError(t, err)

	task := TaskInfo{
		Typename: payloads.Introspect,
		Payload:  jsonPayload,
	}

	_, parseErr := task.GetPulpData(mockPulpClient)
	assert.Error(t, parseErr)
}

func (s *TaskInfoModelSuite) TestGetPulpDataInvalidPayload() {
	t := s.T()

	mockPulpClient := pulp_client.NewMockPulpClient(s.T())

	jsonPayload, err := json.Marshal("not a valid payload")
	assert.NoError(t, err)

	task := TaskInfo{
		Typename: payloads.Snapshot,
		Payload:  jsonPayload,
	}

	_, parseErr := task.GetPulpData(mockPulpClient)
	assert.Error(t, parseErr)
}
