package main

import (
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/rs/zerolog/log"
)

func main() {
	config.Load()
	config.ConfigureLogging()
	echo := config.ConfigureEcho()

	err := db.Connect()
	if err != nil {
		log.Fatal().Err(err)
	}

	handler.RegisterRoutes(echo)
	err = echo.Start(":8000")
	if err != nil {
		log.Fatal().Err(err)
	}
}
