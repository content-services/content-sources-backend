package dao

import (
	"context"
	"encoding/json"
	"errors"
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
	pulpClient pulp_client.PulpGlobalClient
}

func GetAdminTaskDao(db *gorm.DB, pulpClient pulp_client.PulpClient) AdminTaskDao {
	return adminTaskInfoDaoImpl{
		db:         db,
		pulpClient: pulpClient,
	}
}

func (a adminTaskInfoDaoImpl) Fetch(ctx context.Context, id string) (api.AdminTaskInfoResponse, error) {
	taskInfo := models.TaskInfo{}
	result := a.db.Where("id = ?", UuidifyString(id)).First(&taskInfo)

	taskInfoResponse := api.AdminTaskInfoResponse{}
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return taskInfoResponse, &ce.DaoError{NotFound: true, Message: "Could not find task with UUID " + id}
		} else {
			return taskInfoResponse, DBErrorToApi(result.Error)
		}
	}

	if taskInfo.Typename == payloads.Snapshot {
		pulpData, err := getPulpData(ctx, taskInfo, a.pulpClient)
		if err != nil {
			return api.AdminTaskInfoResponse{}, ce.NewErrorResponse(http.StatusInternalServerError, "Error parsing task payload", err.Error())
		}
		taskInfoResponse.Pulp = pulpData
	}
	taskInfoResponse.Payload = taskInfo.Payload

	adminTaskInfoModelToApiFields(&taskInfo, &taskInfoResponse)
	return taskInfoResponse, nil
}

func (a adminTaskInfoDaoImpl) List(
	ctx context.Context,
	pageData api.PaginationData,
	filterData api.AdminTaskFilterData,
) (api.AdminTaskInfoCollectionResponse, int64, error) {
	var totalTasks int64
	tasks := make([]models.TaskInfo, 0)

	filteredDB := a.db.WithContext(ctx)
	if filterData.OrgId != "" {
		filteredDB = filteredDB.Where("tasks.org_id = ?", filterData.OrgId)
	}

	if filterData.AccountId != "" {
		filteredDB = filteredDB.Where("tasks.account_id = ?", filterData.AccountId)
	}

	if filterData.Status != "" {
		statuses := strings.Split(filterData.Status, ",")
		filteredDB = filteredDB.Where("status IN ?", statuses)
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

	order := convertSortByToSQL(pageData.SortBy, sortMap, "started_at asc")

	result := filteredDB.Model(&tasks).Count(&totalTasks)
	if result.Error != nil {
		return api.AdminTaskInfoCollectionResponse{}, totalTasks, DBErrorToApi(filteredDB.Error)
	}

	result = filteredDB.Offset(pageData.Offset).Limit(pageData.Limit).Order(order).Find(&tasks)

	if result.Error != nil {
		return api.AdminTaskInfoCollectionResponse{}, totalTasks, DBErrorToApi(filteredDB.Error)
	}

	taskResponses := convertAdminTaskInfoToResponses(tasks)
	return api.AdminTaskInfoCollectionResponse{Data: taskResponses}, totalTasks, nil
}

func adminTaskInfoModelToApiFields(taskInfo *models.TaskInfo, apiTaskInfo *api.AdminTaskInfoResponse) {
	apiTaskInfo.UUID = taskInfo.Id.String()
	apiTaskInfo.OrgId = taskInfo.OrgId
	apiTaskInfo.Status = taskInfo.Status
	apiTaskInfo.Typename = taskInfo.Typename
	apiTaskInfo.AccountId = taskInfo.AccountId

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

func convertAdminTaskInfoToResponses(taskInfo []models.TaskInfo) []api.AdminTaskInfoResponse {
	tasks := make([]api.AdminTaskInfoResponse, len(taskInfo))
	for i := range taskInfo {
		adminTaskInfoModelToApiFields(&taskInfo[i], &tasks[i])
	}
	return tasks
}

func getPulpData(ctx context.Context, ti models.TaskInfo, pulpClient pulp_client.PulpGlobalClient) (api.PulpResponse, error) {
	if ti.Typename == payloads.Snapshot {
		var payload payloads.SnapshotPayload
		response := api.PulpResponse{}

		if err := json.Unmarshal(ti.Payload, &payload); err != nil {
			return api.PulpResponse{}, errors.New("invalid snapshot payload")
		}

		if payload.SyncTaskHref != nil {
			sync, syncErr := pulpClient.GetTask(ctx, *payload.SyncTaskHref)
			if syncErr != nil {
				return api.PulpResponse{}, syncErr
			}
			response.Sync = &api.PulpTaskResponse{}
			api.ZestTaskResponseToApi(&sync, response.Sync)
		}

		if payload.DistributionTaskHref != nil {
			distribution, distributionErr := pulpClient.GetTask(ctx, *payload.DistributionTaskHref)
			if distributionErr != nil {
				return api.PulpResponse{}, distributionErr
			}
			response.Distribution = &api.PulpTaskResponse{}
			api.ZestTaskResponseToApi(&distribution, response.Distribution)
		}

		if payload.PublicationTaskHref != nil {
			publication, publicationErr := pulpClient.GetTask(ctx, *payload.PublicationTaskHref)
			if publicationErr != nil {
				return api.PulpResponse{}, publicationErr
			}
			response.Publication = &api.PulpTaskResponse{}
			api.ZestTaskResponseToApi(&publication, response.Publication)
		}

		return response, nil
	}
	return api.PulpResponse{}, errors.New("incorrect task type")
}
