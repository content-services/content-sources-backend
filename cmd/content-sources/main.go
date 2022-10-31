package main

import (
	"net/http"
	"os"
	"sync"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/event"
	eventHandler "github.com/content-services/content-sources-backend/pkg/event/handler"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/rs/zerolog/log"
)

func main() {
	args := os.Args
	if len(args) < 2 {
		log.Fatal().Msg("arguments:  ./content-sources [api] [consumer]")
	}

	config.Load()
	config.ConfigureLogging()
	err := db.Connect()
	defer db.Close()

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database.")
	}

	var wg sync.WaitGroup
	// If we're not running an api server, still listen for ping requests for liveliness probes
	wg.Add(1)
	go apiServer(&wg, argsContain(args, "api"))

	if argsContain(args, "consumer") {
		wg.Add(1)
		go kafkaConsumer(&wg)
	}
	wg.Wait()
}

func argsContain(args []string, val string) bool {
	for i := 1; i < len(args); i++ {
		if args[i] == val {
			return true
		}
	}
	return false
}

func kafkaConsumer(wg *sync.WaitGroup) {
	handler := eventHandler.NewIntrospectHandler(db.DB)
	event.Start(&config.Get().Kafka, handler)
	wg.Done()
}

func apiServer(wg *sync.WaitGroup, allRoutes bool) {
	echo := config.ConfigureEcho()
	handler.RegisterPing(echo)
	if allRoutes {
		handler.RegisterRoutes(echo)
	}

	go func() {
		err := echo.Start(":8000")
		if err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
		wg.Done()
	}()
	config.ConfigureEchoShutdown(echo)
}
