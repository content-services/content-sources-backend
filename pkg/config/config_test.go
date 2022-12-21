package config

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	identity "github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const MockCertData = "./test_files/cert.crt"

const URLPrefix = "/api/" + DefaultAppName

func TestConfigureCertificateFile(t *testing.T) {
	c := Get()
	c.Certs.CertPath = MockCertData
	os.Setenv(RhCertEnv, "")
	cert, err := ConfigureCertificate()
	assert.Nil(t, err)
	assert.NotNil(t, cert)
}

func TestConfigureCertificateEnv(t *testing.T) {
	file, err := os.ReadFile(MockCertData)
	assert.Nil(t, err)
	os.Setenv(RhCertEnv, string(file))
	cert, err := ConfigureCertificate()
	assert.Nil(t, err)
	assert.NotNil(t, cert)
}

func TestBadCertsConfigureCertificate(t *testing.T) {
	c := Get()

	// Test bad path
	c.Certs.CertPath = "/tmp/foo"
	os.Setenv(RhCertEnv, "")
	cert, err := ConfigureCertificate()
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "no such file")

	// Test bad cert in env variable, should ignore path if set
	os.Setenv(RhCertEnv, "not a real cert")
	cert, err = ConfigureCertificate()
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "failed to find any PEM")
}

func TestNoCertConfigureCertificate(t *testing.T) {
	c := Get()
	os.Setenv(RhCertEnv, "")
	c.Certs.CertPath = ""
	cert, err := ConfigureCertificate()
	assert.Nil(t, cert)
	assert.Nil(t, err)
}

func TestSkipLivenessTrue(t *testing.T) {
	listRoutes := []string{
		"/ping",
		URLPrefix + "/v1.0/ping",
		URLPrefix + "/v1/ping",
	}
	metrics := instrumentation.NewMetrics(prometheus.NewRegistry())
	e := ConfigureEchoService(metrics)

	for _, route := range listRoutes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		res := httptest.NewRecorder()
		c := e.NewContext(req, res)

		result := SkipLiveness(c)
		assert.True(t, result)
	}
}

func TestSkipLivenessFalse(t *testing.T) {
	listRoutes := []string{
		"/api/v1/repositories",
		"/api/v1/repositories/ping",
	}
	metrics := instrumentation.NewMetrics(prometheus.NewRegistry())
	e := ConfigureEchoService(metrics)

	for _, route := range listRoutes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		res := httptest.NewRecorder()
		c := e.NewContext(req, res)

		result := SkipLiveness(c)
		assert.False(t, result)
	}
}

func TestWrapMiddlewareWithSkipper(t *testing.T) {
	var (
		req              *http.Request
		rec              *httptest.ResponseRecorder
		c                echo.Context
		h                func(c echo.Context) error
		err              error
		listSuccessPaths []string
	)
	e := echo.New()
	m := WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipLiveness)

	IdentityHeader := "X-Rh-Identity"
	xrhidentityHeaderSuccess := `{"identity":{"type":"Associate","account_number":"2093","internal":{"org_id":"7066"}}}`
	xrhidentityHeaderFailure := `{"identity":{"account_number":"2093","internal":{"org_id":"7066"}}}`

	bodyResponse := "It Worded!"

	h = func(c echo.Context) error {
		body, err := []byte(bodyResponse), error(nil)
		if err != nil {
			return err
		}
		return c.String(http.StatusOK, string(body))
	}
	e.GET("/ping", h)
	e.GET(URLPrefix+"/v1/ping", h)
	e.GET(URLPrefix+"/v1.0/ping", h)
	e.GET(URLPrefix+"/v1/repository_parameters/", h)

	// A Success request to /ping family path
	listSuccessPaths = []string{
		"/ping",
		URLPrefix + "/v1/ping",
		URLPrefix + "/v1.0/ping",
	}
	for _, path := range listSuccessPaths {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(IdentityHeader, base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderSuccess)))
		rec = httptest.NewRecorder()
		c = e.NewContext(req, rec)

		err = m(h)(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, bodyResponse, rec.Body.String())
	}

	// A Failed request with failed header
	req = httptest.NewRequest(http.MethodGet, URLPrefix+"/v1/repository_parameters/", nil)
	req.Header.Set(IdentityHeader, base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderFailure)))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err = m(h)(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "Bad Request: x-rh-identity header is missing type\n", rec.Body.String())

	// A Success request with a right header
	req = httptest.NewRequest(http.MethodGet, URLPrefix+"/v1/repository_parameters/", nil)
	req.Header.Set(IdentityHeader, base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderSuccess)))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err = m(h)(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, bodyResponse, rec.Body.String())
	encodedHeader := base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderSuccess))
	assert.Equal(t, encodedHeader, rec.Header().Get(IdentityHeader))

	// A Success request with failed header for /ping route
	// The middleware should skip for this route and call the
	// handler which fill the expected bodyResponse
	listSuccessPaths = []string{"/ping", URLPrefix + "/v1/ping", URLPrefix + "/v1.0/ping"}
	for _, path := range listSuccessPaths {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(IdentityHeader, base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderFailure)))
		rec = httptest.NewRecorder()
		c = e.NewContext(req, rec)

		err = m(h)(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, bodyResponse, rec.Body.String())
	}
}

func runTestCustomHTTPErrorHandler(t *testing.T, e *echo.Echo, method string, given error, expected string) {
	req := httptest.NewRequest(method, "/", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	CustomHTTPErrorHandler(given, c)
	if method == echo.HEAD {
		assert.Equal(t, "", rec.Body.String())
	} else {
		assert.Equal(t, expected, rec.Body.String())
	}
}

func TestCustomHTTPErrorHandler(t *testing.T) {
	type TestCase struct {
		Name     string
		Given    error
		Expected string
	}

	var testCases = []TestCase{
		{
			Name:     "ErrorResponse",
			Given:    errors.NewErrorResponse(http.StatusBadRequest, http.StatusText(http.StatusBadRequest), ""),
			Expected: "{\"errors\":[{\"status\":400,\"title\":\"Bad Request\"}]}\n",
		},
		{
			Name:     "echo.HTTPError",
			Given:    echo.NewHTTPError(http.StatusBadRequest, http.StatusText(http.StatusBadRequest)),
			Expected: "{\"errors\":[{\"status\":400,\"detail\":\"Bad Request\"}]}\n",
		},
		{
			Name:     "http.StatusInternalServerError",
			Given:    http.ErrAbortHandler,
			Expected: "{\"errors\":[{\"status\":500,\"detail\":\"Internal Server Error\"}]}\n",
		},
	}

	e := echo.New()
	for _, testCase := range testCases {
		for _, method := range []string{echo.GET, echo.HEAD} {
			t.Log(testCase.Name + ": " + method)
			runTestCustomHTTPErrorHandler(t, e, method, testCase.Given, testCase.Expected)
		}
	}
}

func TestCreateMetricsMiddleware(t *testing.T) {
	var (
		metrics    *instrumentation.Metrics
		middleware echo.MiddlewareFunc
	)
	metrics = instrumentation.NewMetrics(prometheus.NewRegistry())
	middleware = createMetricsMiddleware(metrics)

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
			Name:     "/repositories/validation resource",
			Given:    "/api/content-sources/v1/repositories/validation",
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

func TestConfigureEchoMetrics(t *testing.T) {
	var (
		metrics *instrumentation.Metrics
		e       *echo.Echo
	)
	metrics = instrumentation.NewMetrics(prometheus.NewRegistry())
	e = ConfigureEchoMetrics(metrics)
	require.NotNil(t, e)
}
