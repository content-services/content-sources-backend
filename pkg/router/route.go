package router

import (
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	echo_log "github.com/labstack/gommon/log"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
	"github.com/ziflex/lecho/v3"
)

func ConfigureEcho(allRoutes bool) *echo.Echo {
	e := echo.New()
	// Add global middlewares
	echoLogger := lecho.From(log.Logger,
		lecho.WithTimestamp(),
		lecho.WithCaller(),
		lecho.WithLevel(echo_log.INFO),
	)
	e.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: config.HeaderRequestId,
	}))
	e.Use(middleware.AddRequestId)
	e.Use(lecho.Middleware(lecho.Config{
		Logger:          echoLogger,
		RequestIDHeader: config.HeaderRequestId,
		RequestIDKey:    config.RequestIdLoggingKey,
		Skipper:         config.SkipLogging,
	}))
	e.Use(middleware.EnforceJSONContentType)

	// Add routes
	handler.RegisterPing(e)
	if allRoutes {
		handler.RegisterRoutes(e)
	}

	// Set error handler
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	return e
}

func ConfigureEchoWithMetrics(metrics *instrumentation.Metrics) *echo.Echo {
	e := ConfigureEcho(true)

	// Add additional global middlewares
	e.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))
	e.Use(middleware.EnforceOrgId)
	e.Use(middleware.CreateMetricsMiddleware(metrics))
	if config.Get().Clients.RbacEnabled {
		rbacBaseUrl := config.Get().Clients.RbacBaseUrl
		rbacTimeout := time.Duration(int64(config.Get().Clients.RbacTimeout) * int64(time.Second))
		rbacClient := rbac.NewClientWrapperImpl(rbacBaseUrl, rbacTimeout)
		log.Info().Msgf("rbacBaseUrl=%s", rbacBaseUrl)
		log.Info().Msgf("rbacTimeout=%d secs", rbacTimeout/time.Second)
		e.Use(
			middleware.NewRbac(
				middleware.Rbac{
					BaseUrl:        config.Get().Clients.RbacBaseUrl,
					Skipper:        middleware.SkipAuth,
					PermissionsMap: rbac.ServicePermissions,
					Client:         rbacClient,
				},
			),
		)
	}
	return e
}
