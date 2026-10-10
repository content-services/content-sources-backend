package lightwell

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type LightwellTermsSuite struct {
	LightwellSuite
}

func TestLightwellTermsSuite(t *testing.T) {
	suite.Run(t, new(LightwellTermsSuite))
}

func (s *LightwellTermsSuite) SetupTest() {
	s.LightwellSuite.SetupTest()
	s.tsClient = terms_service_client.NewMockTermsServiceClient(s.T())

	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Features.LightwellTerms = config.Feature{Enabled: true}
	config.LoadedConfig.Clients.TermsService.EventFeatureMap = map[string]string{
		"network":  "lightwell-network",
		"academic": "lightwell-research-institutions",
	}
}

// --- Route registration ---

func (s *LightwellTermsSuite) TestTermsRequiredRoute() {
	t := s.T()

	s.tsClient.On("GetRequiredEvents", test.MockCtx(), "user").
		Return([]string{}, nil)

	path := fmt.Sprintf("%s/terms/required", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	assert.Nil(t, err)
	assert.NotEqual(t, http.StatusNotFound, code, "Route should be registered")
}

// --- GetTermsRequired ---

func (s *LightwellTermsSuite) TestGetTermsRequired_Required() {
	t := s.T()

	s.tsClient.On("GetRequiredEvents", test.MockCtx(), "user").
		Return([]string{"network", "academic"}, nil)
	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{"lightwell-network", "lightwell-research-institutions"}, nil)

	path := fmt.Sprintf("%s/terms/required", LightwellAPIPath)
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

	s.tsClient.On("GetRequiredEvents", test.MockCtx(), "user").
		Return([]string{}, nil)

	path := fmt.Sprintf("%s/terms/required", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.TermsRequiredResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.False(t, resp.Required)
}

func (s *LightwellTermsSuite) TestGetTermsRequired_FilteredByEntitlement() {
	t := s.T()

	s.tsClient.On("GetRequiredEvents", test.MockCtx(), "user").
		Return([]string{"network", "academic"}, nil)
	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{"lightwell-network"}, nil)

	path := fmt.Sprintf("%s/terms/required", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.TermsRequiredResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.True(t, resp.Required)
	assert.Equal(t, []string{"network"}, resp.Events, "should only return network for network-entitled org")
}

func (s *LightwellTermsSuite) TestGetTermsRequired_FeatureDisabled() {
	t := s.T()

	config.LoadedConfig.Features.LightwellTerms = config.Feature{Enabled: false}

	path := fmt.Sprintf("%s/terms/required", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.TermsRequiredResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.False(t, resp.Required)
}

func (s *LightwellTermsSuite) TestGetTermsRequired_ClientError_FailsOpen() {
	t := s.T()

	s.tsClient.On("GetRequiredEvents", test.MockCtx(), "user").
		Return([]string(nil), fmt.Errorf("connection refused"))

	path := fmt.Sprintf("%s/terms/required", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.TermsRequiredResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.False(t, resp.Required, "should fail open when terms service is unavailable")
}
