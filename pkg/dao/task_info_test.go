package dao

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/utils"
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
	fetchedTask, err := dao.Fetch(context.Background(), task.OrgId, task.Id.String())
	assert.NoError(t, err)

	fetchedUUID, uuidErr := uuid.Parse(fetchedTask.UUID)
	assert.NoError(t, uuidErr)
	assert.Equal(t, task.Id, fetchedUUID)
	assert.Equal(t, task.OrgId, fetchedTask.OrgId)
	assert.Equal(t, task.Status, fetchedTask.Status)
	assert.Equal(t, task.Queued.UTC().Format(time.RFC3339), fetchedTask.CreatedAt)
	assert.Equal(t, task.Finished.UTC().Format(time.RFC3339), fetchedTask.EndedAt)
	assert.Equal(t, *task.Error, fetchedTask.Error)
	assert.Equal(t, task.Typename, fetchedTask.Typename)
	assert.Equal(t, config.ObjectTypeRepository, fetchedTask.ObjectType)
	assert.Equal(t, repoConfig.UUID, fetchedTask.ObjectUUID)
	assert.Equal(t, repoConfig.Name, fetchedTask.ObjectName)

	// Seed task without repo config to test that it is also included in response
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

	fetchedTask, err = dao.Fetch(context.Background(), noRepoTask.OrgId, noRepoTask.Id.String())
	assert.NoError(t, err)

	fetchedUUID, uuidErr = uuid.Parse(fetchedTask.UUID)
	assert.NoError(t, uuidErr)
	assert.Equal(t, noRepoTask.Id, fetchedUUID)
	assert.Equal(t, noRepoTask.OrgId, fetchedTask.OrgId)
	assert.Equal(t, "", fetchedTask.ObjectName)
	assert.Equal(t, "", fetchedTask.ObjectUUID)

	templateTask, template := suite.createTemplateTask(task.OrgId)
	fetchedTask, err = dao.Fetch(context.Background(), templateTask.OrgId, templateTask.Id.String())
	assert.NoError(t, err)
	assert.Equal(t, templateTask.Id.String(), fetchedTask.UUID)
	assert.Equal(t, templateTask.OrgId, fetchedTask.OrgId)
	assert.Equal(t, config.ObjectTypeTemplate, fetchedTask.ObjectType)
	assert.Equal(t, template.UUID, fetchedTask.ObjectUUID)
	assert.Equal(t, template.Name, fetchedTask.ObjectName)
}

func (suite *TaskInfoSuite) TestFetchRedHat() {
	task, repoConfig := suite.createRedHatTask()
	t := suite.T()

	dao := GetTaskInfoDao(suite.tx)
	fetchedTask, err := dao.Fetch(context.Background(), task.OrgId, task.Id.String())
	assert.NoError(t, err)

	fetchedUUID, uuidErr := uuid.Parse(fetchedTask.UUID)
	assert.NoError(t, uuidErr)
	assert.Equal(t, task.Id, fetchedUUID)

	fetchedTask, err = dao.Fetch(context.Background(), "non red hat org", task.Id.String())
	assert.NoError(t, err)
	assert.Equal(t, config.RedHatOrg, fetchedTask.OrgId)
	assert.Equal(t, repoConfig.Name, fetchedTask.ObjectName)
	assert.Equal(t, repoConfig.UUID, fetchedTask.ObjectUUID)
}

