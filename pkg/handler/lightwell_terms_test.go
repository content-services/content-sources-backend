package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type LightwellTermsSuite struct {
	suite.Suite
	echo   *echo.Echo
	tsMock *terms_service_client.MockTermsServiceClient
}

func TestLightwellTermsSuite(t *testing.T) {
	suite.Run(t, new(LightwellTermsSuite))
}

func (s *LightwellTermsSuite) SetupTest() {
	s.echo = echo.New()
	s.echo.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	s.echo.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	s.tsMock = terms_service_client.NewMockTermsServiceClient(s.T())

	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Features.LightwellTerms = config.Feature{Enabled: true}
}

func (s *LightwellTermsSuite) TearDownTest() {
	require.NoError(s.T(), s.echo.Shutdown(context.Background()))
}

func (s *LightwellTermsSuite) serveRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	pathPrefix := router.Group(api.FullRootPath())
	var tsClient terms_service_client.TermsServiceClient = s.tsMock
	RegisterLightwellTermsRoutes(pathPrefix, &tsClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

// --- GetTermsRequired tests ---

func (s *LightwellTermsSuite) TestGetTermsRequired_Required() {
	t := s.T()

	s.tsMock.On("IsTermsAcceptanceRequired", test.MockCtx(), "user").
		Return(true, nil)

	path := fmt.Sprintf("%s/lightwell/terms/required", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.TermsRequiredResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.True(t, resp.Required)
}

func (s *LightwellTermsSuite) TestGetTermsRequired_NotRequired() {
	t := s.T()

	s.tsMock.On("IsTermsAcceptanceRequired", test.MockCtx(), "user").
		Return(false, nil)

	path := fmt.Sprintf("%s/lightwell/terms/required", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.TermsRequiredResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.False(t, resp.Required)
}

func (s *LightwellTermsSuite) TestGetTermsRequired_FeatureDisabled() {
	t := s.T()

	config.LoadedConfig.Features.LightwellTerms = config.Feature{Enabled: false}

	path := fmt.Sprintf("%s/lightwell/terms/required", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.TermsRequiredResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.False(t, resp.Required)
}

func (s *LightwellTermsSuite) TestGetTermsRequired_ClientError() {
	t := s.T()

	s.tsMock.On("IsTermsAcceptanceRequired", test.MockCtx(), "user").
		Return(false, fmt.Errorf("connection refused"))

	path := fmt.Sprintf("%s/lightwell/terms/required", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}
