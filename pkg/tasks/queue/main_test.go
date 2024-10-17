package queue

import (
	"os"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/rs/zerolog/log"
)

func TestMain(m *testing.M) {
	// open database connection
	var err = db.Connect()
	config.ConfigureLogging()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open DB")
	}

	// run tests
	exitCode := m.Run()

	if err := db.Close(); err != nil {
		log.Fatal().Err(err).Msg("Failed to close DB")
	}
	os.Exit(exitCode)
}
