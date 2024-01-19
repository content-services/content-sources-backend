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
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/handler"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	custom_collector "github.com/content-services/content-sources-backend/pkg/instrumentation/custom"
	"github.com/content-services/content-sources-backend/pkg/router"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/tasks/worker"
	mocks_rbac "github.com/content-services/content-sources-backend/pkg/test/mocks/rbac"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
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
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database.")
	}
	defer db.Close()

	if argsContain(args, "api") {
		err = config.ConfigureTang()
		if err != nil {
			log.Panic().Err(err).Msg("Could not initialize tang, was pulp database information provided?")
		}
		if config.Tang != nil {
			defer (*config.Tang).Close()
		}
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

	if argsContain(args, "mock_rbac") {
		mockRbac(ctx, &wg)
	}
	config.SetupNotifications()

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
		pgqueue, err := queue.NewPgQueue(db.GetUrl())
		if err != nil {
			panic(err)
		}
		wrk := worker.NewTaskWorkerPool(&pgqueue, metrics)
		wrk.RegisterHandler(config.IntrospectTask, tasks.IntrospectHandler)
		wrk.RegisterHandler(config.RepositorySnapshotTask, tasks.SnapshotHandler)
		wrk.RegisterHandler(config.DeleteRepositorySnapshotsTask, tasks.DeleteSnapshotHandler)
		wrk.HeartbeatListener()
		go wrk.StartWorkers(ctx)
		<-ctx.Done()
		wrk.Stop()
	}()
}

func apiServer(ctx context.Context, wg *sync.WaitGroup, allRoutes bool, metrics *m.Metrics) {
	wg.Add(2) // api server & shutdown monitor

	echo := router.ConfigureEchoWithMetrics(metrics)
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
	wg.Add(2)
	e := router.ConfigureEcho(false)

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

	// Custom go routine
	custom_ctx, custom_cancel := context.WithCancelCause(ctx)
	custom := custom_collector.NewCollector(custom_ctx, metrics, db.DB)
	go func() {
		defer wg.Done()
		log.Logger.Info().Msgf("Starting custom metrics go routine")
		custom.Run()
		log.Logger.Info().Msgf("custom metrics stopped")
	}()

	go func() {
		<-custom_ctx.Done()
		custom_cancel(ce.ErrServerExited)
	}()
}

func mockRbac(ctx context.Context, wg *sync.WaitGroup) {
	// If clients.rbac_enabled is false into the configuration or
	// the environment variable CLIENTS_RBAC_ENABLED, then
	// no service is started
	// CLIENTS_RBAC_BASE_URL environment variable can be used
	// to point out to http://localhost:8800/api/rbac/v1
	// for developing proposes in the workstation
	if !config.Get().Clients.RbacEnabled {
		return
	}
	ctx, cancel := context.WithCancel(ctx)

	e := echo.New()
	e.HideBanner = true
	e.Use(
		echo_middleware.Logger(),
		echo_middleware.Recover(),
	)
	e.Add(echo.GET, mocks_rbac.RbacV1Access, mocks_rbac.MockRbac)
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info().Msgf("mock rbac service starting")
		err := e.Start(":8800")
		if err != nil && err != http.ErrServerClosed {
			log.Fatal().Msgf("error starting mock rbac service: %s", err.Error())
		}
		log.Info().Msgf("mock rbac service stopped")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		defer cancel()
		log.Logger.Info().Msgf("stopping mock rbac service")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := e.Shutdown(ctx); err != nil {
			log.Fatal().Msgf("error shutting down mock rbac service: %s", err.Error())
		}
	}()
}
