// adapted from: https://github.com/osbuild/osbuild-composer/blob/main/pkg/jobqueue/jobqueue.go

package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/google/uuid"
)

const MaxTaskRetries = 3 // Maximum number of times a task can be retried before failing

type Task struct {
	Typename       string
	Payload        interface{}
	Dependencies   []uuid.UUID
	OrgId          string
	AccountId      string
	RepositoryUUID *string
	RequestID      string
	Priority       int
}

//go:generate $GO_OUTPUT/mockery  --name Queue --filename queue_mock.go --inpackage
type Queue interface {
	// Enqueue Enqueues a job
	Enqueue(task *Task) (uuid.UUID, error)
	// Dequeue Dequeues a job of a type in taskTypes, blocking until one is available.
	Dequeue(ctx context.Context, taskTypes []string) (*models.TaskInfo, error)
	// Status returns Status of the given task
	Status(taskId uuid.UUID) (*models.TaskInfo, error)
	// Finish finishes given task, setting status to completed or failed if taskError is not nil
	Finish(taskId uuid.UUID, taskError error) error
	// Requeue requeues the given task
	Requeue(taskId uuid.UUID) error
	// Heartbeats returns the tokens of all tasks older than given duration
	Heartbeats(olderThan time.Duration) []uuid.UUID
	// IdFromToken returns a task's ID given its token
	IdFromToken(token uuid.UUID) (id uuid.UUID, isRunning bool, err error)
	// RefreshHeartbeat refresh heartbeat of task given its token
	RefreshHeartbeat(token uuid.UUID) error
	// UpdatePayload update the payload on a task
	UpdatePayload(task *models.TaskInfo, payload interface{}) (*models.TaskInfo, error)
	// ListenForCancel registers a channel and listens for notification for given task, then calls cancelFunc on receive. Should run as goroutine.
	ListenForCancel(ctx context.Context, taskID uuid.UUID, cancelFunc context.CancelCauseFunc)
	// SendCancelNotification sends notification to cancel given task
	SendCancelNotification(ctx context.Context, taskId uuid.UUID) error
	// RequeueFailedTasks requeues all failed tasks of taskTypes to the queue
	RequeueFailedTasks(taskTypes []string) error
}

var (
	ErrNotExist           = fmt.Errorf("task does not exist")
	ErrNotRunning         = fmt.Errorf("task is not running")
	ErrTaskCanceled       = fmt.Errorf("task was canceled")
	ErrContextCanceled    = fmt.Errorf("dequeue context timed out or was canceled")
	ErrRowsNotAffected    = fmt.Errorf("no rows were affected")
	ErrMaxRetriesExceeded = fmt.Errorf("task has exceeded the maximum amount of retries")
)
