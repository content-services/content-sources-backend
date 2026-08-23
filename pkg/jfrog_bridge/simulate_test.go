package jfrog_bridge

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckBridgeEnabled_Disabled(t *testing.T) {
	config.LoadedConfig.JFrogBridge = config.JFrogBridge{Enabled: false}
	config.LoadedConfig.Loaded = true

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := checkBridgeEnabled(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.Error(t, err)
}

func TestCheckBridgeEnabled_Enabled(t *testing.T) {
	config.LoadedConfig.JFrogBridge = config.JFrogBridge{Enabled: true}
	config.LoadedConfig.Loaded = true

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := checkBridgeEnabled(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSimulateHandler_Status(t *testing.T) {
	config.LoadedConfig.JFrogBridge = config.JFrogBridge{
		Enabled:    true,
		CatalogURL: "https://test.jfrog.io",
		CatalogRepo: "test-repo",
		RegistryURL: "https://test.registry",
		RegistryOSVURL: "https://test.osv",
	}
	config.LoadedConfig.Loaded = true

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/jfrog_bridge/status", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := &simulateHandler{}
	err := h.status(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "test.jfrog.io")
}

func TestSimulateHandler_BadPayload(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/jfrog_bridge/simulate",
		strings.NewReader(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := &simulateHandler{}
	err := h.simulate(c)
	require.Error(t, err)
}

func TestSimulate_SuccessPayload(t *testing.T) {
	config.LoadedConfig.JFrogBridge = config.JFrogBridge{Enabled: true}
	config.LoadedConfig.Loaded = true

	pomData, err := os.ReadFile(testdataPath("registry", "org", "springframework",
		"spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.pom"))
	require.NoError(t, err)

	jarData := pomData

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".jar") {
			w.Write(jarData)
		} else if strings.HasSuffix(r.URL.Path, ".pom") {
			w.Write(pomData)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer registryServer.Close()

	osvServer := httptest.NewServer(http.FileServer(http.Dir(testdataPath("osv"))))
	defer osvServer.Close()

	testKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusCreated)
		case http.MethodPost:
			if strings.Contains(r.URL.Path, "prepare") {
				resp := prepareResponse{
					DSSEPayload: base64.StdEncoding.EncodeToString([]byte("test-payload")),
					PAE:         "test-pae",
					PostURL:     "/evidence/api/v1/evidence/deploy",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			} else {
				resp := struct {
					Verified bool   `json:"verified"`
					ID       string `json:"id"`
				}{Verified: true, ID: "evd-123"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			}
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		}
	}))
	defer jfrogServer.Close()

	registry := &httpRegistryClient{
		httpClient:  registryServer.Client(),
		registryURL: registryServer.URL,
		osvURL:      osvServer.URL,
		maxRetries:  0,
	}
	jfrog := &httpJFrogClient{
		httpClient:  jfrogServer.Client(),
		catalogURL:  jfrogServer.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
		maxRetries:  0,
	}
	evidence := &evidenceCreator{
		httpClient:  jfrogServer.Client(),
		catalogURL:  jfrogServer.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
		privateKey:  testKey,
		keyAlias:    "redhat-partner-lightwell",
		maxRetries:  0,
	}

	metrics := newBridgeMetrics(prometheus.NewRegistry())
	bh := NewBridgeHandler(registry, jfrog, evidence, metrics)
	h := &simulateHandler{bridgeHandler: bh}

	payload := `{"package_name":"org.springframework:spring-core","releases":[{"name":"5.3.18.rhlw-00003","cves_fixed":["CVE-2025-41249"]}]}`
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/jfrog_bridge/simulate",
		strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = h.simulate(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "success")
}
