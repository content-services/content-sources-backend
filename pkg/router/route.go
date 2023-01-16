package router

import (
	"time"

	"github.com/content-services/content-sources-backend/pkg/client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/rs/zerolog/log"
	"github.com/ziflex/lecho/v3"
)

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

	if allRoutes {
		handler.RegisterRoutes(e)
	}

	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	return e
}

func ConfigureEchoWithMetrics(metrics *instrumentation.Metrics) *echo.Echo {
	e := ConfigureEcho(true)
	e.Use(middleware.CreateMetricsMiddleware(metrics))
	e.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipLiveness))
	if config.Get().Clients.RbacEnabled {
		rbacBaseUrl := config.Get().Clients.RbacBaseUrl
		rbacTimeout := time.Duration(int64(config.Get().Clients.RbacTimeout) * int64(time.Second))
		rbacClient := client.NewRbac(rbacBaseUrl, rbacTimeout)
		log.Info().Msgf("rbacBaseUrl=%s", rbacBaseUrl)
		log.Info().Msgf("rbacTimeout=%d secs", rbacTimeout/time.Second)
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
	return e
}