func (suite *TaskInfoSuite) TestFetchWithOrgs() {
	task, repoConfig := suite.createTask()
	otherOrg := "oohgabooga"
	t := suite.T()

	repoConfig2 := models.RepositoryConfiguration{Name: "Another repo", OrgID: otherOrg, RepositoryUUID: repoConfig.RepositoryUUID}
	err := suite.tx.Create(&repoConfig2).Error
	require.NoError(suite.T(), err)

	task2 := suite.newTask()
	task2.OrgId = otherOrg
	task2.ObjectUUID = UuidifyString(repoConfig.RepositoryUUID)
	task2.ObjectType = utils.Ptr(config.ObjectTypeRepository)
	err = suite.tx.Create(&task2).Error
	require.NoError(suite.T(), err)

	dao := GetTaskInfoDao(suite.tx)
	fetchedTask, err := dao.Fetch(context.Background(), task.OrgId, task.Id.String())
	assert.NoError(t, err)
	assert.Equal(t, repoConfig.UUID, fetchedTask.ObjectUUID)
	assert.Equal(t, repoConfig.Name, fetchedTask.ObjectName)

	fetchedTask, err = dao.Fetch(context.Background(), otherOrg, task2.Id.String())
	assert.NoError(t, err)

	assert.Equal(t, repoConfig2.UUID, fetchedTask.ObjectUUID)
	assert.Equal(t, repoConfig2.Name, fetchedTask.ObjectName)
}
func (suite *TaskInfoSuite) TestFetchNotFound() {
	task, _ := suite.createTask()
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)

	var err error

	_, err = dao.Fetch(context.Background(), "bad org id", task.Id.String())
	assert.NotNil(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	_, err = dao.Fetch(context.Background(), task.OrgId, uuid.NewString())
	assert.NotNil(t, err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	_, err = dao.Fetch(context.Background(), task.OrgId, "bad-uuid")
	assert.NotNil(t, err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

func (suite *TaskInfoSuite) TestList() {
	var err error
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)

	templateTask, template := suite.createTemplateTask(orgIDTest)

	var existingRhRepoCount int64
	err = suite.tx.Model(&models.TaskInfo{}).Where("org_id = ?", config.RedHatOrg).Count(&existingRhRepoCount).Error
	assert.NoError(suite.T(), err)

	rhTask, rhRepoConfig := suite.createRedHatTask()
	task, repoConfig := suite.createTask()

	// Seed task without repo config to test that it is also included in response
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

	var total int64
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}

	response, total, err := dao.List(context.Background(), task.OrgId, pageData, api.TaskInfoFilterData{})
	assert.Nil(t, err)
	assert.Equal(t, int64(4)+existingRhRepoCount, total)
	assert.Equal(t, 4+int(existingRhRepoCount), len(response.Data))

	fetchedUUID, uuidErr := uuid.Parse(response.Data[1].UUID)
	assert.NoError(t, uuidErr)
	assert.Equal(t, task.Id, fetchedUUID)
	assert.Equal(t, task.OrgId, response.Data[1].OrgId)
	assert.Equal(t, task.Status, response.Data[1].Status)
	assert.Equal(t, task.Queued.UTC().Format(time.RFC3339), response.Data[1].CreatedAt)
	assert.Equal(t, task.Finished.UTC().Format(time.RFC3339), response.Data[1].EndedAt)
	assert.Equal(t, *task.Error, response.Data[1].Error)
	assert.Equal(t, task.Typename, response.Data[1].Typename)
	assert.Equal(t, repoConfig.UUID, response.Data[1].ObjectUUID)
	assert.Equal(t, repoConfig.Name, response.Data[1].ObjectName)
	assert.Equal(t, noRepoTask.OrgId, response.Data[0].OrgId)
	assert.Equal(t, "", response.Data[0].ObjectName)
	assert.Equal(t, "", response.Data[0].ObjectUUID)

	// list tasks returns newest first, so RH repo should be second to last
	rhUUID, uuidErr := uuid.Parse(response.Data[2].UUID)
	assert.NoError(t, uuidErr)

	assert.Equal(t, rhTask.Id, rhUUID)
	assert.Equal(t, response.Data[2].ObjectUUID, rhRepoConfig.UUID)

	// template task
	assert.Equal(t, templateTask.Id.String(), response.Data[3].UUID)
	assert.Equal(t, templateTask.OrgId, response.Data[3].OrgId)
	assert.Equal(t, config.ObjectTypeTemplate, response.Data[3].ObjectType)
	assert.Equal(t, template.UUID, response.Data[3].ObjectUUID)
	assert.Equal(t, template.Name, response.Data[3].ObjectName)
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

	response, total, err := dao.List(context.Background(), otherOrgId, pageData, api.TaskInfoFilterData{ExcludeRedHatOrg: true})
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

	_, err = seeds.SeedTasks(suite.tx, 20, seeds.TaskSeedOptions{
		OrgID:     orgID,
		AccountID: accountIdTest,
	})
	assert.NoError(t, err)

	pageData := api.PaginationData{
		Limit:  10,
		Offset: 0,
	}

	var foundTasks []models.TaskInfo
	result := suite.tx.Where("org_id = ?", orgID)
	result.Model(&foundTasks).Count(&total)
	result.Find(&foundTasks)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(20), total)

	response, total, err := dao.List(context.Background(), orgID, pageData, api.TaskInfoFilterData{ExcludeRedHatOrg: true})
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

	_, err = seeds.SeedTasks(suite.tx, 11, seeds.TaskSeedOptions{
		OrgID:     orgID,
		AccountID: accountIdTest,
	})
	assert.NoError(t, err)

	pageData := api.PaginationData{
		Limit:  10,
		Offset: 0,
	}

	var foundTasks []models.TaskInfo
	result := suite.tx.Where("org_id = ?", orgID)
	result.Model(&foundTasks).Count(&total)
	result.Find(&foundTasks)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(11), total)

	response, total, err := dao.List(context.Background(), orgID, pageData, api.TaskInfoFilterData{ExcludeRedHatOrg: true})
	assert.Nil(t, err)
	assert.Equal(t, pageData.Limit, len(response.Data))
	assert.Equal(t, int64(11), total)

	nextPageData := api.PaginationData{
		Limit:  10,
		Offset: 10,
	}

	nextResponse, nextTotal, err := dao.List(context.Background(), orgID, nextPageData, api.TaskInfoFilterData{ExcludeRedHatOrg: true})
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

	_, err = seeds.SeedTasks(suite.tx, 10, seeds.TaskSeedOptions{
		OrgID:     orgID,
		Status:    status,
		AccountID: accountIdTest,
	})
	assert.NoError(t, err)
	_, err = seeds.SeedTasks(suite.tx, 20, seeds.TaskSeedOptions{
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
		Status:           status,
		ExcludeRedHatOrg: true,
	}

	var foundTasks []models.TaskInfo
	result := suite.tx.Where("org_id = ?", orgID)
	result.Model(&foundTasks).Count(&total)
	result.Find(&foundTasks)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(30), total)

	response, total, err := dao.List(context.Background(), orgID, pageData, filterData)
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

	_, err = seeds.SeedTasks(suite.tx, 10, seeds.TaskSeedOptions{
		OrgID:     orgID,
		Typename:  expectedType,
		AccountID: accountIdTest,
	})
	assert.NoError(t, err)
	_, err = seeds.SeedTasks(suite.tx, 20, seeds.TaskSeedOptions{
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
	result := suite.tx.Where("org_id = ?", orgID)
	result.Model(&foundTasks).Count(&total)
	result.Find(&foundTasks)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(30), total)

	response, total, err := dao.List(context.Background(), orgID, pageData, filterData)
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
	_, err = seeds.SeedTasks(suite.tx, 1, seeds.TaskSeedOptions{RepoConfigUUID: expectedRepoConfig.UUID})
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
	_, err = seeds.SeedTasks(suite.tx, 1, seeds.TaskSeedOptions{RepoConfigUUID: otherRepoConfig.UUID})
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
	result := suite.tx.Where("org_id = ?", orgIDTest)
	result.Model(&foundTasks).Count(&total)
	result.Find(&foundTasks)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(2), total)

	response, total, err := dao.List(context.Background(), orgIDTest, pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, int64(1), total)
}

