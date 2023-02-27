package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const URLPrefix = "/api/" + config.DefaultAppName

func TestCreateMetricsMiddleware(t *testing.T) {
	var (
		metrics    *instrumentation.Metrics
		middleware echo.MiddlewareFunc
	)
	metrics = instrumentation.NewMetrics(prometheus.NewRegistry())
	middleware = CreateMetricsMiddleware(metrics)

	assert.NotNil(t, middleware)
}

func TestMetricsMiddlewareSkipper(t *testing.T) {
	type TestCase struct {
		Name     string
		Given    string
		Expected bool
	}
	testCases := []TestCase{
		{
			Name:     "Empty",
			Given:    "/",
			Expected: false,
		},
		{
			Name:     "Ping",
			Given:    "/ping",
			Expected: true,
		},
		{
			Name:     "Ping with /",
			Given:    "/ping/",
			Expected: true,
		},
		{
			Name:     "Metrics",
			Given:    "/metrics",
			Expected: true,
		},
		{
			Name:     "Metrics with /",
			Given:    "/metrics/",
			Expected: true,
		},
		{
			Name:     "Ping as resource",
			Given:    "/api/content-sources/v1/ping",
			Expected: true,
		},
		{
			Name:     "Ping as resource with slash",
			Given:    "/api/content-sources/v1/ping/",
			Expected: true,
		},
		{
			Name:     "/repositories resource",
			Given:    "/api/content-sources/v1/repositories",
			Expected: false,
		},
		{
			Name:     "/repositories resource beta",
			Given:    "/beta/api/content-sources/v1/repositories",
			Expected: false,
		},
		{
			Name:     "/repository_parameters/validate resource",
			Given:    "/api/content-sources/v1/repository_parameters/validate",
			Expected: false,
		},
		{
			Name:     "/repository_parameters/validate resource for v1.0",
			Given:    "/api/content-sources/v1.0/repository_parameters/validate",
			Expected: false,
		},
	}
	for _, testCase := range testCases {
		t.Log(testCase.Name)
		ctx := echo.New().NewContext(
			httptest.NewRequest(http.MethodGet, testCase.Given, http.NoBody),
			httptest.NewRecorder())
		result := metricsMiddlewareSkipper(ctx)
		assert.Equal(t, testCase.Expected, result)
	}
}

func TestMapStatus(t *testing.T) {
	type TestCase struct {
		Name     string
		Given    int
		Expected string
	}
	testCases := []TestCase{
		{Name: "0", Given: 0, Expected: ""},
		{Name: "1xx", Given: http.StatusContinue, Expected: "1xx"},
		{Name: "2xx", Given: http.StatusOK, Expected: "2xx"},
		{Name: "3xx", Given: http.StatusMultipleChoices, Expected: "3xx"},
		{Name: "4xx", Given: http.StatusBadRequest, Expected: "4xx"},
		{Name: "5xx", Given: http.StatusInternalServerError, Expected: "5xx"},
	}

	for _, testCase := range testCases {
		result := mapStatus(testCase.Given)
		assert.Equal(t, testCase.Expected, result)
	}
}

func TestMetricsMiddlewareWithConfigCreation(t *testing.T) {
	var (
		reg    *prometheus.Registry
		config *MetricsConfig
	)

	config = &MetricsConfig{
		Metrics: nil,
		Skipper: nil,
	}
	assert.Panics(t, func() {
		MetricsMiddlewareWithConfig(config)
	})

	reg = prometheus.NewRegistry()
	config = &MetricsConfig{
		Metrics: instrumentation.NewMetrics(reg),
		Skipper: func(c echo.Context) bool {
			return c.Path() == "/ping"
		},
	}

	require.NotPanics(t, func() {
		MetricsMiddlewareWithConfig(config)
	})

	assert.NotPanics(t, func() {
		MetricsMiddlewareWithConfig(nil)
	})

	h := func(c echo.Context) error {
		return c.String(http.StatusOK, "Ok")
	}

	e := echo.New()
	m := MetricsMiddlewareWithConfig(config)
	e.Use(m)
	path := "/api/content-sources/v1/repositories/"
	e.Add(http.MethodGet, path, h)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	e.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "Ok", resp.Body.String())
}
