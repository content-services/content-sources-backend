package api

import (
	"encoding/json"
	"time"

	zest "github.com/content-services/zest/release/v3"
)

// AdminTaskInfoResponse holds data returned by a admin tasks API response
type AdminTaskInfoResponse struct {
	UUID       string          `json:"uuid"`           // UUID of the object
	Status     string          `json:"status"`         // Status of task (running, failed, completed, canceled, pending)
	Typename   string          `json:"typename"`       // Type of task (e.g. introspect, completed)
	QueuedAt   string          `json:"queued_at"`      // Timestamp task was queued at
	StartedAt  string          `json:"started_at"`     // Timestamp task started running at
	FinishedAt string          `json:"finished_at"`    // Timestamp task finished running at
	Error      string          `json:"error"`          // Error thrown while running task
	OrgId      string          `json:"org_id"`         // Organization ID of the owner
	AccountId  string          `json:"account_id"`     // Account ID of the owner
	Payload    json.RawMessage `json:"payload"`        // Payload of task (only returned in fetch)
	Pulp       PulpResponse    `json:"pulp,omitempty"` // Pulp data for snapshot tasks (only returned in fetch)
}

type AdminTaskInfoCollectionResponse struct {
	Data  []AdminTaskInfoResponse `json:"data"`  // Requested Data
	Meta  ResponseMetadata        `json:"meta"`  // Metadata about the request
	Links Links                   `json:"links"` // Links to other pages of results
}

type PulpResponse struct {
	Sync         *PulpTaskResponse `json:"sync,omitempty"`
	Distribution *PulpTaskResponse `json:"distribution,omitempty"`
	Publication  *PulpTaskResponse `json:"publication,omitempty"`
}

type PulpTaskResponse struct {
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

func (a *AdminTaskInfoCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	a.Meta = meta
	a.Links = links
}

func zestProgressReportToApi(zestProgressReport *zest.ProgressReportResponse, apiProgressReport *pulpProgressReportResponse) {
	apiProgressReport.Message = zestProgressReport.Message
	apiProgressReport.Code = zestProgressReport.Code
	apiProgressReport.State = zestProgressReport.State
	apiProgressReport.Total = zestProgressReport.Total
	apiProgressReport.Done = zestProgressReport.Done
	apiProgressReport.Suffix = zestProgressReport.Suffix
}

func ZestTaskResponseToApi(zestTaskResponse *zest.TaskResponse, apiTaskResponse *PulpTaskResponse) {
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
	if len(zestTaskResponse.ProgressReports) > 0 {
		apiTaskResponse.ProgressReports = make([]pulpProgressReportResponse, len(zestTaskResponse.ProgressReports))
		for i := range apiTaskResponse.ProgressReports {
			zestProgressReportToApi(&zestTaskResponse.ProgressReports[i], &apiTaskResponse.ProgressReports[i])
		}
	}
	apiTaskResponse.CreatedResources = zestTaskResponse.CreatedResources
	apiTaskResponse.ReservedResourcesRecord = zestTaskResponse.ReservedResourcesRecord
}
