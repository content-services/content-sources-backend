package instrumentation

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			if c.Path() == "/ping" {
				return true
			}
			return false
		},
	}

	assert.NotPanics(t, func() {
		MetricsMiddlewareWithConfig(config)
	})

	assert.NotPanics(t, func() {
		MetricsMiddlewareWithConfig(nil)
	})
}

func TestMetricsMiddlewareWithConfig(t *testing.T) {
	var (
		reg    *prometheus.Registry
		config *MetricsConfig
		m      echo.MiddlewareFunc
	)

	type TestCase struct {
		Name     string
		Given    int
		Expected string
	}

	testCases := []TestCase{
		{
			Name:     "",
			Given:    200,
			Expected: "",
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
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
				if c.Path() == "/ping" {
					return true
				}
				return false
			},
		}
		m = MetricsMiddlewareWithConfig(config)
		h := func(c echo.Context) error {
			retCode := c.Request().URL.Query().Get("ret_code")
			ret, err := strconv.Atoi(retCode)
			if err == nil && ret >= 100 {
				return c.String(ret, http.StatusText(ret))
			}
			return c.String(http.StatusInternalServerError, "")
		}
		e := echo.New()
		e.Use(m)
		g := e.Group("/api/content-sources/v1")
		g.Add(http.MethodGet, "/repositories", h)

		rec := httptest.NewRecorder()
		e.ServeHTTP(
			rec,
			httptest.NewRequest(
				http.MethodGet,
				fmt.Sprintf("/api/content-sources/v1/repositories?ret_code=%d", testCase.Given),
				http.NoBody,
			),
		)

		mf, err := reg.Gather()
		require.NoError(t, err)
		for _, mfi := range mf {
			if mfi.GetName() == "http_status_histogram" {
				require.NotNil(t, mfi.Help)
				assert.Equal(t, "http status histogram", *mfi.Help)
				assert.Equal(t, "HISTOGRAM", mfi.Type.String())
				for _, mfi2 := range mfi.Metric {
					assert.Equal(t, float64(1.0), *mfi2.GetGauge().Value)

				}
				assert.Equal(t, "", mfi.String())
			}
		}
	}
}
