package dao

import (
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

const JoinSelectQuery = ` t.id,
       t.type,
       t.payload,
       t.org_id,
       t.repository_uuid,
       t.token,
       t.queued_at,
       t.started_at,
       t.finished_at,
       t.error,
       t.status,
       t.request_id,
       rc.uuid as rc_uuid,
       rc.name as rc_name
`

type taskInfoDaoImpl struct {
	db *gorm.DB
}

func GetTaskInfoDao(db *gorm.DB) TaskInfoDao {
	return taskInfoDaoImpl{
		db: db,
	}
}

func (t taskInfoDaoImpl) Fetch(orgID string, id string) (api.TaskInfoResponse, error) {
	taskInfo := models.TaskInfoRepositoryConfiguration{}
	taskInfoResponse := api.TaskInfoResponse{}

	result := t.db.Table(taskInfo.TableName()+" AS t ").
		Select(JoinSelectQuery).
		Joins("LEFT JOIN repositories r on t.repository_uuid = r.uuid").
		Joins("LEFT JOIN repository_configurations rc on r.uuid = rc.repository_uuid").
		Where("text(t.id) = ? AND t.org_id = ?", id, orgID).First(&taskInfo)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return taskInfoResponse, &ce.DaoError{NotFound: true, Message: "Could not find task with UUID " + id}
		} else {
			return taskInfoResponse, DBErrorToApi(result.Error)
		}
	}
	taskInfoModelToApiFields(&taskInfo, &taskInfoResponse)
	return taskInfoResponse, nil
}

func (t taskInfoDaoImpl) List(
	orgID string,
	pageData api.PaginationData,
	filterData api.TaskInfoFilterData,
) (api.TaskInfoCollectionResponse, int64, error) {
	var totalTasks int64

	var taskInfo models.TaskInfo
	tasks := make([]models.TaskInfoRepositoryConfiguration, 0)

	filteredDB := t.db.Table(taskInfo.TableName()+" AS t ").
		Select(JoinSelectQuery).
		Joins("LEFT JOIN repositories r on t.repository_uuid = r.uuid").
		Joins("LEFT JOIN repository_configurations rc on r.uuid = rc.repository_uuid").
		Where("t.org_id = ?", orgID)

	if filterData.Status != "" {
		filteredDB = filteredDB.Where("t.status = ?", filterData.Status)
	}

	if filterData.Typename != "" {
		filteredDB = filteredDB.Where("t.type = ?", filterData.Typename)
	}

	if filterData.RepoConfigUUID != "" {
		filteredDB = filteredDB.Where("rc.uuid = ?", filterData.RepoConfigUUID)
	}

	filteredDB.Find(&tasks).Count(&totalTasks)
	// Most recently queued (created) first
	filteredDB.Order("queued_at DESC").Offset(pageData.Offset).Limit(pageData.Limit).Find(&tasks)

	if filteredDB.Error != nil {
		return api.TaskInfoCollectionResponse{}, totalTasks, DBErrorToApi(filteredDB.Error)
	}
	taskResponses := convertTaskInfoToResponses(tasks)
	return api.TaskInfoCollectionResponse{Data: taskResponses}, totalTasks, nil
}

func (t taskInfoDaoImpl) IsSnapshotInProgress(orgID, repoUUID string) (bool, error) {
	taskInfo := models.TaskInfo{}
	result := t.db.Where("org_id = ? and repository_uuid = ? and status = ? and type = ?",
		orgID, repoUUID, config.TaskStatusRunning, config.RepositorySnapshotTask).First(&taskInfo)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return false, nil
		} else {
			return false, result.Error
		}
	}
	if taskInfo.Status == config.TaskStatusRunning {
		return true, nil
	}
	return false, nil
}

func taskInfoModelToApiFields(taskInfo *models.TaskInfoRepositoryConfiguration, apiTaskInfo *api.TaskInfoResponse) {
	apiTaskInfo.UUID = taskInfo.Id.String()
	apiTaskInfo.OrgId = taskInfo.OrgId
	apiTaskInfo.Status = taskInfo.Status
	apiTaskInfo.Typename = taskInfo.Typename
	apiTaskInfo.RepoConfigUUID = taskInfo.RepositoryConfigUUID
	apiTaskInfo.RepoConfigName = taskInfo.RepositoryConfigName

	if taskInfo.Error != nil {
		apiTaskInfo.Error = *taskInfo.Error
	}

	if taskInfo.Queued != nil {
		apiTaskInfo.CreatedAt = taskInfo.Queued.Format(time.RFC3339)
	}

	if taskInfo.Finished != nil {
		apiTaskInfo.EndedAt = taskInfo.Finished.Format(time.RFC3339)
	}
}

func convertTaskInfoToResponses(taskInfo []models.TaskInfoRepositoryConfiguration) []api.TaskInfoResponse {
	tasks := make([]api.TaskInfoResponse, len(taskInfo))
	for i := range taskInfo {
		taskInfoModelToApiFields(&taskInfo[i], &tasks[i])
	}
	return tasks
}
