package jfrog_bridge

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
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
		Enabled:        true,
		CatalogURL:     "https://test.jfrog.io",
		CatalogRepo:    "test-repo",
		RegistryURL:    "https://test.registry",
		RegistryOSVURL: "https://test.osv",
	}
	config.LoadedConfig.Loaded = true

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/jfrog_bridge/status", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	h := &adminHandler{}
	err := h.status(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "test.jfrog.io")
}

func TestCheckAdminJfrogUploadAccessible_Disabled(t *testing.T) {
	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Features.AdminJfrogUpload = config.Feature{Enabled: false}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := checkAdminJfrogUploadAccessible(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.Error(t, err)
}

func TestCheckAdminJfrogUploadAccessible_OpenAccess(t *testing.T) {
	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Features.AdminJfrogUpload = config.Feature{
		Enabled: true,
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := checkAdminJfrogUploadAccessible(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCheckAdminJfrogUploadAccessible_UserInACL(t *testing.T) {
	users := []string{test_handler.MockIdentity.Identity.User.Username}
	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Features.AdminJfrogUpload = config.Feature{
		Enabled: true,
		Users:   &users,
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := identity.WithIdentity(req.Context(), test_handler.MockIdentity)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := checkAdminJfrogUploadAccessible(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}
