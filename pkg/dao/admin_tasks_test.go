package dao

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	zest "github.com/content-services/zest/release/v3"
	"github.com/google/uuid"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type AdminTaskSuite struct {
	*DaoSuite
	dao              AdminTaskDao
	mockPulpClient   *pulp_client.MockPulpClient
	initialTaskCount int64
}

func (suite *AdminTaskSuite) SetupTest() {
	suite.DaoSuite.SetupTest()
	pulpClient := pulp_client.NewMockPulpClient(suite.T())
	suite.mockPulpClient = pulpClient
	suite.dao = GetAdminTaskDao(suite.tx, pulpClient)

	if suite.tx.Model(&models.TaskInfo{}).Count(&suite.initialTaskCount).Error != nil {
		suite.FailNow(suite.tx.Error.Error())
	}
}

func TestAdminTaskSuite(t *testing.T) {
	m := DaoSuite{}
	adminTaskSuite := AdminTaskSuite{DaoSuite: &m}
	suite.Run(t, &adminTaskSuite)
}

func (suite *AdminTaskSuite) TestFetch() {
	task, accountId := suite.createTask()
	t := suite.T()

	fetchedTask, err := suite.dao.Fetch(task.Id.String())
	assert.NoError(t, err)

	fetchedUUID, uuidErr := uuid.Parse(fetchedTask.UUID)
	assert.NoError(t, uuidErr)
	assert.Equal(t, task.Id, fetchedUUID)
	assert.Equal(t, task.OrgId, fetchedTask.OrgId)
	assert.Equal(t, accountId, fetchedTask.AccountId)
	assert.Equal(t, task.Typename, fetchedTask.Typename)
	assert.Equal(t, task.Status, fetchedTask.Status)
	assert.Equal(t, task.Queued.Format(time.RFC3339), fetchedTask.QueuedAt)
	assert.Equal(t, task.Started.Format(time.RFC3339), fetchedTask.StartedAt)
	assert.Equal(t, task.Finished.Format(time.RFC3339), fetchedTask.FinishedAt)
	assert.JSONEq(t, "{\"url\":\"https://example.com\"}", string(task.Payload))
	assert.Equal(t, *task.Error, fetchedTask.Error)
}

