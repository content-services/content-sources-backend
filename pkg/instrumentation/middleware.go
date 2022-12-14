package instrumentation

import (
	"time"

	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

type MetricsConfig struct {
	Skipper echo_middleware.Skipper
	Metrics *Metrics
}

var defaultConfig MetricsConfig = MetricsConfig{
	Skipper: echo_middleware.DefaultSkipper,
	Metrics: NewMetrics(prometheus.NewRegistry()),
}

// https://github.com/labstack/echo/pull/1502/files
// This method exist for v5 echo framework
func matchedRoute(ctx echo.Context) string {
	pathx := ctx.Path()
	for _, r := range ctx.Echo().Routes() {
		if pathx == r.Path {
			return r.Path
		}
	}
	return ""
}

func MetricsMiddlewareWithConfig(config *MetricsConfig) echo.MiddlewareFunc {
	if config == nil {
		config = &defaultConfig
	}
	if config.Skipper == nil {
		config.Skipper = echo_middleware.DefaultSkipper
	}
	if config.Metrics == nil {
		panic("config.Metrics can not be nil")
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			start := time.Now()
			method := ctx.Request().Method
			path := matchedRoute(ctx)

			config.Metrics.HttpTotalRequests.Inc()
			err := next(ctx)
			if ctx.Response().Status >= 500 {
				config.Metrics.HttpTotalFailedRequests.Inc()
			}
			config.Metrics.HttpRequestLatency.WithLabelValues(method, path).Observe(float64(time.Since(start).Nanoseconds()) / 1000000)
			return err
		}
	}
}
