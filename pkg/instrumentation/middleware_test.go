package instrumentation

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestMatchedRoute(t *testing.T) {
	type TestCase struct {
		Name     string
		Given    string
		Expected string
	}
	testCases := []TestCase{
		{
			Name:     "no matching /",
			Given:    "/",
			Expected: "{\"message\":\"Not Found\"}\n",
		},
		{
			Name:     "matching /api/test",
			Given:    "/api/test",
			Expected: "/api/test",
		},
		{
			Name:     "matching /api/test/:id",
			Given:    "/api/test/12345",
			Expected: "/api/test/:id",
		},
		{
			Name:     "no matching /anything",
			Given:    "/api/anything",
			Expected: "{\"message\":\"Not Found\"}\n",
		},
		{
			Name:     "no matching /test/",
			Given:    "/api/test/",
			Expected: "{\"message\":\"Not Found\"}\n",
		},
	}
	h := func(c echo.Context) error {
		// The context.Path() is filled during serving the request,
		// so it is not enough create the context and call to matchedRoute
		match := matchedRoute(c)
		return c.String(http.StatusOK, match)
	}
	for _, testCase := range testCases {
		t.Log(testCase.Name)
		e := echo.New()
		g := e.Group("/api")
		g.Add(http.MethodGet, "/test", h)
		g.Add(http.MethodGet, "/test/:id", h)

		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, testCase.Given, http.NoBody))
		assert.Equal(t, testCase.Expected, rec.Body.String())
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
		Metrics: NewMetrics(reg),
		Skipper: func(c echo.Context) bool {
			return c.Path() == "/ping"
		},
	}

	assert.NotPanics(t, func() {
		MetricsMiddlewareWithConfig(config)
	})

	assert.NotPanics(t, func() {
		MetricsMiddlewareWithConfig(nil)
	})
}
