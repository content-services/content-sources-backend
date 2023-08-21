package dao

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TaskInfoSuite struct {
	*DaoSuite
}

func TestTaskInfoSuite(t *testing.T) {
	m := DaoSuite{}
	taskInfoSuite := TaskInfoSuite{DaoSuite: &m}
	suite.Run(t, &taskInfoSuite)
}

func (suite *TaskInfoSuite) TestFetch() {
	task, repoConfig := suite.createTask()
	t := suite.T()

	dao := GetTaskInfoDao(suite.tx)
	fetchedTask, err := dao.Fetch(task.OrgId, task.Id.String())
	assert.NoError(t, err)

	fetchedUUID, uuidErr := uuid.Parse(fetchedTask.UUID)
	assert.NoError(t, uuidErr)
	assert.Equal(t, task.Id, fetchedUUID)
	assert.Equal(t, task.OrgId, fetchedTask.OrgId)
	assert.Equal(t, task.Status, fetchedTask.Status)
	assert.Equal(t, task.Queued.Format(time.RFC3339), fetchedTask.CreatedAt)
	assert.Equal(t, task.Finished.Format(time.RFC3339), fetchedTask.EndedAt)
	assert.Equal(t, *task.Error, fetchedTask.Error)
	assert.Equal(t, task.Typename, fetchedTask.Typename)
	assert.Equal(t, repoConfig.UUID, fetchedTask.RepoConfigUUID)
	assert.Equal(t, repoConfig.Name, fetchedTask.RepoConfigName)

	//Seed task without repo config to test that it is also included in response
	timeQueued := time.Now().Add(time.Minute)
	timeFinished := time.Now().Add(time.Minute * 2)
	noRepoTask := models.TaskInfo{
		OrgId:    task.OrgId,
		Queued:   &timeQueued,
		Started:  &timeFinished,
		Finished: &timeFinished,
		Token:    uuid.New(),
	}
	err = suite.tx.Create(&noRepoTask).Error
	assert.NoError(t, err)

	fetchedTask, err = dao.Fetch(noRepoTask.OrgId, noRepoTask.Id.String())
	assert.NoError(t, err)

	fetchedUUID, uuidErr = uuid.Parse(fetchedTask.UUID)
	assert.NoError(t, uuidErr)
	assert.Equal(t, noRepoTask.Id, fetchedUUID)
	assert.Equal(t, noRepoTask.OrgId, fetchedTask.OrgId)
	assert.Equal(t, "", fetchedTask.RepoConfigName)
	assert.Equal(t, "", fetchedTask.RepoConfigUUID)
}

func (suite *TaskInfoSuite) TestFetchNotFound() {
	task, _ := suite.createTask()
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)

	var err error

	_, err = dao.Fetch("bad org id", task.Id.String())
	assert.NotNil(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	_, err = dao.Fetch(task.OrgId, uuid.NewString())
	assert.NotNil(t, err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	_, err = dao.Fetch(task.OrgId, "bad-uuid")
	assert.NotNil(t, err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

func (suite *TaskInfoSuite) TestList() {
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)

	task, repoConfig := suite.createTask()

	//Seed task without repo config to test that it is also included in response
	timeQueued := time.Now().Add(time.Minute)
	timeFinished := time.Now().Add(time.Minute * 2)
	noRepoTask := models.TaskInfo{
		OrgId:    task.OrgId,
		Queued:   &timeQueued,
		Started:  &timeFinished,
		Finished: &timeFinished,
		Token:    uuid.New(),
	}
	err := suite.tx.Create(&noRepoTask).Error
	assert.NoError(t, err)

	var total int64
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}

	response, total, err := dao.List(task.OrgId, pageData, api.TaskInfoFilterData{})
	assert.Nil(t, err)
	assert.Equal(t, int64(2), total)
	assert.Equal(t, 2, len(response.Data))
	if len(response.Data) > 0 {
		fetchedUUID, uuidErr := uuid.Parse(response.Data[1].UUID)
		assert.NoError(t, uuidErr)
		assert.Equal(t, task.Id, fetchedUUID)
		assert.Equal(t, task.OrgId, response.Data[1].OrgId)
		assert.Equal(t, task.Status, response.Data[1].Status)
		assert.Equal(t, task.Queued.Format(time.RFC3339), response.Data[1].CreatedAt)
		assert.Equal(t, task.Finished.Format(time.RFC3339), response.Data[1].EndedAt)
		assert.Equal(t, *task.Error, response.Data[1].Error)
		assert.Equal(t, task.Typename, response.Data[1].Typename)
		assert.Equal(t, repoConfig.UUID, response.Data[1].RepoConfigUUID)
		assert.Equal(t, repoConfig.Name, response.Data[1].RepoConfigName)
		assert.Equal(t, noRepoTask.OrgId, response.Data[0].OrgId)
		assert.Equal(t, "", response.Data[0].RepoConfigName)
		assert.Equal(t, "", response.Data[0].RepoConfigUUID)
	}
}

func (suite *TaskInfoSuite) TestListNoRepositories() {
	suite.createTask()
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)
	otherOrgId := seeds.RandomOrgId()
	var err error
	var total int64
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}

	response, total, err := dao.List(otherOrgId, pageData, api.TaskInfoFilterData{})
	assert.Nil(t, err)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, 0, len(response.Data))
}

