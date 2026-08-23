package jfrog_bridge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/labstack/echo/v4"
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
