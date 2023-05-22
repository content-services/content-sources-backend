package client

import (
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
)

//go:generate mockery --name TaskClient
type TaskClient interface {
	Enqueue(task queue.Task) (uuid.UUID, error)
}

type Client struct {
	queue queue.Queue
}

func NewTaskClient(q queue.Queue) TaskClient {
	return &Client{
		queue: q,
	}
}

func (c *Client) Enqueue(task queue.Task) (uuid.UUID, error) {
	return c.queue.Enqueue(&task)
}