func (suite *TaskInfoSuite) TestListFilterTemplateUUID() {
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)
	orgID := seeds.RandomOrgId()

	_, _ = suite.createTemplateTask(orgID)
	task, template := suite.createTemplateTask(orgID)
	_, _ = suite.createTaskForOrg(orgID)

	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.TaskInfoFilterData{
		TemplateUUID: template.UUID,
	}

	response, total, err := dao.List(context.Background(), orgID, pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, int64(1), total)

	foundTask := response.Data[0]
	assert.Equal(t, task.Id.String(), foundTask.UUID)
	assert.Equal(t, config.ObjectTypeTemplate, foundTask.ObjectType)
	assert.Equal(t, template.UUID, foundTask.ObjectUUID)
}

func (suite *TaskInfoSuite) NewTaskForCleanup(taskType string, finishedAt time.Time, status string, repoConfig api.RepositoryResponse) models.TaskInfo {
	task := suite.newTask()
	task.Typename = taskType
	task.Status = status
	task.ObjectUUID, _ = uuid.Parse(repoConfig.RepositoryUUID)
	task.ObjectType = utils.Ptr(config.ObjectTypeRepository)
	task.OrgId = repoConfig.OrgID
	task.Finished = utils.Ptr(finishedAt)
	task.Started = utils.Ptr(finishedAt.Add(-1 * time.Hour))
	task.Queued = utils.Ptr(finishedAt.Add(-2 * time.Hour))
	return task
}

