package router

import (
	"context"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/lecho/v3"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func ConfigureEcho(ctx context.Context, allRoutes bool) *echo.Echo {
	e := echo.New()
	// Add global middlewares
	echoLogger := lecho.From(log.Logger,
		lecho.WithTimestamp(),
		lecho.WithCaller(),
	)

	e.Use(middleware.AddRequestId)
	e.Use(lecho.Middleware(lecho.Config{
		Logger:              echoLogger,
		RequestIDHeader:     config.HeaderRequestId,
		RequestIDKey:        config.RequestIdLoggingKey,
		Skipper:             config.SkipLogging,
		RequestLatencyLevel: zerolog.WarnLevel,
		RequestLatencyLimit: 500 * time.Millisecond,
	}))
	e.Use(middleware.ExtractStatus) // Must be after lecho
	e.Use(middleware.EnforceJSONContentType)
	e.Use(middleware.LogServerErrorRequest)

	// Add routes
	handler.RegisterPing(e)
	if allRoutes {
		handler.RegisterRoutes(ctx, e)
	}

	// Set error handler
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	return e
}

func ConfigureEchoWithMetrics(ctx context.Context, metrics *instrumentation.Metrics) *echo.Echo {
	e := ConfigureEcho(ctx, true)

	// Add additional global middlewares
	e.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
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
					Skipper:        middleware.SkipMiddleware,
					PermissionsMap: rbac.ServicePermissions,
					Client:         rbacClient,
				},
			),
		)
	}
	return e
}
