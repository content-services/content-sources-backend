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

	query :=
		`		
			UPDATE snapshots
			SET version_href = CONCAT('/api', version_href)
			WHERE version_href NOT LIKE '/api%';
			
			UPDATE snapshots
			SET publication_href = CONCAT('/api', publication_href)
			WHERE publication_href NOT LIKE '/api%';

			UPDATE snapshots
			SET distribution_href = CONCAT('/api', distribution_href)
			WHERE distribution_href NOT LIKE '/api%';
		`
	result := db.DB.Exec(query)
	if result.Error != nil {
		log.Fatal().Err(result.Error).Msg("Could not update hrefs.")
	} else {
		log.Warn().Msgf("Updated %v hrefs", result.RowsAffected)
	}
}
