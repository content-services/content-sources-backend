package jfrog_bridge

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setAdminFeatures(jfrog, sonatype config.Feature) {
	config.LoadedConfig.Features.AdminJfrogUpload = jfrog
	config.LoadedConfig.Features.AdminSonatypeUpload = sonatype
	config.LoadedConfig.Loaded = true
}

// newAdminHandlerForTest wires fake partners for jfrog and sonatype.
func newAdminHandlerForTest() *adminHandler {
	return &adminHandler{bridges: map[string]*BridgeHandler{
		jfrogPartnerName:    newTestBridgeHandler(&fakePartner{name: jfrogPartnerName, healthy: true}, nil),
		sonatypePartnerName: newTestBridgeHandler(&fakePartner{name: sonatypePartnerName}, nil),
	}}
}

// callPartnerMiddleware runs checkPartnerAccessible for partnerName and
// returns the error and the bridge handler that reached the route handler.
func callPartnerMiddleware(t *testing.T, h *adminHandler, partnerName string, withIdentity bool) (*BridgeHandler, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if withIdentity {
		req = req.WithContext(identity.WithIdentity(req.Context(), test_handler.MockIdentity))
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("partner")
	c.SetParamValues(partnerName)

	var reached *BridgeHandler
	err := h.checkPartnerAccessible(func(c echo.Context) error {
		reached = bridgeFromContext(c)
		return c.String(http.StatusOK, "ok")
	})(c)
	return reached, err
}

func errorStatus(t *testing.T, err error) int {
	t.Helper()
	var resp ce.ErrorResponse
	require.True(t, errors.As(err, &resp), "expected an ErrorResponse, got %v", err)
	require.NotEmpty(t, resp.Errors)
	return resp.Errors[0].Status
}

func TestCheckPartnerAccessible_UnknownPartner(t *testing.T) {
	setBridgeConfig(true, completeSonatypeBlock(true))
	setAdminFeatures(config.Feature{Enabled: true}, config.Feature{Enabled: true})

	reached, err := callPartnerMiddleware(t, newAdminHandlerForTest(), "artifactory", false)
	assert.Equal(t, http.StatusNotFound, errorStatus(t, err))
	assert.Nil(t, reached)
}

func TestCheckPartnerAccessible_DisabledPartner(t *testing.T) {
	setBridgeConfig(true, completeSonatypeBlock(false))
	setAdminFeatures(config.Feature{Enabled: true}, config.Feature{Enabled: true})

	_, err := callPartnerMiddleware(t, newAdminHandlerForTest(), sonatypePartnerName, false)
	assert.Equal(t, http.StatusNotFound, errorStatus(t, err))
}

func TestCheckPartnerAccessible_SonarSourceNotFound(t *testing.T) {
	setBridgeConfig(true, completeSonatypeBlock(true))
	setAdminFeatures(config.Feature{Enabled: true}, config.Feature{Enabled: true})

	h := newAdminHandlerForTest()
	h.bridges[sonarSourcePartnerName] = newTestBridgeHandler(sonarSourceHandler{}, nil)
	_, err := callPartnerMiddleware(t, h, sonarSourcePartnerName, false)
	assert.Equal(t, http.StatusNotFound, errorStatus(t, err))
}

func TestCheckPartnerAccessible_FeatureDisabled(t *testing.T) {
	setBridgeConfig(true, completeSonatypeBlock(true))
	// Each partner is gated only by its own admin feature flag.
	setAdminFeatures(config.Feature{Enabled: true}, config.Feature{Enabled: false})

	_, err := callPartnerMiddleware(t, newAdminHandlerForTest(), sonatypePartnerName, false)
	assert.Equal(t, http.StatusBadRequest, errorStatus(t, err))
	assert.Contains(t, err.Error(), "Sonatype")

	reached, err := callPartnerMiddleware(t, newAdminHandlerForTest(), jfrogPartnerName, false)
	require.NoError(t, err)
	assert.Equal(t, jfrogPartnerName, reached.partner.Name())

	setAdminFeatures(config.Feature{Enabled: false}, config.Feature{Enabled: true})
	_, err = callPartnerMiddleware(t, newAdminHandlerForTest(), jfrogPartnerName, false)
	assert.Equal(t, http.StatusBadRequest, errorStatus(t, err))
	assert.Contains(t, err.Error(), "JFrog")
}

func TestCheckPartnerAccessible_OpenAccess(t *testing.T) {
	setBridgeConfig(true, completeSonatypeBlock(true))
	setAdminFeatures(config.Feature{Enabled: true}, config.Feature{Enabled: true})

	for _, name := range []string{jfrogPartnerName, sonatypePartnerName} {
		reached, err := callPartnerMiddleware(t, newAdminHandlerForTest(), name, false)
		require.NoError(t, err, name)
		require.NotNil(t, reached, name)
		assert.Equal(t, name, reached.partner.Name())
	}
}

func TestCheckPartnerAccessible_UserACL(t *testing.T) {
	setBridgeConfig(true, completeSonatypeBlock(true))
	allowed := []string{test_handler.MockIdentity.Identity.User.Username}
	other := []string{"someone-else"}
	setAdminFeatures(
		config.Feature{Enabled: true, Users: &other},
		config.Feature{Enabled: true, Users: &allowed},
	)

	_, err := callPartnerMiddleware(t, newAdminHandlerForTest(), sonatypePartnerName, true)
	require.NoError(t, err)

	_, err = callPartnerMiddleware(t, newAdminHandlerForTest(), jfrogPartnerName, true)
	assert.Equal(t, http.StatusBadRequest, errorStatus(t, err))
}

func TestAdminStatus_DispatchesToPartner(t *testing.T) {
	h := newAdminHandlerForTest()
	e := echo.New()
	rec := httptest.NewRecorder()
	c := echoContextWithBridge(e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec), h.bridges[sonatypePartnerName])

	require.NoError(t, h.status(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"partner":"sonatype"}`, rec.Body.String())
}

func TestAdminHealth_StatusCode(t *testing.T) {
	h := newAdminHandlerForTest()
	tests := map[string]int{
		jfrogPartnerName:    http.StatusOK,                 // fake reports healthy
		sonatypePartnerName: http.StatusServiceUnavailable, // fake reports unhealthy
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			e := echo.New()
			rec := httptest.NewRecorder()
			c := echoContextWithBridge(e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec), h.bridges[name])

			require.NoError(t, h.health(c))
			assert.Equal(t, want, rec.Code)
			var body map[string]interface{}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Contains(t, body, "healthy")
		})
	}
}

func TestRegisterBridgeRoutes(t *testing.T) {
	setBridgeConfig(true, completeSonatypeBlock(true))

	e := echo.New()
	RegisterBridgeRoutes(e.Group("/api/content-sources/v1"))

	routes := map[string]bool{}
	for _, r := range e.Routes() {
		routes[r.Method+" "+r.Path] = true
	}
	for _, route := range []string{
		"POST /api/content-sources/v1/admin/bridge/:partner/simulate",
		"GET /api/content-sources/v1/admin/bridge/:partner/status",
		"GET /api/content-sources/v1/admin/bridge/:partner/health",
	} {
		assert.True(t, routes[route], "missing route %s", route)
	}
	for route := range routes {
		assert.NotContains(t, route, "jfrog_bridge")
		assert.NotContains(t, route, "sonatype_bridge")
	}

	res, verb, err := rbac.ServicePermissions.Permission(http.MethodPost, "/admin/bridge/:partner/simulate")
	require.NoError(t, err)
	assert.Equal(t, rbac.ResourceRepositories, res)
	assert.Equal(t, rbac.RbacVerbWrite, verb)
	_, verb, err = rbac.ServicePermissions.Permission(http.MethodGet, "/admin/bridge/:partner/status")
	require.NoError(t, err)
	assert.Equal(t, rbac.RbacVerbRead, verb)
}

func TestRegisterBridgeRoutes_NoPartnerEnabled(t *testing.T) {
	setBridgeConfig(false, completeSonatypeBlock(false))

	e := echo.New()
	RegisterBridgeRoutes(e.Group("/api/content-sources/v1"))
	assert.Empty(t, e.Routes())
}