func (suite *AdminTaskSuite) TestFetchNotFound() {
	suite.createTask()
	t := suite.T()
	otherUUID := uuid.NewString()

	_, err := suite.dao.Fetch(otherUUID)
	assert.NotNil(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

// Occurs if repository/repository configuration associated with task is deleted
func (suite *AdminTaskSuite) TestFetchMissingAccountId() {
	t := suite.T()
	taskUUID := uuid.New()
	nonExistentRepo := uuid.New()

	createTaskErr := suite.tx.Create(models.TaskInfo{Id: taskUUID, RepositoryUUID: nonExistentRepo, Token: uuid.New()}).Error
	assert.NoError(t, createTaskErr)

	response, err := suite.dao.Fetch(taskUUID.String())
	assert.NoError(t, err)
	assert.Equal(t, "", response.AccountId)
}

func (suite *AdminTaskSuite) TestFetchSnapshotRepository() {
	t := suite.T()
	var initialPayload, err = json.Marshal(payloads.SnapshotPayload{
		SyncTaskHref:         pointy.String("/example-sync/"),
		PublicationTaskHref:  pointy.String("/example-publication/"),
		DistributionTaskHref: pointy.String("/example-distribution/"),
	})
	assert.NoError(t, err)

	task := models.TaskInfo{
		Id:       uuid.New(),
		Typename: payloads.Snapshot,
		Payload:  initialPayload,
		Token:    uuid.New(),
	}
	assert.NoError(t, suite.tx.Create(&task).Error)

	suite.mockPulpClient.On("GetTask", "/example-sync/").Return(zest.TaskResponse{Name: "example sync", LoggingCid: "1"}, nil)
	suite.mockPulpClient.On("GetTask", "/example-publication/").Return(zest.TaskResponse{Name: "example publication", LoggingCid: "2"}, nil)
	suite.mockPulpClient.On("GetTask", "/example-distribution/").Return(zest.TaskResponse{Name: "example distribution", LoggingCid: "3"}, nil)

	fetchedTask, err := suite.dao.Fetch(task.Id.String())
	assert.NoError(t, err)

	fetchedUUID, uuidErr := uuid.Parse(fetchedTask.UUID)
	assert.NoError(t, uuidErr)
	assert.Equal(t, task.Id, fetchedUUID)

	expectedPulpData := api.PulpResponse{
		Sync: &api.PulpTaskResponse{
			Name:       "example sync",
			LoggingCid: "1",
		},
		Publication: &api.PulpTaskResponse{
			Name:       "example publication",
			LoggingCid: "2",
		},
		Distribution: &api.PulpTaskResponse{
			Name:       "example distribution",
			LoggingCid: "3",
		},
	}

	assert.JSONEq(t, string(initialPayload), string(fetchedTask.Payload))
	assert.Equal(t, expectedPulpData, fetchedTask.Pulp)
}

func (suite *AdminTaskSuite) TestFetchSnapshotRepositoryPulpError() {
	t := suite.T()
	var initialPayload, err = json.Marshal(payloads.SnapshotPayload{
		SyncTaskHref:         pointy.String("/example-sync/"),
		PublicationTaskHref:  pointy.String("/example-publication/"),
		DistributionTaskHref: pointy.String("/example-distribution/"),
	})
	assert.NoError(t, err)

	task := models.TaskInfo{
		Id:       uuid.New(),
		Typename: payloads.Snapshot,
		Payload:  initialPayload,
		Token:    uuid.New(),
	}
	assert.NoError(t, suite.tx.Create(&task).Error)

	suite.mockPulpClient.On("GetTask", "/example-sync/").Return(zest.TaskResponse{}, errors.New("a pulp error"))

	_, fetchErr := suite.dao.Fetch(task.Id.String())
	assert.Error(t, fetchErr)
}

func (suite *AdminTaskSuite) TestList() {
	task, accountId := suite.createTask()
	t := suite.T()

	var total int64
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.AdminTaskFilterData{
		Status:    "",
		OrgId:     orgIDTest,
		AccountId: "",
	}
	var err error

	response, total, err := suite.dao.List(pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 1, len(response.Data))
	if len(response.Data) > 0 {
		fetchedTask := response.Data[0]
		fetchedUUID, uuidErr := uuid.Parse(fetchedTask.UUID)
		assert.NoError(t, uuidErr)
		assert.Equal(t, task.Id, fetchedUUID)
		assert.Equal(t, task.OrgId, fetchedTask.OrgId)
		assert.Equal(t, accountId, fetchedTask.AccountId)
		assert.Equal(t, task.Typename, fetchedTask.Typename)
		assert.Equal(t, task.Status, fetchedTask.Status)
		assert.Equal(t, task.Queued.Format(time.RFC3339), fetchedTask.QueuedAt)
		assert.Equal(t, task.Started.Format(time.RFC3339), fetchedTask.StartedAt)
		assert.Equal(t, task.Finished.Format(time.RFC3339), fetchedTask.FinishedAt)
		// Payload is omitted in List
		assert.Nil(t, fetchedTask.Payload)
		assert.Equal(t, *task.Error, fetchedTask.Error)
	}
}

func (suite *AdminTaskSuite) TestListAllTasks() {
	t := suite.T()

	seedErr := seeds.SeedTasks(suite.tx, 10, seeds.TaskSeedOptions{})
	assert.NoError(t, seedErr)

	_, daoTotal, err := suite.dao.List(api.PaginationData{}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	assert.Equal(t, suite.initialTaskCount+10, daoTotal)
}

func (suite *AdminTaskSuite) TestListMultipleOrgs() {
	t := suite.T()
	accountId := seeds.RandomAccountId()

	org1SeedOptions := seeds.TaskSeedOptions{
		AccountID: accountId,
		OrgID:     seeds.RandomOrgId(),
	}
	seed1Err := seeds.SeedTasks(suite.tx, 10, org1SeedOptions)
	assert.NoError(t, seed1Err)

	org2SeedOptions := seeds.TaskSeedOptions{
		AccountID: accountId,
		OrgID:     seeds.RandomOrgId(),
	}

	seed2Err := seeds.SeedTasks(suite.tx, 10, org2SeedOptions)
	assert.NoError(t, seed2Err)

	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
		SortBy: "org_id",
	}
	filterData := api.AdminTaskFilterData{
		AccountId: accountId,
	}
	response, total, err := suite.dao.List(pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(20), total)
	assert.NotEqual(t, response.Data[0].OrgId, response.Data[11].OrgId)
}

func (suite *AdminTaskSuite) TestListMultipleAccounts() {
	t := suite.T()
	orgId := seeds.RandomOrgId()

	account1SeedOptions := seeds.TaskSeedOptions{
		AccountID: seeds.RandomAccountId(),
		OrgID:     orgId,
	}
	seed1Err := seeds.SeedTasks(suite.tx, 10, account1SeedOptions)
	assert.NoError(t, seed1Err)

	account2SeedOptions := seeds.TaskSeedOptions{
		AccountID: seeds.RandomAccountId(),
		OrgID:     orgId,
	}

	seed2Err := seeds.SeedTasks(suite.tx, 10, account2SeedOptions)
	assert.NoError(t, seed2Err)

	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
		SortBy: "account_id",
	}
	filterData := api.AdminTaskFilterData{
		OrgId: orgId,
	}
	response, total, err := suite.dao.List(pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(20), total)
	assert.NotEqual(t, response.Data[0].AccountId, response.Data[11].AccountId)
}

func (suite *AdminTaskSuite) TestListNoRepositories() {
	suite.createTask()
	t := suite.T()
	otherOrgId := seeds.RandomOrgId()

	var total int64
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.AdminTaskFilterData{
		Status:    "",
		OrgId:     otherOrgId,
		AccountId: "",
	}
	var err error

	response, total, err := suite.dao.List(pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, 0, len(response.Data))
}

func (suite *AdminTaskSuite) TestListPageLimit() {
	var err error
	var total int64
	t := suite.T()
	orgID := seeds.RandomOrgId()

	err = seeds.SeedTasks(suite.tx, 20, seeds.TaskSeedOptions{
		OrgID: orgID,
	})
	assert.NoError(t, err)

	pageData := api.PaginationData{
		Limit:  10,
		Offset: 0,
	}
	filterData := api.AdminTaskFilterData{}

	response, total, err := suite.dao.List(pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, pageData.Limit, len(response.Data))
	assert.Equal(t, int64(20)+suite.initialTaskCount, total)
}

func (suite *AdminTaskSuite) TestListOffsetPage() {
	var err error
	var total int64
	t := suite.T()
	orgID := seeds.RandomOrgId()

	err = seeds.SeedTasks(suite.tx, 11, seeds.TaskSeedOptions{
		OrgID: orgID,
	})
	assert.NoError(t, err)

	pageData := api.PaginationData{
		Limit:  10,
		Offset: 0,
	}
	filterData := api.AdminTaskFilterData{
		OrgId: orgID,
	}

	response, total, err := suite.dao.List(pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, pageData.Limit, len(response.Data))
	assert.Equal(t, int64(11), total)

	nextPageData := api.PaginationData{
		Limit:  10,
		Offset: 10,
	}

	nextResponse, nextTotal, err := suite.dao.List(nextPageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(nextResponse.Data))
	assert.Equal(t, int64(11), nextTotal)
}

func (suite *AdminTaskSuite) TestListFilterStatus() {
	var err error
	var total int64
	t := suite.T()
	orgID := seeds.RandomOrgId()
	status := uuid.NewString()

	err = seeds.SeedTasks(suite.tx, 10, seeds.TaskSeedOptions{
		OrgID:  orgID,
		Status: status,
	})
	assert.NoError(t, err)
	err = seeds.SeedTasks(suite.tx, 20, seeds.TaskSeedOptions{
		OrgID:  orgID,
		Status: uuid.NewString(),
	})
	assert.NoError(t, err)

	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.AdminTaskFilterData{
		Status: status,
	}

	response, total, err := suite.dao.List(pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, int64(10), int64(len(response.Data)))
	assert.Equal(t, int64(10), total)
}

func (suite *AdminTaskSuite) TestFilterOrgID() {
	task, _ := suite.createTask()
	t := suite.T()
	otherOrgId := seeds.RandomOrgId()

	var total int64
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.AdminTaskFilterData{
		Status:    "",
		OrgId:     otherOrgId,
		AccountId: "",
	}
	var err error

	response, total, err := suite.dao.List(pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, 0, len(response.Data))

	filterData = api.AdminTaskFilterData{
		OrgId: task.OrgId,
	}
	response, total, err = suite.dao.List(pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 1, len(response.Data))
}

func (suite *AdminTaskSuite) TestFilterAccountID() {
	_, accountId := suite.createTask()
	t := suite.T()
	otherAccountId := seeds.RandomAccountId()

	var total int64
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.AdminTaskFilterData{
		Status:    "",
		OrgId:     "",
		AccountId: otherAccountId,
	}
	var err error

	response, total, err := suite.dao.List(pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, 0, len(response.Data))

	filterData = api.AdminTaskFilterData{
		AccountId: accountId,
	}
	response, total, err = suite.dao.List(pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 1, len(response.Data))
}

func (suite *AdminTaskSuite) TestSort() {
	t := suite.T()

	seedErr1 := seeds.SeedTasks(suite.tx, 20, seeds.TaskSeedOptions{
		AccountID: seeds.RandomAccountId(),
		OrgID:     seeds.RandomOrgId(),
	})
	assert.NoError(t, seedErr1)
	seedErr2 := seeds.SeedTasks(suite.tx, 20, seeds.TaskSeedOptions{
		AccountID: seeds.RandomAccountId(),
		OrgID:     seeds.RandomOrgId(),
	})
	assert.NoError(t, seedErr2)

	var response api.AdminTaskInfoCollectionResponse
	var err error

	response, _, err = suite.dao.List(api.PaginationData{Limit: 100, SortBy: "org_id:asc"}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	assert.LessOrEqual(t, response.Data[0].OrgId, response.Data[len(response.Data)-1].OrgId)

	response, _, err = suite.dao.List(api.PaginationData{Limit: 100, SortBy: "org_id:desc"}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, response.Data[0].OrgId, response.Data[len(response.Data)-1].OrgId)

	response, _, err = suite.dao.List(api.PaginationData{Limit: 100, SortBy: "typename:asc"}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	assert.LessOrEqual(t, response.Data[0].Typename, response.Data[len(response.Data)-1].Typename)

	response, _, err = suite.dao.List(api.PaginationData{Limit: 100, SortBy: "typename:desc"}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, response.Data[0].Typename, response.Data[len(response.Data)-1].Typename)

	var firstTime, lastTime time.Time

	response, _, err = suite.dao.List(api.PaginationData{Limit: 100, SortBy: "queued_at:asc"}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	firstTime, err = time.Parse(time.RFC3339, response.Data[0].QueuedAt)
	assert.NoError(t, err)
	lastTime, err = time.Parse(time.RFC3339, response.Data[len(response.Data)-1].QueuedAt)
	assert.NoError(t, err)
	assert.True(t, firstTime.Before(lastTime))

	response, _, err = suite.dao.List(api.PaginationData{Limit: 100, SortBy: "queued_at:desc"}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	firstTime, err = time.Parse(time.RFC3339, response.Data[0].QueuedAt)
	assert.NoError(t, err)
	lastTime, err = time.Parse(time.RFC3339, response.Data[len(response.Data)-1].QueuedAt)
	assert.NoError(t, err)
	assert.True(t, lastTime.Before(firstTime))

	response, _, err = suite.dao.List(api.PaginationData{Limit: 100, SortBy: "started_at:asc"}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	firstTime, err = time.Parse(time.RFC3339, response.Data[0].StartedAt)
	assert.NoError(t, err)
	lastTime, err = time.Parse(time.RFC3339, response.Data[len(response.Data)-1].StartedAt)
	assert.NoError(t, err)
	assert.True(t, firstTime.Before(lastTime))

	response, _, err = suite.dao.List(api.PaginationData{Limit: 100, SortBy: "started_at:desc"}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	firstTime, err = time.Parse(time.RFC3339, response.Data[0].QueuedAt)
	assert.NoError(t, err)
	lastTime, err = time.Parse(time.RFC3339, response.Data[len(response.Data)-1].StartedAt)
	assert.NoError(t, err)
	assert.True(t, lastTime.Before(firstTime))

	response, _, err = suite.dao.List(api.PaginationData{Limit: 100, SortBy: "finished_at:asc"}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	firstTime, err = time.Parse(time.RFC3339, response.Data[0].QueuedAt)
	assert.NoError(t, err)
	lastTime, err = time.Parse(time.RFC3339, response.Data[len(response.Data)-1].FinishedAt)
	assert.NoError(t, err)
	assert.True(t, firstTime.Before(lastTime))

	response, _, err = suite.dao.List(api.PaginationData{Limit: 100, SortBy: "finished_at:desc"}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	firstTime, err = time.Parse(time.RFC3339, response.Data[0].QueuedAt)
	assert.NoError(t, err)
	lastTime, err = time.Parse(time.RFC3339, response.Data[len(response.Data)-1].FinishedAt)
	assert.NoError(t, err)
	assert.True(t, lastTime.Before(firstTime))

	response, _, err = suite.dao.List(api.PaginationData{Limit: 100, SortBy: "status:asc"}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	assert.LessOrEqual(t, response.Data[0].Status, response.Data[len(response.Data)-1].Status)

	response, _, err = suite.dao.List(api.PaginationData{Limit: 100, SortBy: "status:desc"}, api.AdminTaskFilterData{})
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, response.Data[0].Status, response.Data[len(response.Data)-1].Status)
}

func (suite *AdminTaskSuite) createTask() (models.TaskInfo, string) {
	t := suite.T()

	var queued = time.Now()
	var started = time.Now().Add(time.Minute * 5)
	var finished = time.Now().Add(time.Minute * 10)
	var taskError = "test task error"
	var payload, err = json.Marshal(map[string]string{"url": "https://example.com"})
	assert.NoError(t, err)

	repo := models.Repository{
		URL: "https://www.example.com",
	}

	createRepoErr := suite.tx.Create(&repo).Error
	assert.NoError(t, createRepoErr)

	accountId := accountIdTest

	repoConfig := models.RepositoryConfiguration{
		RepositoryUUID: repo.UUID,
		AccountID:      accountId,
		Name:           fmt.Sprintf("%s - %s - %s", seeds.RandStringBytes(2), "TestRepo", seeds.RandStringBytes(10)),
		OrgID:          orgIDTest,
	}

	task := models.TaskInfo{
		Id:             uuid.New(),
		Typename:       "test task type",
		Payload:        payload,
		OrgId:          orgIDTest,
		RepositoryUUID: uuid.MustParse(repo.UUID),
		Dependencies:   make([]uuid.UUID, 0),
		Token:          uuid.New(),
		Queued:         &queued,
		Started:        &started,
		Finished:       &finished,
		Error:          &taskError,
		Status:         "test task status",
	}

	createTaskErr := suite.tx.Create(&task).Error
	assert.NoError(t, createTaskErr)
	createRepoConfigErr := suite.tx.Create(&repoConfig).Error
	assert.NoError(t, createRepoConfigErr)

	return task, accountId
}

func (suite *AdminTaskSuite) TestGetPulpData() {
	t := suite.T()

	mockPulpClient := pulp_client.NewMockPulpClient(t)
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

	task := models.TaskInfo{
		Typename: payloads.Snapshot,
		Payload:  jsonPayload,
	}

	pulpData, parseErr := getPulpData(task, mockPulpClient)
	assert.NoError(t, parseErr)

	expectedPulpData := api.PulpResponse{
		Sync: &api.PulpTaskResponse{
			Name:       "example sync",
			LoggingCid: "1",
		},
		Publication: &api.PulpTaskResponse{
			Name:       "example publication",
			LoggingCid: "2",
		},
		Distribution: &api.PulpTaskResponse{
			Name:       "example distribution",
			LoggingCid: "3",
		},
	}

	assert.Equal(t, expectedPulpData, pulpData)
}

func (suite *AdminTaskSuite) TestGetPulpDataIncomplete() {
	t := suite.T()

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	mockPulpClient.On("GetTask", "/example-sync/").Return(zest.TaskResponse{Name: "example sync", LoggingCid: "1"}, nil)

	payload := payloads.SnapshotPayload{
		SyncTaskHref: pointy.String("/example-sync/"),
	}
	jsonPayload, err := json.Marshal(payload)
	assert.NoError(t, err)

	task := models.TaskInfo{
		Typename: payloads.Snapshot,
		Payload:  jsonPayload,
	}

	pulpData, parseErr := getPulpData(task, mockPulpClient)
	assert.NoError(t, parseErr)

	expectedPulpData := api.PulpResponse{
		Sync: &api.PulpTaskResponse{
			Name:       "example sync",
			LoggingCid: "1",
		},
	}

	assert.Equal(t, expectedPulpData, pulpData)
}

func (suite *AdminTaskSuite) TestGetPulpDataPulpError() {
	t := suite.T()

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	mockPulpClient.On("GetTask", "/example-sync/").Return(zest.TaskResponse{}, errors.New("a pulp error"))

	payload := payloads.SnapshotPayload{
		SyncTaskHref: pointy.String("/example-sync/"),
	}
	jsonPayload, err := json.Marshal(payload)
	assert.NoError(t, err)

	task := models.TaskInfo{
		Typename: payloads.Snapshot,
		Payload:  jsonPayload,
	}

	_, parseErr := getPulpData(task, mockPulpClient)
	assert.Error(t, parseErr)
}

func (suite *AdminTaskSuite) TestGetPulpDataWrongType() {
	t := suite.T()

	mockPulpClient := pulp_client.NewMockPulpClient(t)

	payload := payloads.SnapshotPayload{
		SyncTaskHref: pointy.String("/example-sync/"),
	}
	jsonPayload, err := json.Marshal(payload)
	assert.NoError(t, err)

	task := models.TaskInfo{
		Typename: payloads.Introspect,
		Payload:  jsonPayload,
	}

	_, parseErr := getPulpData(task, mockPulpClient)
	assert.Error(t, parseErr)
}

func (suite *AdminTaskSuite) TestGetPulpDataInvalidPayload() {
	t := suite.T()

	mockPulpClient := pulp_client.NewMockPulpClient(t)

	jsonPayload, err := json.Marshal("not a valid payload")
	assert.NoError(t, err)

	task := models.TaskInfo{
		Typename: payloads.Snapshot,
		Payload:  jsonPayload,
	}

	_, parseErr := getPulpData(task, mockPulpClient)
	assert.Error(t, parseErr)
}
