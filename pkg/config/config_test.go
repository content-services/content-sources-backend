package config

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"
	identity "github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/stretchr/testify/assert"
)

const MockCertData = "./test_files/cert.crt"

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
		"/api/content_sources/v1.0/ping",
		"/api/content_sources/v1/ping",
	}
	e := ConfigureEcho()

	for _, route := range listRoutes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		res := httptest.NewRecorder()
		c := e.NewContext(req, res)

		result := SkipLiveness(c)
		assert.True(t, result)
	}
}

func TestSkipLivenessFalse(t *testing.T) {
	e := ConfigureEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/repositories", nil)
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	result := SkipLiveness(c)
	assert.False(t, result)
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

	// GET /api/content_sources/v1/repository_parameters/
	bodyResponse := "It Worded!"

	h = func(c echo.Context) error {
		body, err := []byte(bodyResponse), error(nil)
		if err != nil {
			return err
		}
		return c.String(http.StatusOK, string(body))
	}
	e.GET("/ping", h)
	e.GET("/api/content_sources/v1/ping", h)
	e.GET("/api/content_sources/v1.0/ping", h)
	e.GET("/api/content_sources/v1/repository_parameters/", h)

	// A Success request to /ping family path
	listSuccessPaths = []string{
		"/ping",
		"/api/content_sources/v1/ping",
		"/api/content_sources/v1.0/ping",
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
	req = httptest.NewRequest(http.MethodGet, "/api/content_sources/v1/repository_parameters/", nil)
	req.Header.Set(IdentityHeader, base64.StdEncoding.EncodeToString([]byte(xrhidentityHeaderFailure)))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err = m(h)(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "Bad Request: x-rh-identity header is missing type\n", rec.Body.String())

	// A Success request with a right header
	req = httptest.NewRequest(http.MethodGet, "/api/content_sources/v1/repository_parameters/", nil)
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
	listSuccessPaths = []string{"/ping", "/api/content_sources/v1/ping", "/api/content_sources/v1.0/ping"}
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
