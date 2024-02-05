package main

import (
	"os"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/rs/zerolog/log"
)

func main() {
	args := os.Args
	config.Load()
	config.ConfigureLogging()
	err := db.Connect()

	if err != nil {
		log.Panic().Err(err).Msg("Failed to connect to database")
	}

	if len(args) < 2 || args[1] != "--force" {
		log.Fatal().Msg("Requires arguments: --force")
	}

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
