package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	fsc "github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestLightwellBearerAuthSuccess(t *testing.T) {
	reg := dao.GetMockDaoRegistry(t)
	fs := fsc.NewMockFeatureServiceClient(t)
	config.Get().Options.FeatureFilter = []string{config.LightwellNetworkFeature}

	orgID := "org-123"
	userID := "user-9"
	reg.LightwellToken.On("Validate", mock.Anything, "lw_good").Return(api.LightwellTokenValidateResponse{
		OrgID: orgID, UserID: userID, TokenUUID: "tok-1",
	}, nil)
	fs.On("GetEntitledFeatures", mock.Anything, orgID).Return([]string{config.LightwellNetworkFeature}, nil)

	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(LightwellBearerAuth(reg.ToDaoRegistry(), fs))
	e.GET("/api/content-sources/v1.0/repositories/:uuid/packages", func(c echo.Context) error {
		id := identity.GetIdentity(c.Request().Context())
		assert.Equal(t, orgID, id.Identity.Internal.OrgID)
		assert.Equal(t, userID, id.Identity.User.UserID)
		assert.True(t, HasLightwellBearerAuth(c))
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1.0/repositories/abc/packages", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer lw_good")
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestLightwellBearerAuthMissingEntitlement(t *testing.T) {
	reg := dao.GetMockDaoRegistry(t)
	fs := fsc.NewMockFeatureServiceClient(t)

	orgID := "org-123"
	reg.LightwellToken.On("Validate", mock.Anything, "lw_good").Return(api.LightwellTokenValidateResponse{
		OrgID: orgID, UserID: "u", TokenUUID: "tok-1",
	}, nil)
	fs.On("GetEntitledFeatures", mock.Anything, orgID).Return([]string{"RHEL-OS-x86_64"}, nil)

	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(LightwellBearerAuth(reg.ToDaoRegistry(), fs))
	e.GET("/api/content-sources/v1.0/repositories/:uuid/packages", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1.0/repositories/abc/packages", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer lw_good")
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestLightwellBearerAuthInvalidToken(t *testing.T) {
	reg := dao.GetMockDaoRegistry(t)
	fs := fsc.NewMockFeatureServiceClient(t)

	reg.LightwellToken.On("Validate", mock.Anything, "lw_bad").Return(api.LightwellTokenValidateResponse{}, assert.AnError)

	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(LightwellBearerAuth(reg.ToDaoRegistry(), fs))
	e.GET("/api/content-sources/v1.0/repositories/:uuid/packages", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1.0/repositories/abc/packages", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer lw_bad")
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	body, err := io.ReadAll(rr.Body)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
}

func TestLightwellBearerAuthIgnoresNonLightwellBearer(t *testing.T) {
	reg := dao.GetMockDaoRegistry(t)
	fs := fsc.NewMockFeatureServiceClient(t)

	e := echo.New()
	e.HTTPErrorHandler = config.CustomHTTPErrorHandler
	e.Use(LightwellBearerAuth(reg.ToDaoRegistry(), fs))
	e.GET("/api/content-sources/v1.0/repositories/:uuid/packages", func(c echo.Context) error {
		assert.False(t, HasLightwellBearerAuth(c))
		return c.NoContent(http.StatusOK)
	})

	// Console SSO JWTs are Bearer but do not use the lw_ prefix.
	req := httptest.NewRequest(http.MethodGet, "/api/content-sources/v1.0/repositories/abc/packages", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer eyJhbGciOiJSUzI1NiJ9.sso-jwt")
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	reg.LightwellToken.AssertNotCalled(t, "Validate", mock.Anything, mock.Anything)
}
