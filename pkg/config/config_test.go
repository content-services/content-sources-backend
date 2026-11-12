package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

const MockCertData = "./test_files/cert.crt"

func TestImageBuilderUsernameConfig(t *testing.T) {
	t.Setenv("CLIENTS_PULP_IMAGE_BUILDER_USERNAME", "test-org|test-image-builder")
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	setDefaults(v)

	var loaded Configuration
	assert.NoError(t, v.Unmarshal(&loaded))
	assert.Equal(t, "test-org|test-image-builder", loaded.Clients.Pulp.ImageBuilderUsername)
	orgID, username, err := loaded.Clients.Pulp.ImageBuilderIdentity()
	assert.NoError(t, err)
	assert.Equal(t, "test-org", orgID)
	assert.Equal(t, "test-image-builder", username)
}

func TestImageBuilderIdentity(t *testing.T) {
	orgID, username, err := (Pulp{}).ImageBuilderIdentity()
	assert.NoError(t, err)
	assert.Empty(t, orgID)
	assert.Empty(t, username)

	for _, login := range []string{"no-separator", "org|", "|user", " |user", "org|\t", "org|user|extra"} {
		_, _, err := (Pulp{ImageBuilderUsername: login}).ImageBuilderIdentity()
		assert.Error(t, err, login)
	}
}

func TestConfigureCertificateFile(t *testing.T) {
	c := Get()
	c.Certs.CertPath = MockCertData
	os.Setenv(RhCertEnv, "")
	cert, strCert, err := ConfigureCertificate()
	assert.Nil(t, err)
	assert.NotNil(t, cert)
	assert.NotNil(t, strCert)

	days, err := DaysTillExpiration(cert)
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

func TestTermsServiceConfigured(t *testing.T) {
	c := Get()

	c.Clients.TermsService.Server = ""
	assert.False(t, TermsServiceConfigured())

	c.Clients.TermsService.Server = "https://terms.stage.api.redhat.com"
	assert.True(t, TermsServiceConfigured())

	// reset to avoid leaking state into other tests
	c.Clients.TermsService.Server = ""
}

func TestParseEventFeatureMap(t *testing.T) {
	assert.Equal(t, map[string]string{}, parseEventFeatureMap(""))
	assert.Equal(t, map[string]string{}, parseEventFeatureMap("bad-entry,also-bad"))
	assert.Equal(t, map[string]string{
		"network":  "lightwell-network",
		"academic": "lightwell-research-institutions",
	}, parseEventFeatureMap("network:lightwell-network,academic:lightwell-research-institutions"))
	assert.Equal(t, map[string]string{"network": "lightwell-network"}, parseEventFeatureMap("bad,network:lightwell-network"))

	// Deployed stage config uses JSON format — parser must handle it
	assert.Equal(t, map[string]string{
		"network":  "lightwell-network",
		"academic": "lightwell-research-institutions",
	}, parseEventFeatureMap(`{"network":"lightwell-network","academic":"lightwell-research-institutions"}`))
}

func TestTermsServiceCertUser(t *testing.T) {
	c := Get()
	c.Clients.TermsService.ClientCert = "test-cert"
	c.Clients.TermsService.ClientKey = "test-key"
	c.Clients.TermsService.CACert = "test-ca"
	c.Clients.TermsService.ClientCertPath = "/path/to/cert"
	c.Clients.TermsService.ClientKeyPath = "/path/to/key"
	c.Clients.TermsService.CACertPath = "/path/to/ca"

	certUser := &TermsServiceCertUser{}
	assert.Equal(t, "test-cert", certUser.ClientCert())
	assert.Equal(t, "test-key", certUser.ClientKey())
	assert.Equal(t, "test-ca", certUser.CACert())
	assert.Equal(t, "/path/to/cert", certUser.ClientCertPath())
	assert.Equal(t, "/path/to/key", certUser.ClientKeyPath())
	assert.Equal(t, "/path/to/ca", certUser.CACertPath())
	assert.Equal(t, "terms_service", certUser.Label())
	assert.Equal(t, "", certUser.Proxy())

	// reset to avoid leaking state into other tests
	c.Clients.TermsService = TermsService{}
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

	testCases := []TestCase{
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

func stubRbacV2Endpoint(t *testing.T, endpoints map[string]map[string]clowder.DependencyEndpointV2) {
	orig := clowder.DependencyEndpointsV2
	clowder.DependencyEndpointsV2 = endpoints
	t.Cleanup(func() { clowder.DependencyEndpointsV2 = orig })
}

func TestClowderRbacEndpoint(t *testing.T) {
	caPath := "/cdapp/certs/rbac-ca.pem"
	tests := []struct {
		name          string
		endpoint      clowder.DependencyEndpointV2
		expectedURL   string
		expectedCa    string
		authenticated bool
	}{
		{
			name:        "in-cluster, unauthenticated, no CA",
			endpoint:    clowder.DependencyEndpointV2{Uri: "http://rbac-service:8000"},
			expectedURL: "http://rbac-service:8000/api/rbac/v1",
		},
		{
			name:          "gateway, authenticated, CA, trailing slash",
			endpoint:      clowder.DependencyEndpointV2{Uri: "https://gw.example.com/", Authenticated: true, CaCertificate: &caPath},
			expectedURL:   "https://gw.example.com/api/rbac/v1",
			expectedCa:    caPath,
			authenticated: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubRbacV2Endpoint(t, map[string]map[string]clowder.DependencyEndpointV2{"rbac": {"service": tt.endpoint}})
			url, ca, authenticated, err := ClowderRbacEndpoint()
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedURL, url)
			assert.Equal(t, tt.expectedCa, ca)
			assert.Equal(t, tt.authenticated, authenticated)
		})
	}
}

func TestClowderRbacEndpointMissing(t *testing.T) {
	for name, endpoints := range map[string]map[string]map[string]clowder.DependencyEndpointV2{
		"no v2 endpoints": nil,
		"no rbac app":     {"other": {"service": {Uri: "http://other:8000"}}},
		"empty uri":       {"rbac": {"service": {Uri: ""}}},
	} {
		t.Run(name, func(t *testing.T) {
			stubRbacV2Endpoint(t, endpoints)
			url, _, _, err := ClowderRbacEndpoint()
			assert.ErrorContains(t, err, "rbac.service not found")
			assert.Empty(t, url)
		})
	}
}
