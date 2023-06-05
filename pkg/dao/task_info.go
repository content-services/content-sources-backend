package dao

import (
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

type taskInfoDaoImpl struct {
	db *gorm.DB
}

func GetTaskInfoDao(db *gorm.DB) TaskInfoDao {
	return taskInfoDaoImpl{
		db: db,
	}
}

func (t taskInfoDaoImpl) Fetch(orgId string, id string) (api.TaskInfoResponse, error) {
	taskInfo := models.TaskInfo{}
	result := t.db.Where("id = ? AND org_id = ?", id, orgId).First(&taskInfo)
	taskInfoResponse := api.TaskInfoResponse{}

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return taskInfoResponse, &ce.DaoError{NotFound: true, Message: "Could not find task with UUID " + id}
		} else {
			return taskInfoResponse, result.Error
		}
	}
	taskInfoModelToApiFields(&taskInfo, &taskInfoResponse)
	return taskInfoResponse, nil
}

func (t taskInfoDaoImpl) List(
	orgId string,
	pageData api.PaginationData,
	statusFilter string,
) (api.TaskInfoCollectionResponse, int64, error) {
	var totalTasks int64
	tasks := make([]models.TaskInfo, 0)

	filteredDB := t.db.Where("org_id = ?", orgId)

	if statusFilter != "" {
		filteredDB = filteredDB.Where("status = ?", statusFilter)
	}

	filteredDB.Find(&tasks).Count(&totalTasks)
	// Most recently queued (created) first
	filteredDB.Order("queued_at DESC").Offset(pageData.Offset).Limit(pageData.Limit).Find(&tasks)

	if filteredDB.Error != nil {
		return api.TaskInfoCollectionResponse{}, totalTasks, filteredDB.Error
	}
	taskResponses := convertTaskInfoToResponses(tasks)
	return api.TaskInfoCollectionResponse{Data: taskResponses}, totalTasks, nil
}

func taskInfoModelToApiFields(taskInfo *models.TaskInfo, apiTaskInfo *api.TaskInfoResponse) {
	apiTaskInfo.UUID = taskInfo.Id.String()
	apiTaskInfo.OrgId = taskInfo.OrgId
	apiTaskInfo.Status = taskInfo.Status

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

func convertTaskInfoToResponses(taskInfo []models.TaskInfo) []api.TaskInfoResponse {
	tasks := make([]api.TaskInfoResponse, len(taskInfo))
	for i := range taskInfo {
		taskInfoModelToApiFields(&taskInfo[i], &tasks[i])
	}
	return tasks
}
