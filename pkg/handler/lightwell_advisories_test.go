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
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type LightwellAdvisorySuite struct {
	suite.Suite
	echo *echo.Echo
	reg  *dao.MockDaoRegistry
}

func TestLightwellAdvisorySuite(t *testing.T) {
	suite.Run(t, new(LightwellAdvisorySuite))
}

func (s *LightwellAdvisorySuite) SetupTest() {
	s.echo = echo.New()
	s.echo.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	s.echo.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	s.reg = dao.GetMockDaoRegistry(s.T())
}

func (s *LightwellAdvisorySuite) TearDownTest() {
	require.NoError(s.T(), s.echo.Shutdown(context.Background()))
}

func (s *LightwellAdvisorySuite) serveRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	pathPrefix := router.Group(api.FullRootPath())
	RegisterLightwellAdvisoryRoutes(pathPrefix, s.reg.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (s *LightwellAdvisorySuite) TestListAdvisories() {
	t := s.T()

	data := []api.LightwellAdvisoryResponse{
		{
			AdvisoryID:    "CVE-2024-1234",
			Severity:      "critical",
			Details:       "Remote code execution vulnerability",
			ReferenceURLs: []string{"https://access.redhat.com/security/cve/CVE-2024-1234"},
			PackageName:   "spring-core",
			FixedVersions: []string{"5.3.18.rhlw-00003"},
			Repository:    "lightwell/java/remediated",
		},
	}

	s.reg.LightwellAdvisory.On("ListAdvisories", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellAdvisoriesOptions) bool {
		return opts.Limit == int32(DefaultLimit) && opts.Offset == 0
	})).Return(data, int64(1), nil)

	path := fmt.Sprintf("%s/lightwell/advisories", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellAdvisoryCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(1), resp.Meta.Count)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "CVE-2024-1234", resp.Data[0].AdvisoryID)
	assert.Equal(t, "critical", resp.Data[0].Severity)
	assert.Equal(t, "spring-core", resp.Data[0].PackageName)
	assert.Equal(t, []string{"5.3.18.rhlw-00003"}, resp.Data[0].FixedVersions)
	assert.Equal(t, "lightwell/java/remediated", resp.Data[0].Repository)
}

func (s *LightwellAdvisorySuite) TestListAdvisoriesWithFilters() {
	t := s.T()

	s.reg.LightwellAdvisory.On("ListAdvisories", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellAdvisoriesOptions) bool {
		return opts.PackageName != nil && *opts.PackageName == "spring" &&
			opts.SeverityMin == "important" &&
			opts.Limit == 10 && opts.Offset == 5
	})).Return([]api.LightwellAdvisoryResponse{}, int64(0), nil)

	path := fmt.Sprintf("%s/lightwell/advisories?package_name=spring&severity_min=important&limit=10&offset=5", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellAdvisoryCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(0), resp.Meta.Count)
	assert.Empty(t, resp.Data)
}

func (s *LightwellAdvisorySuite) TestListAdvisoriesInvalidSeverity() {
	t := s.T()

	s.reg.LightwellAdvisory.On("ListAdvisories", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellAdvisoriesOptions) bool {
		return opts.SeverityMin == "bogus"
	})).Return(nil, int64(0), fmt.Errorf("invalid severity: bogus (must be one of: low, moderate, important, critical)"))

	path := fmt.Sprintf("%s/lightwell/advisories?severity_min=bogus", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (s *LightwellAdvisorySuite) TestListAdvisoriesFilterByRepoName() {
	t := s.T()

	s.reg.LightwellAdvisory.On("ListAdvisories", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellAdvisoriesOptions) bool {
		return opts.RepoName != nil && *opts.RepoName == "java-remediated"
	})).Return([]api.LightwellAdvisoryResponse{}, int64(0), nil)

	path := fmt.Sprintf("%s/lightwell/advisories?repository=java-remediated", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (s *LightwellAdvisorySuite) TestListAdvisoriesEmptyResult() {
	t := s.T()

	s.reg.LightwellAdvisory.On("ListAdvisories", test.MockCtx(), mock.Anything).
		Return([]api.LightwellAdvisoryResponse{}, int64(0), nil)

	path := fmt.Sprintf("%s/lightwell/advisories", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellAdvisoryCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(0), resp.Meta.Count)
	assert.NotNil(t, resp.Data)
	assert.Empty(t, resp.Data)
}
