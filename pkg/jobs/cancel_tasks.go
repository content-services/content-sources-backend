package jobs

import (
	"context"
	"strconv"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// CancelTasks cancels tasks of a specific type, optionally filtered by how long ago they were queued
// Usage: go run cmd/jobs/main.go cancel-tasks [task-type] [hours]
// Examples:
//   - cancel-tasks delete-repository-snapshots 3  (cancel tasks queued in the last 3 hours)
//   - cancel-tasks snapshot 6                     (cancel tasks queued in the last 6 hours)
//   - cancel-tasks delete-repository-snapshots    (cancel all pending/running tasks, no time constraint)
//   - cancel-tasks                                (defaults to delete-repository-snapshots, all tasks)
func CancelTasks(args []string) {
	taskType := config.DeleteRepositorySnapshotsTask
	hours := 0 // 0 means no time constraint

	// Parse optional task type argument
	if len(args) > 0 && args[0] != "" {
		taskType = args[0]
	}

	// Parse optional hours argument
	if len(args) > 1 {
		parsedHours, err := strconv.Atoi(args[1])
		if err != nil {
			log.Fatal().Err(err).Msg("Invalid hours parameter. Must be an integer.")
		}
		hours = parsedHours
	}

	ctx := context.Background()

	q, err := queue.NewPgQueue(ctx, db.GetUrl())
	if err != nil {
		log.Fatal().Err(err).Msg("could not create task queue.")
		return
	}
	defer q.Close()

	var tasks []models.TaskInfo
	var result *gorm.DB

	if hours > 0 {
		// Cancel tasks queued in the last N hours
		cutoffTime := time.Now().Add(-time.Duration(hours) * time.Hour)
		result = db.DB.Where("type = ? AND status in (?) AND finished_at IS NULL AND queued_at >= ?",
			taskType,
			[]string{config.TaskStatusPending, config.TaskStatusRunning},
			cutoffTime).
			Find(&tasks)
	} else {
		result = db.DB.Where("type = ? AND status in (?) AND finished_at IS NULL",
			taskType,
			[]string{config.TaskStatusPending, config.TaskStatusRunning}).
			Find(&tasks)
	}

	if result.Error != nil {
		log.Fatal().Err(result.Error).Str("task_type", taskType).Msg("Could not query tasks.")
		return
	}

	if hours > 0 {
		cutoffTime := time.Now().Add(-time.Duration(hours) * time.Hour)
		log.Info().
			Str("task_type", taskType).
			Int("hours", hours).
			Time("cutoff_time", cutoffTime).
			Int("found_count", len(tasks)).
			Msgf("Found %d '%s' tasks queued in the last %d hours to cancel", len(tasks), taskType, hours)
	} else {
		log.Info().
			Str("task_type", taskType).
			Int("found_count", len(tasks)).
			Msgf("Found %d '%s' tasks to cancel (no time constraint)", len(tasks), taskType)
	}

	canceledCount := 0
	for _, task := range tasks {
		err := q.Cancel(ctx, task.Id)
		if err != nil {
			log.Warn().Err(err).Str("task_id", task.Id.String()).Msg("Failed to cancel task")
		} else {
			canceledCount++
		}
	}

	log.Info().
		Str("task_type", taskType).
		Int("canceled_count", canceledCount).
		Int("total_count", len(tasks)).
		Msgf("Canceled %v of %v '%s' tasks", canceledCount, len(tasks), taskType)
}
