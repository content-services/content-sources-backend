// adapted from: https://github.com/osbuild/osbuild-composer/blob/main/pkg/jobqueue/jobqueue.go

package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/google/uuid"
)

type Task struct {
	Typename       string
	Payload        interface{}
	Dependencies   []uuid.UUID
	OrgId          string
	RepositoryUUID string
	RequestID      string
}

//go:generate mockery  --name Queue --filename queue_mock.go --inpackage
type Queue interface {
	// Enqueue Enqueues a job
	Enqueue(task *Task) (uuid.UUID, error)
	// Dequeue Dequeues a job of a type in taskTypes, blocking until one is available.
	// TODO possibly make this non-blocking and handle that elsewhere
	Dequeue(ctx context.Context, taskTypes []string) (*models.TaskInfo, error)
	// Status returns Status of the given task
	Status(taskId uuid.UUID) (*models.TaskInfo, error)
	// Finish finishes given task, setting status to completed or failed if taskError is not nil
	Finish(taskId uuid.UUID, taskError error) error
	// Cancel sets status of given task to canceled
	Cancel(taskId uuid.UUID) error
	// TryCancel sets the try cancel flag to true to mark the task for cancellation
	TryCancel(ctx context.Context, taskId uuid.UUID) error
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
}

var (
	ErrNotExist        = fmt.Errorf("task does not exist")
	ErrNotRunning      = fmt.Errorf("task is not running")
	ErrCanceled        = fmt.Errorf("task was canceled")
	ErrContextCanceled = fmt.Errorf("dequeue context timed out or was canceled")
	ErrRowsNotAffected = fmt.Errorf("no rows were affected")
)