type CleanupTestCase struct {
	name      string
	task      models.TaskInfo
	beDeleted bool
}

func (suite *TaskInfoSuite) TestTaskCleanup() {
	results, err := seeds.SeedRepositoryConfigurations(suite.tx, 2, seeds.SeedOptions{OrgID: orgIDTest})
	assert.NoError(suite.T(), err)

	mockPulpClient := pulp_client.NewMockPulpClient(suite.T())
	repoConfigDao := GetRepositoryConfigDao(suite.tx, mockPulpClient)

	repoToKeep, err := repoConfigDao.Fetch(context.Background(), orgIDTest, results[0].UUID)
	assert.NoError(suite.T(), err)
	repoToDel, err := repoConfigDao.Fetch(context.Background(), orgIDTest, results[1].UUID)
	assert.NoError(suite.T(), err)

	cases := []CleanupTestCase{
		{
			name: "oldIntrospect",
			task: suite.NewTaskForCleanup(config.IntrospectTask, time.Now().Add(-32*24*time.Hour),
				config.TaskStatusCompleted, repoToKeep),
			beDeleted: true,
		},
		{
			name: "newIntrospect",
			task: suite.NewTaskForCleanup(config.IntrospectTask, time.Now().Add(-1*24*time.Hour),
				config.TaskStatusCompleted, repoToKeep),
			beDeleted: false,
		},
		{
			name: "oldFailedIntrospect",
			task: suite.NewTaskForCleanup(config.IntrospectTask, time.Now().Add(-32*24*time.Hour),
				config.TaskStatusFailed, repoToKeep),
			beDeleted: true,
		},
		{
			name: "orphanSnapshot",
			task: suite.NewTaskForCleanup(config.RepositorySnapshotTask, time.Now().Add(-1*24*time.Hour),
				config.TaskStatusCompleted, repoToDel),
			beDeleted: true,
		},
		{
			name: "nonOrphanSnapshot",
			task: suite.NewTaskForCleanup(config.RepositorySnapshotTask, time.Now().Add(-1*24*time.Hour),
				config.TaskStatusCompleted, repoToKeep),
			beDeleted: false,
		},
		{
			name: "oldDeleteRepo",
			task: suite.NewTaskForCleanup(config.DeleteRepositorySnapshotsTask, time.Now().Add(-11*24*time.Hour),
				config.TaskStatusCompleted, repoToKeep),
			beDeleted: true,
		},
		{
			name: "oldFailedDeleteRepo",
			task: suite.NewTaskForCleanup(config.DeleteRepositorySnapshotsTask, time.Now().Add(-32*24*time.Hour),
				config.TaskStatusFailed, repoToKeep),
			beDeleted: false,
		},
		{
			name: "newDeleteRepo",
			task: suite.NewTaskForCleanup(config.DeleteRepositorySnapshotsTask, time.Now().Add(-1*24*time.Hour),
				config.TaskStatusCompleted, repoToKeep),
			beDeleted: false,
		},
		{
			name: "oldDeleteTemplate",
			task: suite.NewTaskForCleanup(config.DeleteTemplatesTask, time.Now().Add(-11*24*time.Hour),
				config.TaskStatusCompleted, repoToKeep),
			beDeleted: true,
		},
		{
			name: "oldFailedDeleteTemplate",
			task: suite.NewTaskForCleanup(config.DeleteTemplatesTask, time.Now().Add(-32*24*time.Hour),
				config.TaskStatusFailed, repoToKeep),
			beDeleted: false,
		},
		{
			name: "newDeleteTemplate",
			task: suite.NewTaskForCleanup(config.DeleteTemplatesTask, time.Now().Add(-1*24*time.Hour),
				config.TaskStatusCompleted, repoToKeep),
			beDeleted: false,
		},
		{
			name: "oldUpdateTemplateContent",
			task: suite.NewTaskForCleanup(config.UpdateTemplateContentTask, time.Now().Add(-32*24*time.Hour),
				config.TaskStatusCompleted, repoToKeep),
			beDeleted: true,
		},
		{
			name: "newUpdateTemplateContent",
			task: suite.NewTaskForCleanup(config.UpdateTemplateContentTask, time.Now().Add(-1*24*time.Hour),
				config.TaskStatusCompleted, repoToKeep),
			beDeleted: false,
		},
		{
			name: "oldUpdateLatestSnapshot",
			task: suite.NewTaskForCleanup(config.UpdateLatestSnapshotTask, time.Now().Add(-32*24*time.Hour),
				config.TaskStatusCompleted, repoToKeep),
			beDeleted: true,
		},
		{
			name: "newUpdateLatestSnapshot",
			task: suite.NewTaskForCleanup(config.UpdateLatestSnapshotTask, time.Now().Add(-1*24*time.Hour),
				config.TaskStatusCompleted, repoToKeep),
			beDeleted: false,
		},
		{
			name: "oldCanceledTask",
			task: suite.NewTaskForCleanup(config.UpdateLatestSnapshotTask, time.Now().Add(-32*24*time.Hour),
				config.TaskStatusCanceled, repoToKeep),
			beDeleted: true,
		},
		{
			name: "newCanceledTask",
			task: suite.NewTaskForCleanup(config.UpdateLatestSnapshotTask, time.Now().Add(-1*24*time.Hour),
				config.TaskStatusCanceled, repoToKeep),
			beDeleted: false,
		},
	}
	for _, testCase := range cases {
		createErr := suite.tx.Create(&testCase.task).Error
		assert.NoError(suite.T(), createErr, "Couldn't create %v", testCase.name)
	}
	err = repoConfigDao.Delete(context.Background(), repoToDel.OrgID, repoToDel.UUID)
	assert.NoError(suite.T(), err)

	dao := GetTaskInfoDao(suite.tx)
	err = dao.Cleanup(context.Background())
	assert.NoError(suite.T(), err)

	for _, testCase := range cases {
		found := models.TaskInfo{}
		result := suite.tx.First(&found, testCase.task.Id)
		if testCase.beDeleted {
			assert.Equal(suite.T(), int64(0), result.RowsAffected, "Task %v expected to be deleted but wasn't", testCase.name)
		} else {
			assert.Equal(suite.T(), int64(1), result.RowsAffected, "Task %v expected to be present but wasn't", testCase.name)
		}
	}
}

