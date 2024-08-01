package tasks

import (
	"context"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
)

type UpdateLatestSnapshotPayload struct {
}

func UpdateLatestSnapshotHandler(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error {
	return nil
}
