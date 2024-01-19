package main

import (
	"os"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
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

	result := db.DB.Exec("DELETE FROM snapshots")
	if result.Error != nil {
		log.Fatal().Err(result.Error).Msg("Could not delete snapshots.")
	} else {
		log.Warn().Msgf("Deleted %v snapshots", result.RowsAffected)
	}
}
