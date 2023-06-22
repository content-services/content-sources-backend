package models

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/pulp_client"
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

func (s *TaskInfoModelSuite) TestParsePayloadSimple() {
	t := s.T()

	mockPulpClient := pulp_client.NewMockPulpClient(s.T())
	payload := IntrospectPayload{
		Url:   "http://www.example.com",
		Force: true,
	}
	jsonPayload, err := json.Marshal(payload)
	assert.NoError(t, err)

	task := TaskInfo{
		Typename: Introspect,
		Payload:  jsonPayload,
	}

	parsedPayload, parseErr := task.ParsePayload(mockPulpClient)
	assert.NoError(t, parseErr)
	assert.JSONEq(t, string(jsonPayload), string(parsedPayload))
}

func (s *TaskInfoModelSuite) TestParseSnapshotPayload() {
	t := s.T()

	mockPulpClient := pulp_client.NewMockPulpClient(s.T())
	mockPulpClient.On("GetTask", "/example-sync/").Return(zest.TaskResponse{Name: "example sync", LoggingCid: "1"}, nil)
	mockPulpClient.On("GetTask", "/example-publication/").Return(zest.TaskResponse{Name: "example publication", LoggingCid: "2"}, nil)
	mockPulpClient.On("GetTask", "/example-distribution/").Return(zest.TaskResponse{Name: "example distribution", LoggingCid: "3"}, nil)

	payload := SnapshotPayload{
		SyncTaskHref:         pointy.String("/example-sync/"),
		PublicationTaskHref:  pointy.String("/example-publication/"),
		DistributionTaskHref: pointy.String("/example-distribution/"),
	}
	jsonPayload, err := json.Marshal(payload)
	assert.NoError(t, err)

	task := TaskInfo{
		Typename: Snapshot,
		Payload:  jsonPayload,
	}

	parsedPayload, parseErr := task.ParsePayload(mockPulpClient)
	assert.NoError(t, parseErr)

	var expectedParsedPayload, _ = json.Marshal(map[string]map[string]string{
		"sync": {
			"name":        "example sync",
			"logging_cid": "1",
		},
		"publication": {
			"name":        "example publication",
			"logging_cid": "2",
		},
		"distribution": {
			"name":        "example distribution",
			"logging_cid": "3",
		},
	})

	assert.JSONEq(t, string(expectedParsedPayload), string(parsedPayload))
}

func (s *TaskInfoModelSuite) TestParseSnapshotPayloadIncomplete() {
	t := s.T()

	mockPulpClient := pulp_client.NewMockPulpClient(s.T())
	mockPulpClient.On("GetTask", "/example-sync/").Return(zest.TaskResponse{Name: "example sync", LoggingCid: "1"}, nil)

	payload := SnapshotPayload{
		SyncTaskHref: pointy.String("/example-sync/"),
	}
	jsonPayload, err := json.Marshal(payload)
	assert.NoError(t, err)

	task := TaskInfo{
		Typename: Snapshot,
		Payload:  jsonPayload,
	}

	parsedPayload, parseErr := task.ParsePayload(mockPulpClient)
	assert.NoError(t, parseErr)

	var expectedParsedPayload, _ = json.Marshal(map[string]map[string]string{
		"sync": {
			"name":        "example sync",
			"logging_cid": "1",
		},
	})

	assert.JSONEq(t, string(expectedParsedPayload), string(parsedPayload))
}

func (s *TaskInfoModelSuite) TestParseSnapshotPayloadPulpError() {
	t := s.T()

	mockPulpClient := pulp_client.NewMockPulpClient(s.T())
	mockPulpClient.On("GetTask", "/example-sync/").Return(zest.TaskResponse{}, errors.New("a pulp error"))

	payload := SnapshotPayload{
		SyncTaskHref: pointy.String("/example-sync/"),
	}
	jsonPayload, err := json.Marshal(payload)
	assert.NoError(t, err)

	task := TaskInfo{
		Typename: Snapshot,
		Payload:  jsonPayload,
	}

	_, parseErr := task.ParsePayload(mockPulpClient)
	assert.Error(t, parseErr)
}
