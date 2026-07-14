package jobs

import (
	"strconv"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

const (
	defaultRetryFailedDeletionLimit = 100
	// minAgeForDeletionRetryHours ensures each task is retried at most once per day
	// by the daily cron, even if the job is invoked manually.
	minAgeForDeletionRetryHours = 24
)

// RetryFailedDeletionTasks resets permanently failed deletion tasks so they can be re-queued
// by the background processor. Targets tasks that have exhausted automatic retries.
// Usage: go run cmd/jobs/main.go retry-failed-deletion-tasks [limit]
// Example: retry-failed-deletion-tasks 100
func RetryFailedDeletionTasks(args []string) {
	limit := defaultRetryFailedDeletionLimit

	if len(args) > 0 && args[0] != "" {
		parsedLimit, err := strconv.Atoi(args[0])
		if err != nil {
			log.Fatal().Err(err).Msg("Invalid limit parameter. Must be an integer.")
		}
		if parsedLimit <= 0 {
			log.Fatal().Msg("Invalid limit parameter. Must be greater than zero.")
		}
		limit = parsedLimit
	}

	query := `
		UPDATE tasks
		SET next_retry_time = statement_timestamp(), retries = 0
		WHERE id IN (
			SELECT id FROM tasks
			WHERE started_at IS NOT NULL AND finished_at IS NOT NULL
				AND status = 'failed' AND cancel_attempted = false
				AND type = ANY($1::text[])
				AND retries >= $2
				AND finished_at <= statement_timestamp() - make_interval(hours => $4)
			ORDER BY finished_at ASC
			LIMIT $3
		)`

	result := db.DB.Exec(query, pq.Array(config.DeletionTasks), queue.MaxTaskRetries, limit, minAgeForDeletionRetryHours)
	if result.Error != nil {
		log.Fatal().Err(result.Error).Msg("Could not update failed deletion tasks.")
	}

	log.Info().
		Int("limit", limit).
		Int("min_age_hours", minAgeForDeletionRetryHours).
		Strs("task_types", config.DeletionTasks).
		Int64("updated_count", result.RowsAffected).
		Msgf("Updated %v permanently failed deletion tasks for retry", result.RowsAffected)
}