func (suite *TaskInfoSuite) TestFetchActiveTasks() {
	t := suite.T()
	dao := GetTaskInfoDao(suite.tx)
	repoUUID := uuid.New()
	orgID := seeds.RandomOrgId()

	notCompletedTask := models.TaskInfo{
		Typename:   config.IntrospectTask,
		Status:     config.TaskStatusRunning,
		ObjectUUID: repoUUID,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		Token:      uuid.New(),
		Id:         uuid.New(),
		OrgId:      orgID,
	}
	createErr := suite.tx.Create(&notCompletedTask).Error
	require.NoError(t, createErr)

	notCompletedTask = models.TaskInfo{
		Typename:      config.RepositorySnapshotTask,
		Status:        config.TaskStatusFailed,
		ObjectUUID:    repoUUID,
		ObjectType:    utils.Ptr(config.ObjectTypeRepository),
		Token:         uuid.New(),
		Id:            uuid.New(),
		OrgId:         orgID,
		NextRetryTime: utils.Ptr(time.Now()),
	}
	createErr = suite.tx.Create(&notCompletedTask).Error
	require.NoError(t, createErr)

	notCompletedTask = models.TaskInfo{
		Typename:   config.UpdateRepositoryTask,
		Status:     config.TaskStatusPending,
		ObjectUUID: repoUUID,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		Token:      uuid.New(),
		Id:         uuid.New(),
		OrgId:      orgID,
	}
	createErr = suite.tx.Create(&notCompletedTask).Error
	require.NoError(t, createErr)

	completedSnap := models.TaskInfo{
		Typename:   config.UpdateTemplateContentTask,
		Status:     config.TaskStatusCompleted,
		ObjectUUID: repoUUID,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		Token:      uuid.New(),
		Id:         uuid.New(),
		OrgId:      orgID,
	}
	createErr = suite.tx.Create(&completedSnap).Error
	require.NoError(t, createErr)

	// Test fetches all tasks except completed task
	taskTypes := []string{config.IntrospectTask, config.RepositorySnapshotTask, config.UpdateRepositoryTask, config.UpdateTemplateContentTask}
	val, err := dao.FetchActiveTasks(context.Background(), orgID, repoUUID.String(), taskTypes...)
	assert.NoError(t, err)
	assert.Len(t, val, 3)

	// Test bad org ID
	val, err = dao.FetchActiveTasks(context.Background(), "bad org ID", repoUUID.String(), taskTypes...)
	assert.NoError(t, err)
	assert.Empty(t, val)
}

