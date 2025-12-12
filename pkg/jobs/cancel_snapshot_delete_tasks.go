package jobs

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/rs/zerolog/log"
)

func CancelSnapshotDeleteTasks(_ []string) {
	ctx := context.Background()

	q, err := queue.NewPgQueue(ctx, db.GetUrl())
	if err != nil {
		log.Fatal().Err(err).Msg("could not create task queue.")
		return
	}
	defer q.Close()

	var tasks []models.TaskInfo
	result := db.DB.Where("type = ? AND status in (?) AND finished_at IS NULL",
		config.DeleteSnapshotsTask,
		[]string{config.TaskStatusRunning, config.TaskStatusPending}).
		Find(&tasks)

	if result.Error != nil {
		log.Fatal().Err(result.Error).Msg("Could not query delete-snapshots tasks.")
		return
	}

	canceledCount := 0
	for _, task := range tasks {
		log.Info().Str("task_id", task.Id.String()).Msgf("[Job] canceling task")
		err := q.Cancel(ctx, task.Id)
		if err != nil {
			log.Warn().Err(err).Str("task_id", task.Id.String()).Msg("Failed to cancel task")
		} else {
			canceledCount++
		}
	}

	log.Warn().Msgf("Canceled %v of %v delete-snapshots tasks", canceledCount, len(tasks))
}
