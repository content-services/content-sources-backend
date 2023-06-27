package dao

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"gorm.io/gorm"
)

type adminTaskInfoDaoImpl struct {
	db         *gorm.DB
	pulpClient pulp_client.PulpClient
}

func GetAdminTaskDao(db *gorm.DB, pulpClient pulp_client.PulpClient) AdminTaskDao {
	return adminTaskInfoDaoImpl{
		db:         db,
		pulpClient: pulpClient,
	}
}

func (a adminTaskInfoDaoImpl) Fetch(id string) (api.AdminTaskInfoResponse, error) {
	taskInfo := models.TaskInfo{}
	query := a.db.Where("id = ?", id).Joins("LEFT JOIN repository_configurations ON (tasks.repository_uuid = repository_configurations.repository_uuid AND tasks.org_id = repository_configurations.org_id)")
	result := query.First(&taskInfo)
	var account_id sql.NullString
	query.Select("account_id").First(&account_id)

	taskInfoResponse := api.AdminTaskInfoResponse{}

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return taskInfoResponse, &ce.DaoError{NotFound: true, Message: "Could not find task with UUID " + id}
		} else {
			return taskInfoResponse, result.Error
		}
	}

	if taskInfo.Typename == payloads.Snapshot {
		pulpData, err := taskInfo.GetPulpData(a.pulpClient)
		if err != nil {
			return api.AdminTaskInfoResponse{}, ce.NewErrorResponse(http.StatusInternalServerError, "Error parsing task payload", err.Error())
		}
		taskInfoResponse.Pulp = pulpData
	}
	taskInfoResponse.Payload = taskInfo.Payload

	adminTaskInfoModelToApiFields(&taskInfo, account_id, &taskInfoResponse)
	return taskInfoResponse, nil
}

func (a adminTaskInfoDaoImpl) List(
	pageData api.PaginationData,
	filterData api.AdminTaskFilterData,
) (api.AdminTaskInfoCollectionResponse, int64, error) {
	var totalTasks int64
	tasks := make([]models.TaskInfo, 0)
	accountIds := make([]sql.NullString, 0) // Could be nil

	// Finds associated repository configuration for the task's org ID, only need account id
	filteredDB := a.db.Select("tasks.*", "repository_configurations.account_id").Joins("LEFT JOIN repository_configurations ON (tasks.repository_uuid = repository_configurations.repository_uuid AND tasks.org_id = repository_configurations.org_id)")

	if filterData.OrgId != "" {
		filteredDB = filteredDB.Where("tasks.org_id = ?", filterData.OrgId)
	}

	if filterData.AccountId != "" {
		filteredDB = filteredDB.Where("account_id = ?", filterData.AccountId)
	}

	if filterData.Status != "" {
		statuses := strings.Split(strings.ToUpper(filterData.Status), ",")
		// Case insensitive
		filteredDB = filteredDB.Where("UPPER(status) IN ?", statuses)
	}

	sortMap := map[string]string{
		"org_id":      "tasks.org_id",
		"account_id":  "account_id",
		"typename":    "type",
		"queued_at":   "queued_at",
		"started_at":  "started_at",
		"finished_at": "finished_at",
		"status":      "status",
	}

	order := convertSortByToSQL(pageData.SortBy, sortMap)

	filteredDB.Order(order).Model(&tasks).Count(&totalTasks)
	filteredDB.Offset(pageData.Offset).Limit(pageData.Limit)
	filteredDB.Find(&tasks)
	filteredDB.Select("account_id").Find(&accountIds)

	if filteredDB.Error != nil {
		return api.AdminTaskInfoCollectionResponse{}, totalTasks, filteredDB.Error
	}

	taskResponses := convertAdminTaskInfoToResponses(tasks, accountIds)
	return api.AdminTaskInfoCollectionResponse{Data: taskResponses}, totalTasks, nil
}

func adminTaskInfoModelToApiFields(taskInfo *models.TaskInfo, accountId sql.NullString, apiTaskInfo *api.AdminTaskInfoResponse) {
	apiTaskInfo.UUID = taskInfo.Id.String()
	apiTaskInfo.OrgId = taskInfo.OrgId
	apiTaskInfo.Status = taskInfo.Status
	apiTaskInfo.Typename = taskInfo.Typename
	if accountId.Valid {
		apiTaskInfo.AccountId = accountId.String
	}

	if taskInfo.Error != nil {
		apiTaskInfo.Error = *taskInfo.Error
	}

	if taskInfo.Queued != nil {
		apiTaskInfo.QueuedAt = taskInfo.Queued.Format(time.RFC3339)
	}

	if taskInfo.Started != nil {
		apiTaskInfo.StartedAt = taskInfo.Started.Format(time.RFC3339)
	}

	if taskInfo.Finished != nil {
		apiTaskInfo.FinishedAt = taskInfo.Finished.Format(time.RFC3339)
	}
}

func convertAdminTaskInfoToResponses(taskInfo []models.TaskInfo, accountIds []sql.NullString) []api.AdminTaskInfoResponse {
	tasks := make([]api.AdminTaskInfoResponse, len(taskInfo))
	for i := range taskInfo {
		adminTaskInfoModelToApiFields(&taskInfo[i], accountIds[i], &tasks[i])
	}
	return tasks
}
