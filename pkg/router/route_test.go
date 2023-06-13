package router

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureEcho(t *testing.T) {
	type TestCaseExpected map[string]map[string]string

	testCases := TestCaseExpected{
		"/ping": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.ping",
		},
		"/api/content-sources/v1/openapi.json": {
			"GET": "github.com/content-services/content-sources-backend/pkg/handler.openapi",
		},
		"/api/content-sources/v1/repositories/": {
			"GET":  "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).listRepositories-fm",
			"POST": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).createRepository-fm",
		},
		"/api/content-sources/v1.0/repositories/": {
			"GET":  "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).listRepositories-fm",
			"POST": "github.com/content-services/content-sources-backend/pkg/handler.(*RepositoryHandler).createRepository-fm",
		},
	}

	e := ConfigureEcho(true)
	require.NotNil(t, e)

	for path, endpoints := range testCases {
		for method, fnc := range endpoints {
			found := false

			for _, route := range e.Routes() {
				if route.Path == path && method == route.Method {
					found = true
					assert.Equal(t, fnc, route.Name)
				}
			}
			assert.True(t, found, "Could not find route for %v: %v", method, path)
		}
	}
}

func TestEchoWithMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := instrumentation.NewMetrics(reg)
	var e *echo.Echo
	require.NotPanics(t, func() {
		e = ConfigureEchoWithMetrics(metrics)
	})
	assert.NotNil(t, e)
}