func (suite *TaskInfoSuite) TestListPageLimit() {
	var err error
	var total int64
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)
	orgID := seeds.RandomOrgId()

	err = seeds.SeedTasks(suite.tx, 20, seeds.TaskSeedOptions{
		OrgID:     orgID,
		AccountID: accountIdTest,
	})
	assert.NoError(t, err)

	pageData := api.PaginationData{
		Limit:  10,
		Offset: 0,
	}

	var foundTasks []models.TaskInfo
	result := suite.tx.Where("org_id = ?", orgID).Find(&foundTasks).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(20), total)

	response, total, err := dao.List(orgID, pageData, api.TaskInfoFilterData{})
	assert.Nil(t, err)
	assert.Equal(t, pageData.Limit, len(response.Data))
	assert.Equal(t, int64(20), total)

	// Asserts that the first task is more recent than the last task
	firstItem, err := time.Parse(time.RFC3339, response.Data[0].CreatedAt)
	assert.NoError(t, err)
	lastItem, err := time.Parse(time.RFC3339, response.Data[len(response.Data)-1].CreatedAt)
	assert.NoError(t, err)
	assert.True(t, lastItem.Before(firstItem))
}

func (suite *TaskInfoSuite) TestListOffsetPage() {
	var err error
	var total int64
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)
	orgID := seeds.RandomOrgId()

	err = seeds.SeedTasks(suite.tx, 11, seeds.TaskSeedOptions{
		OrgID:     orgID,
		AccountID: accountIdTest,
	})
	assert.NoError(t, err)

	pageData := api.PaginationData{
		Limit:  10,
		Offset: 0,
	}

	var foundTasks []models.TaskInfo
	result := suite.tx.Where("org_id = ?", orgID).Find(&foundTasks).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(11), total)

	response, total, err := dao.List(orgID, pageData, api.TaskInfoFilterData{})
	assert.Nil(t, err)
	assert.Equal(t, pageData.Limit, len(response.Data))
	assert.Equal(t, int64(11), total)

	nextPageData := api.PaginationData{
		Limit:  10,
		Offset: 10,
	}

	nextResponse, nextTotal, err := dao.List(orgID, nextPageData, api.TaskInfoFilterData{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(nextResponse.Data))
	assert.Equal(t, int64(11), nextTotal)

	// Asserts that the first task is more recent than the last task
	firstItem, err := time.Parse(time.RFC3339, response.Data[0].CreatedAt)
	assert.NoError(t, err)
	lastItem, err := time.Parse(time.RFC3339, nextResponse.Data[len(nextResponse.Data)-1].CreatedAt)
	assert.NoError(t, err)
	assert.True(t, lastItem.Before(firstItem))
}

func (suite *TaskInfoSuite) TestListFilterStatus() {
	var err error
	var total int64
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)
	orgID := seeds.RandomOrgId()
	status := "completed"

	err = seeds.SeedTasks(suite.tx, 10, seeds.TaskSeedOptions{
		OrgID:     orgID,
		Status:    status,
		AccountID: accountIdTest,
	})
	assert.NoError(t, err)
	err = seeds.SeedTasks(suite.tx, 20, seeds.TaskSeedOptions{
		OrgID:     orgID,
		Status:    "other status",
		AccountID: accountIdTest,
	})
	assert.NoError(t, err)

	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.TaskInfoFilterData{
		Status: status,
	}

	var foundTasks []models.TaskInfo
	result := suite.tx.Where("org_id = ?", orgID).Find(&foundTasks).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(30), total)

	response, total, err := dao.List(orgID, pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 10, len(response.Data))
	assert.Equal(t, int64(10), total)
}

func (suite *TaskInfoSuite) TestListFilterType() {
	var err error
	var total int64
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)
	orgID := seeds.RandomOrgId()
	expectedType := "expected type"
	otherType := "other type"

	err = seeds.SeedTasks(suite.tx, 10, seeds.TaskSeedOptions{
		OrgID:     orgID,
		Typename:  expectedType,
		AccountID: accountIdTest,
	})
	assert.NoError(t, err)
	err = seeds.SeedTasks(suite.tx, 20, seeds.TaskSeedOptions{
		OrgID:     orgID,
		Typename:  otherType,
		AccountID: accountIdTest,
	})
	assert.NoError(t, err)

	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.TaskInfoFilterData{
		Typename: expectedType,
	}

	var foundTasks []models.TaskInfo
	result := suite.tx.Where("org_id = ?", orgID).Find(&foundTasks).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(30), total)

	response, total, err := dao.List(orgID, pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 10, len(response.Data))
	assert.Equal(t, int64(10), total)
}

