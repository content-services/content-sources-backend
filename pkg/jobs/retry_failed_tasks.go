package jobs

import (
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/rs/zerolog/log"
)

func RetryFailedTasks() {
	query :=
		`
			UPDATE tasks
			SET next_retry_time = statement_timestamp(), retries = 0
			WHERE started_at IS NOT NULL AND finished_at IS NOT NULL 
		  		AND status = 'failed' AND type = 'delete-repository-snapshots';
		`
	result := db.DB.Exec(query)
	if result.Error != nil {
		log.Fatal().Err(result.Error).Msg("Could not update failed tasks.")
	} else {
		log.Warn().Msgf("Updated %v tasks", result.RowsAffected)
	}
}
