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

	"github.com/content-services/content-sources-backend/pkg/client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/event"
	eventHandler "github.com/content-services/content-sources-backend/pkg/event/handler"
	"github.com/content-services/content-sources-backend/pkg/handler"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	custom_collector "github.com/content-services/content-sources-backend/pkg/instrumentation/custom"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/rs/zerolog/log"
	"github.com/ziflex/lecho/v3"
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

	echo := config.ConfigureEchoWithMetrics(metrics)
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
	e := ConfigureEcho(false)

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
	custom_ctx, custom_cancel := context.WithCancel(ctx)
	custom := custom_collector.NewCollector(custom_ctx, metrics, db.DB)
	go func() {
		defer wg.Done()
		log.Logger.Info().Msgf("Starting custom metrics go routine")
		custom.Run()
		log.Logger.Info().Msgf("custom metrics stopped")
	}()

	go func() {
		<-custom_ctx.Done()
		custom_cancel()
	}()
}

func ConfigureEcho(allRoutes bool) *echo.Echo {
	e := echo.New()
	echoLogger := lecho.From(log.Logger,
		lecho.WithTimestamp(),
		lecho.WithCaller(),
	)
	e.Use(echo_middleware.AddTrailingSlash(), echo_middleware.Logger())
	e.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	e.Use(lecho.Middleware(lecho.Config{
		Logger:       echoLogger,
		RequestIDKey: "x-rh-insights-request-id",
	}))

	// Liveness and readiness are set up before security checks
	handler.RegisterPing(e)

	e.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipLiveness))
	if config.Get().Clients.RbacEnabled {
		rbacBaseUrl := config.Get().Clients.RbacBaseUrl
		rbacTimeout := time.Duration(int64(config.Get().Clients.RbacTimeout) * int64(time.Second))
		rbacClient := client.NewRbac(rbacBaseUrl, rbacTimeout)
		log.Info().Msgf("RBAC:rbacBaseUrl=%s", rbacBaseUrl)
		log.Info().Msgf("RBAC:rbacTimeout=%d", rbacTimeout)
		log.Info().Msgf("RBAC:rbacTimeout=%d secs", rbacTimeout/time.Second)
		e.Use(
			middleware.NewRbac(
				middleware.Rbac{
					BaseUrl:        config.Get().Clients.RbacBaseUrl,
					Skipper:        middleware.SkipLiveness,
					PermissionsMap: middleware.ServicePermissions,
					Client:         rbacClient,
				},
			),
		)
	}

	if allRoutes {
		handler.RegisterRoutes(e)
	}
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	return e
}
