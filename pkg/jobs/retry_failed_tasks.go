package jobs

import (
	"fmt"
	"strconv"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/rs/zerolog/log"
)

// RetryFailedTasks resets failed tasks so they can be re-queued by the background processor
// Usage: go run cmd/jobs/main.go retry-failed-tasks [task-type] [hours]
// Examples:
//   - retry-failed-tasks delete-repository-snapshots 6
//   - retry-failed-tasks snapshot 12
//   - retry-failed-tasks (defaults to delete-repository-snapshots, 6 hours)
func RetryFailedTasks(args []string) {
	taskType := "delete-repository-snapshots"
	hours := 6

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

	query := fmt.Sprintf(`
		UPDATE tasks
		SET next_retry_time = statement_timestamp(), retries = 0
		WHERE started_at IS NOT NULL AND finished_at IS NOT NULL
			AND status = 'failed' AND type = $1
			AND finished_at >= NOW() - INTERVAL '%d hours';
	`, hours)

	result := db.DB.Exec(query, taskType)
	if result.Error != nil {
		log.Fatal().Err(result.Error).Msg("Could not update failed tasks.")
	} else {
		log.Info().
			Str("task_type", taskType).
			Int("hours", hours).
			Int64("updated_count", result.RowsAffected).
			Msgf("Updated %v failed '%s' tasks from the last %d hours", result.RowsAffected, taskType, hours)
	}
}
