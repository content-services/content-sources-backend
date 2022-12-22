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

// func startMetricsServer(ctx context.Context, metrics *Metrics) *echo.Echo {
// 	e := echo.New()
// 	metricsPath := "/metrics"
// 	e.Add(http.MethodGet, metricsPath, echo.WrapHandler(promhttp.HandlerFor(
// 		metrics.Registry(),
// 		promhttp.HandlerOpts{
// 			// Opt into OpenMetrics to support exemplars.
// 			EnableOpenMetrics: true,
// 			// Pass custom registry
// 			Registry: metrics.Registry(),
// 		},
// 	)))
// 	e.HideBanner = true

// 	go func() {
// 		if err := e.Start(":0"); err != nil && err != http.ErrServerClosed {
// 			log.Logger.Error().Msgf("error starting instrumentation: %s", err.Error())
// 		}
// 		log.Logger.Info().Msgf("instrumentation stopped")
// 	}()

// 	go func() {
// 		<-ctx.Done()
// 		shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
// 		if err := e.Shutdown(shutdownContext); err != nil {
// 			log.Logger.Error().Msgf("error stopping instrumentation: %s", err.Error())
// 		}
// 		cancel()
// 	}()

// 	return e
// }

// func getPort(addr net.Addr) string {
// 	items := strings.Split(addr.String(), ":")
// 	if len(items) == 0 {
// 		return ""
// 	}
// 	return items[len(items)-1]
// }

// func TestMetricsMiddlewareWithConfig(t *testing.T) {
// 	var (
// 		reg    *prometheus.Registry
// 		config *MetricsConfig
// 		m      echo.MiddlewareFunc
// 	)

// 	groupName := "/api/content-sources/v1"
// 	endpointName := "/repositories"

// 	type TestCaseGiven struct {
// 		Method  string
// 		Path    string
// 		Status  string
// 		RetCode int
// 	}
// 	type TestCase struct {
// 		Name     string
// 		Given    TestCaseGiven
// 		Expected string
// 	}

// 	testCases := []TestCase{
// 		{
// 			Name: "",
// 			Given: TestCaseGiven{
// 				Method: http.MethodGet,
// 				Path:   groupName + endpointName,
// 				Status: "2xx",
// 			},
// 			Expected: "http_status_histogram_count{method=\"GET\",path=\"" + groupName + endpointName + "\",status=\"2xx\"} 1",
// 		},
// 	}

// 	for _, testCase := range testCases {
// 		t.Log(testCase.Name)
// 		config = &MetricsConfig{
// 			Metrics: nil,
// 			Skipper: nil,
// 		}
// 		assert.Panics(t, func() {
// 			MetricsMiddlewareWithConfig(config)
// 		})

// 		reg = prometheus.NewRegistry()
// 		metrics := NewMetrics(reg)
// 		config = &MetricsConfig{
// 			Metrics: metrics,
// 			Skipper: func(c echo.Context) bool {
// 				if c.Path() == "/ping" {
// 					return true
// 				}
// 				return false
// 			},
// 		}
// 		m = MetricsMiddlewareWithConfig(config)
// 		h := func(c echo.Context) error {
// 			retCode := c.Request().URL.Query().Get("ret_code")
// 			ret, err := strconv.Atoi(retCode)
// 			if err == nil && ret >= 100 {
// 				return c.String(ret, http.StatusText(ret))
// 			}
// 			return c.String(http.StatusInternalServerError, "")
// 		}
// 		e := echo.New()
// 		e.Use(m)
// 		g := e.Group("/api/content-sources/v1")
// 		g.Add(http.MethodGet, "/repositories", h)

// 		rec := httptest.NewRecorder()
// 		e.ServeHTTP(
// 			rec,
// 			httptest.NewRequest(
// 				testCase.Given.Method,
// 				fmt.Sprintf("%s?ret_code=%d", testCase.Given.Path, testCase.Given.RetCode),
// 				http.NoBody,
// 			),
// 		)

// 		ctxMetrics, cancelMetrics := context.WithCancel(context.Background())
// 		eMetrics := startMetricsServer(ctxMetrics, metrics)
// 		require.NotNil(t, eMetrics)

// 		// See: https://github.com/prometheus/client_golang/blob/main/prometheus/testutil/testutil_test.go#L314
// 		url := "http://localhost:" + getPort(eMetrics.ListenerAddr()) + "/metrics"
// 		err := testutil.ScrapeAndCompare(
// 			url,
// 			strings.NewReader(fmt.Sprintf(`
// # HELP http_status_histogram http status histogram
// # TYPE http_status_histogram histogram
// http_status_histogram{method="GET",path="/api/content-sources/v1/repositories",status="2xx"} 1
// `)),
// 			"http_status_histogram")
// 		cancelMetrics()
// 		require.NoError(t, err)
// 	}
// }