func (suite *TaskInfoSuite) createTask() (models.TaskInfo, models.RepositoryConfiguration) {
	return suite.createTaskForOrg(orgIDTest)
}

func (suite *TaskInfoSuite) createRedHatTask() (models.TaskInfo, models.RepositoryConfiguration) {
	return suite.createTaskForOrg(config.RedHatOrg)
}

func (suite *TaskInfoSuite) createTaskForOrg(orgId string) (models.TaskInfo, models.RepositoryConfiguration) {
	t := suite.T()
	_, err := seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgId})
	assert.NoError(t, err)

	rc := models.RepositoryConfiguration{}
	err = suite.tx.Where("org_id = ?", orgId).First(&rc).Error
	assert.NoError(t, err)

	repoUUID, err := uuid.Parse(rc.RepositoryUUID)
	assert.NoError(t, err)

	task := suite.newTask()
	task.ObjectUUID = repoUUID
	task.ObjectType = utils.Ptr(config.ObjectTypeRepository)
	task.OrgId = rc.OrgID

	createErr := suite.tx.Create(&task).Error
	assert.NoError(t, createErr)
	return task, rc
}

func (suite *TaskInfoSuite) newTask() models.TaskInfo {
	t := suite.T()
	var queued = time.Now()
	var started = time.Now().Add(time.Minute * 5)
	var finished = time.Now().Add(time.Minute * 10)
	var taskError = "test task error"
	var payload, err = json.Marshal(map[string]string{"url": "https://example.com"})
	assert.NoError(t, err)

	var task = models.TaskInfo{
		Id:           uuid.New(),
		Typename:     "test task type " + time.Now().String(),
		Payload:      payload,
		OrgId:        orgIDTest,
		Dependencies: make([]string, 0),
		Token:        uuid.New(),
		Queued:       &queued,
		Started:      &started,
		Finished:     &finished,
		Error:        &taskError,
		Status:       "test task status",
	}

	return task
}

func (suite *TaskInfoSuite) createTemplateTask(orgID string) (models.TaskInfo, api.TemplateResponse) {
	t := suite.T()

	// template
	const lookup string = "0123456789abcdefghijklmnopqrstuvwxyz"
	randomName := seeds.RandStringWithChars(10, lookup)
	timeNow := time.Now()
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr("template test" + randomName),
		Description:     utils.Ptr("template test description"),
		RepositoryUUIDS: []string{},
		Arch:            utils.Ptr(config.AARCH64),
		Version:         utils.Ptr(config.El8),
		Date:            (*api.EmptiableDate)(&timeNow),
		OrgID:           &orgID,
		UseLatest:       utils.Ptr(false),
	}

	tDao := templateDaoImpl{db: suite.tx}
	template, err := tDao.Create(context.Background(), reqTemplate)
	assert.NoError(t, err)

	// task
	task := suite.newTask()
	task.ObjectUUID = UuidifyString(template.UUID)
	task.ObjectType = utils.Ptr(config.ObjectTypeTemplate)
	task.OrgId = orgID

	createErr := suite.tx.Create(&task).Error
	assert.NoError(t, createErr)

	return task, template
}

func (suite *TaskInfoSuite) TestDeleteTask() {
	t := suite.T()
	tasks, err := seeds.SeedTasks(suite.tx, 1, seeds.TaskSeedOptions{Status: "finished"})
	require.NoError(t, err)
	taskUUID := tasks[0].Id.String()

	repos, err := seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: "someOrg", TaskID: taskUUID})
	assert.NoError(t, err)
	assert.NoError(t, suite.tx.Model(&models.TaskInfo{}).Where("id = ?", taskUUID).Delete(tasks[0]).Error)

	rc := models.RepositoryConfiguration{}
	assert.NoError(t, suite.tx.Where("uuid = ?", repos[0].UUID).First(&rc).Error)
	assert.Equal(t, repos[0].UUID, rc.UUID)
	assert.Empty(t, rc.LastSnapshotTaskUUID)
}
