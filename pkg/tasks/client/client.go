package client

import (
	"context"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
)

//go:generate mockery  --name TaskClient --filename client_mock.go --inpackage
type TaskClient interface {
	Enqueue(task queue.Task) (uuid.UUID, error)
	// TODO: cancelling a task immeadiately after enqueueing may result in task getting stuck pending until
	// new task is enqueued. Maybe because some kind of resource lock on task from TryCancel when Dequeue is run?
	TryCancel(ctx context.Context, taskId string) error
}

type Client struct {
	queue queue.Queue
}

func NewTaskClient(q queue.Queue) TaskClient {
	return &Client{
		queue: q,
	}
}

// TODO propgate context to enqueue
func (c *Client) Enqueue(task queue.Task) (uuid.UUID, error) {
	id, err := c.queue.Enqueue(&task)
	if err != nil {
		return uuid.Nil, err
	}
	logger := tasks.LogForTask(id.String(), task.Typename, task.RequestID)
	logger.Info().Msg("[Enqueued Task]")
	return id, nil
}

func (c *Client) TryCancel(ctx context.Context, taskId string) error {
	taskUUID, err := uuid.Parse(taskId)
	if err != nil {
		return err
	}
	err = c.queue.TryCancel(ctx, taskUUID)
	if err != nil {
		return err
	}
	return nil
}
