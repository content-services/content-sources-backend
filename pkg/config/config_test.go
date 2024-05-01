package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	"github.com/stretchr/testify/assert"
)

const MockCertData = "./test_files/cert.crt"

func TestConfigureCertificateFile(t *testing.T) {
	c := Get()
	c.Certs.CertPath = MockCertData
	os.Setenv(RhCertEnv, "")
	cert, strCert, err := ConfigureCertificate()
	assert.Nil(t, err)
	assert.NotNil(t, cert)
	assert.NotNil(t, strCert)

	days, err := daysTillExpiration(cert)
	assert.NoError(t, err)
	assert.True(t, days > 0)
}

func TestConfigureCertificateEnv(t *testing.T) {
	file, err := os.ReadFile(MockCertData)
	assert.Nil(t, err)
	os.Setenv(RhCertEnv, string(file))
	cert, strCert, err := ConfigureCertificate()
	assert.Nil(t, err)
	assert.NotNil(t, cert)
	assert.NotNil(t, strCert)
}

func TestBadCertsConfigureCertificate(t *testing.T) {
	c := Get()

	// Test bad path
	c.Certs.CertPath = "/tmp/foo"
	os.Setenv(RhCertEnv, "")
	cert, strCert, err := ConfigureCertificate()
	assert.Nil(t, cert)
	assert.Nil(t, strCert)
	assert.Contains(t, err.Error(), "no such file")

	// Test bad cert in env variable, should ignore path if set
	os.Setenv(RhCertEnv, "not a real cert")
	cert, strCert, err = ConfigureCertificate()
	assert.Nil(t, cert)
	assert.Nil(t, strCert)
	assert.Contains(t, err.Error(), "failed to find any PEM")
}

func TestNoCertConfigureCertificate(t *testing.T) {
	c := Get()
	os.Setenv(RhCertEnv, "")
	c.Certs.CertPath = ""
	cert, strCert, err := ConfigureCertificate()
	assert.Nil(t, cert)
	assert.Nil(t, strCert)
	assert.Nil(t, err)
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

func TestClowderS3URL(t *testing.T) {
	assert.Equal(t, "http://foo:80", ClowderS3Url(clowder.ObjectStoreConfig{
		Hostname: "foo",
		Port:     80,
		Tls:      false,
	}))
	assert.Equal(t, "https://foo:30", ClowderS3Url(clowder.ObjectStoreConfig{
		Hostname: "foo",
		Port:     30,
		Tls:      true,
	}))
	assert.Equal(t, "https://foo:30", ClowderS3Url(clowder.ObjectStoreConfig{
		Hostname: "https://foo",
		Port:     30,
		Tls:      true,
	}))
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
