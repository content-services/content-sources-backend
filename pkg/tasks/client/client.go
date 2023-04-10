package client

import (
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

//go:generate mockery --name TaskClient
type TaskClient interface {
	Enqueue(task queue.Task) (uuid.UUID, error)
}

type Client struct {
	queue  queue.Queue
	logger *zerolog.Logger
}

func NewTaskClient(q queue.Queue) TaskClient {
	return &Client{
		queue:  q,
		logger: &log.Logger,
	}
}

func (c *Client) Enqueue(task queue.Task) (uuid.UUID, error) {
	return c.queue.Enqueue(&task)
}
