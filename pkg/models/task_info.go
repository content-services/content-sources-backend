package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	zest "github.com/content-services/zest/release/v3"
	"github.com/google/uuid"
)

// Shared by DAO and queue packages
// GORM only used in DAO to read from table
type TaskInfo struct {
	Id             uuid.UUID       `gorm:"primary_key;column:id"`
	Typename       string          `gorm:"column:type"` // "introspect" or "snapshot"
	Payload        json.RawMessage `gorm:"type:jsonb"`
	OrgId          string
	RepositoryUUID uuid.UUID
	Dependencies   []uuid.UUID `gorm:"-"`
	Token          uuid.UUID
	Queued         *time.Time `gorm:"column:queued_at"`
	Started        *time.Time `gorm:"column:started_at"`
	Finished       *time.Time `gorm:"column:finished_at"`
	Error          *string
	Status         string
}

func (*TaskInfo) TableName() string {
	return "tasks"
}

type pulpTaskResponse struct {
	PulpHref *string `json:"pulp_href,omitempty"`
	// Timestamp of creation.
	PulpCreated *time.Time `json:"pulp_created,omitempty"`
	// The current state of the task. The possible values include: 'waiting', 'skipped', 'running', 'completed', 'failed', 'canceled' and 'canceling'.
	State *string `json:"state,omitempty"`
	// The name of task.
	Name string `json:"name"`
	// The logging correlation id associated with this task
	LoggingCid string `json:"logging_cid"`
	// Timestamp of the when this task started execution.
	StartedAt *time.Time `json:"started_at,omitempty"`
	// Timestamp of the when this task stopped execution.
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// A JSON Object of a fatal error encountered during the execution of this task.
	Error map[string]map[string]interface{} `json:"error,omitempty"`
	// The worker associated with this task. This field is empty if a worker is not yet assigned.
	Worker *string `json:"worker,omitempty"`
	// The parent task that spawned this task.
	ParentTask *string `json:"parent_task,omitempty"`
	// Any tasks spawned by this task.
	ChildTasks []string `json:"child_tasks,omitempty"`
	// The task group that this task is a member of.
	TaskGroup       *string                      `json:"task_group,omitempty"`
	ProgressReports []pulpProgressReportResponse `json:"progress_reports,omitempty"`
	// Resources created by this task.
	CreatedResources []string `json:"created_resources,omitempty"`
	// A list of resources required by that task.
	ReservedResourcesRecord []string `json:"reserved_resources_record,omitempty"`
}

type pulpProgressReportResponse struct {
	// The message shown to the user for the progress report.
	Message *string `json:"message,omitempty"`
	// Identifies the type of progress report'.
	Code *string `json:"code,omitempty"`
	// The current state of the progress report. The possible values are: 'waiting', 'skipped', 'running', 'completed', 'failed', 'canceled' and 'canceling'. The default is 'waiting'.
	State *string `json:"state,omitempty"`
	// The total count of items.
	Total *int64 `json:"total,omitempty"`
	// The count of items already processed. Defaults to 0.
	Done *int64 `json:"done,omitempty"`
	// The suffix to be shown with the progress report.
	Suffix zest.NullableString `json:"suffix,omitempty"`
}

// Default
func zestTaskResponseToJSON(zestTask *zest.TaskResponse) (json.RawMessage, error) {
	apiTaskResponse := pulpTaskResponse{}
	zestTaskResponseToApi(zestTask, &apiTaskResponse)
	jsonData, err := json.Marshal(apiTaskResponse)
	return jsonData, err
}

func zestProgressReportToApi(zestProgressReport *zest.ProgressReportResponse, apiProgressReport *pulpProgressReportResponse) {
	apiProgressReport.Message = zestProgressReport.Message
	apiProgressReport.Code = zestProgressReport.Code
	apiProgressReport.State = zestProgressReport.State
	apiProgressReport.Total = zestProgressReport.Total
	apiProgressReport.Done = zestProgressReport.Done
	apiProgressReport.Suffix = zestProgressReport.Suffix
}

func zestTaskResponseToApi(zestTaskResponse *zest.TaskResponse, apiTaskResponse *pulpTaskResponse) {
	apiTaskResponse.PulpHref = zestTaskResponse.PulpHref
	apiTaskResponse.PulpCreated = zestTaskResponse.PulpCreated
	apiTaskResponse.State = zestTaskResponse.State
	apiTaskResponse.Name = zestTaskResponse.Name
	apiTaskResponse.LoggingCid = zestTaskResponse.LoggingCid
	apiTaskResponse.StartedAt = zestTaskResponse.StartedAt
	apiTaskResponse.FinishedAt = zestTaskResponse.FinishedAt
	apiTaskResponse.Error = zestTaskResponse.Error
	apiTaskResponse.Worker = zestTaskResponse.Worker
	apiTaskResponse.ParentTask = zestTaskResponse.ParentTask
	apiTaskResponse.ChildTasks = zestTaskResponse.ChildTasks
	apiTaskResponse.TaskGroup = zestTaskResponse.TaskGroup
	apiTaskResponse.ProgressReports = make([]pulpProgressReportResponse, len(zestTaskResponse.ProgressReports))
	for i := range apiTaskResponse.ProgressReports {
		zestProgressReportToApi(&zestTaskResponse.ProgressReports[i], &apiTaskResponse.ProgressReports[i])
	}
	apiTaskResponse.CreatedResources = zestTaskResponse.CreatedResources
	apiTaskResponse.ReservedResourcesRecord = zestTaskResponse.ReservedResourcesRecord
}

func populatePulpTaskData(pulpClient pulp_client.PulpClient, taskHref *string, key string, dataMap *map[string]interface{}) error {
	if taskHref == nil {
		return nil
	}
	taskDetails, pulpErr := pulpClient.GetTask(*taskHref)
	if pulpErr != nil {
		return pulpErr
	}
	jsonPulpTask, jsonErr := zestTaskResponseToJSON(&taskDetails)
	if jsonErr != nil {
		return jsonErr
	}
	(*dataMap)[key] = jsonPulpTask
	return nil
}

func (ti *TaskInfo) ParsePayload(pulpClient pulp_client.PulpClient) (json.RawMessage, error) {
	if ti.Typename == payloads.Snapshot {
		var payload payloads.SnapshotPayload
		if err := json.Unmarshal(ti.Payload, &payload); err != nil {
			return nil, fmt.Errorf("payload incorrect type for SnapshotHandler")
		}
		data := make(map[string]interface{})

		if err := populatePulpTaskData(pulpClient, payload.SyncTaskHref, "sync", &data); err != nil {
			return nil, err
		}
		if err := populatePulpTaskData(pulpClient, payload.PublicationTaskHref, "publication", &data); err != nil {
			return nil, err
		}
		if err := populatePulpTaskData(pulpClient, payload.DistributionTaskHref, "distribution", &data); err != nil {
			return nil, err
		}

		json_data, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		return json_data, nil
	}
	return ti.Payload, nil
}
