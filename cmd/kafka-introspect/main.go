package main

import (
	"github.com/rs/zerolog/log"

	config "github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/event/handler"
)

func main() {
	log.Logger.Debug().Msg("Reading configuration")
	cfg := config.Get()
	log.Logger.Debug().Msg("Connecting to database")
	if err := db.Connect(); err != nil {
		panic(err)
	}
	log.Logger.Debug().Msg("Initializing handler")
	handler := handler.NewIntrospectHandler(db.DB)
	log.Logger.Debug().Msg("Starting run loop")
	event.Start(cfg, handler)
}