func (suite *TaskInfoSuite) TestListFilterRepoConfigUUID() {
	var err error
	var total int64
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)

	// Create models for expected repo config
	repo := models.Repository{
		URL: "http://expected.example.com",
	}
	err = suite.tx.Create(&repo).Error
	assert.NoError(t, err)
	expectedRepoConfig := models.RepositoryConfiguration{
		Name:           "expectedRepoConfig",
		OrgID:          orgIDTest,
		RepositoryUUID: repo.UUID,
	}
	err = suite.tx.Create(&expectedRepoConfig).Error
	assert.NoError(t, err)
	err = seeds.SeedTasks(suite.tx, 1, seeds.TaskSeedOptions{RepoConfigUUID: expectedRepoConfig.UUID})
	assert.NoError(t, err)

	// Create models for other repo config
	repo = models.Repository{
		URL: "http://other.example.com",
	}
	err = suite.tx.Create(&repo).Error
	assert.NoError(t, err)
	otherRepoConfig := models.RepositoryConfiguration{
		Name:           "otherRepoConfig",
		OrgID:          orgIDTest,
		RepositoryUUID: repo.UUID,
	}
	err = suite.tx.Create(&otherRepoConfig).Error
	assert.NoError(t, err)
	err = seeds.SeedTasks(suite.tx, 1, seeds.TaskSeedOptions{RepoConfigUUID: otherRepoConfig.UUID})
	assert.NoError(t, err)

	// Test list
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.TaskInfoFilterData{
		RepoConfigUUID: expectedRepoConfig.UUID,
	}

	var foundTasks []models.TaskInfo
	result := suite.tx.Where("org_id = ?", orgIDTest).Find(&foundTasks).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(2), total)

	response, total, err := dao.List(orgIDTest, pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, int64(1), total)
}

func (suite *TaskInfoSuite) TestIsSnapshotInProgress() {
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)
	repoUUID := uuid.New()
	orgID := seeds.RandomOrgId()

	notRunningSnap := models.TaskInfo{
		Typename:       "introspect",
		Status:         "running",
		RepositoryUUID: repoUUID,
		Token:          uuid.New(),
		Id:             uuid.New(),
		OrgId:          orgID,
	}
	createErr := suite.tx.Create(notRunningSnap).Error
	require.NoError(t, createErr)

	notRunningSnap = models.TaskInfo{
		Typename:       "snapshot",
		Status:         "failed",
		RepositoryUUID: repoUUID,
		Token:          uuid.New(),
		Id:             uuid.New(),
		OrgId:          orgID,
	}
	createErr = suite.tx.Create(notRunningSnap).Error
	require.NoError(t, createErr)

	val, err := dao.IsSnapshotInProgress(orgID, repoUUID.String())
	assert.NoError(t, err)
	assert.False(t, val)

	runningSnap := models.TaskInfo{
		Typename:       "snapshot",
		Status:         "running",
		RepositoryUUID: repoUUID,
		Token:          uuid.New(),
		Id:             uuid.New(),
		OrgId:          orgID,
	}
	createErr = suite.tx.Create(runningSnap).Error
	require.NoError(t, createErr)

	val, err = dao.IsSnapshotInProgress(orgID, repoUUID.String())
	assert.NoError(t, err)
	assert.True(t, val)

	val, err = dao.IsSnapshotInProgress("bad org ID", repoUUID.String())
	assert.NoError(t, err)
	assert.False(t, val)
}

func (suite *TaskInfoSuite) createTask() (models.TaskInfo, models.RepositoryConfiguration) {
	t := suite.T()
	var queued = time.Now()
	var started = time.Now().Add(time.Minute * 5)
	var finished = time.Now().Add(time.Minute * 10)
	var taskError = "test task error"
	var payload, err = json.Marshal(map[string]string{"url": "https://example.com"})
	assert.NoError(t, err)

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgIDTest})
	assert.NoError(t, err)

	rc := models.RepositoryConfiguration{}
	err = suite.tx.Where("org_id = ?", orgIDTest).First(&rc).Error
	assert.NoError(t, err)

	repoUUID, err := uuid.Parse(rc.RepositoryUUID)
	assert.NoError(t, err)

	var task = models.TaskInfo{
		Id:             uuid.New(),
		Typename:       "test task type " + time.Now().String(),
		Payload:        payload,
		OrgId:          orgIDTest,
		RepositoryUUID: repoUUID,
		Dependencies:   make([]uuid.UUID, 0),
		Token:          uuid.New(),
		Queued:         &queued,
		Started:        &started,
		Finished:       &finished,
		Error:          &taskError,
		Status:         "test task status",
	}

	createErr := suite.tx.Create(&task).Error
	assert.NoError(t, createErr)
	return task, rc
}
