package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/event"
	eventHandler "github.com/content-services/content-sources-backend/pkg/event/handler"
	"github.com/content-services/content-sources-backend/pkg/handler"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

func main() {
	reg := prometheus.NewRegistry()
	metrics := m.NewMetrics(reg)

	args := os.Args
	if len(args) < 2 {
		log.Fatal().Msg("arguments:  ./content-sources [api] [consumer] [instrumentation]")
	}

	config.Load()
	config.ConfigureLogging()
	err := db.Connect()
	defer db.Close()

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database.")
	}

	// Setup cancellation context
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		exit := make(chan os.Signal, 1)
		signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		<-exit
		cancel()
	}()

	// If we're not running an api server, still listen for ping requests for liveliness probes
	apiServer(ctx, &wg, argsContain(args, "api"), metrics)

	if argsContain(args, "consumer") {
		kafkaConsumer(ctx, &wg, metrics)
	}

	if argsContain(args, "instrumentation") {
		instrumentation(ctx, &wg, metrics)
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

func kafkaConsumer(ctx context.Context, wg *sync.WaitGroup, metrics *m.Metrics) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		handler := eventHandler.NewIntrospectHandler(db.DB)
		event.Start(ctx, &config.Get().Kafka, handler)
		log.Logger.Info().Msgf("kafkaConsumer stopped")
	}()
}

func apiServer(ctx context.Context, wg *sync.WaitGroup, allRoutes bool, metrics *m.Metrics) {
	wg.Add(2) // api server & shutdown monitor

	echo := config.ConfigureEchoService(metrics)
	handler.RegisterPing(echo)
	if allRoutes {
		handler.RegisterRoutes(echo)
	}

	go func() {
		defer wg.Done()
		err := echo.Start(":8000")
		if err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
		log.Logger.Info().Msgf("apiServer stopped")
	}()

	go func() {
		defer wg.Done()
		<-ctx.Done()
		log.Logger.Info().Msg("Caught context done, closing api server.")
		if err := echo.Shutdown(context.Background()); err != nil {
			echo.Logger.Fatal(err)
		}
	}()
}

func instrumentation(ctx context.Context, wg *sync.WaitGroup, metrics *m.Metrics) {
	wg.Add(1)
	e := config.ConfigureEchoMetrics()

	metricsPath := config.Get().Metrics.Path
	metricsPort := config.Get().Metrics.Port

	e.Add(http.MethodGet, metricsPath, echo.WrapHandler(promhttp.HandlerFor(
		metrics.Registry(),
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
			// Pass custom registry
			Registry: metrics.Registry(),
		},
	)))
	e.HideBanner = true
	go func() {
		defer wg.Done()
		log.Logger.Info().Msgf("Starting instrumentation")
		if err := e.Start(fmt.Sprintf(":%d", metricsPort)); err != nil && err != http.ErrServerClosed {
			log.Logger.Error().Msgf("error starting instrumentation: %s", err.Error())
		}
		log.Logger.Info().Msgf("instrumentation stopped")
	}()

	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := e.Shutdown(shutdownContext); err != nil {
			log.Logger.Error().Msgf("error stopping instrumentation: %s", err.Error())
		}
		cancel()
	}()
}
