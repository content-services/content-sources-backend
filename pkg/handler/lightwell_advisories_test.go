package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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
	echo        *echo.Echo
	mockQuerier *MockQuerier
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
	s.mockQuerier = &MockQuerier{}
}

func (s *LightwellAdvisorySuite) TearDownTest() {
	require.NoError(s.T(), s.echo.Shutdown(context.Background()))
}

func (s *LightwellAdvisorySuite) serveRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	pathPrefix := router.Group(api.FullRootPath())
	RegisterLightwellAdvisoryRoutes(pathPrefix, s.mockQuerier)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (s *LightwellAdvisorySuite) TestListAdvisories() {
	t := s.T()

	repoUUID := uuid.New()
	rows := []store.ListAdvisoriesRow{
		{
			Uuid:                        uuid.New(),
			AdvisoryID:                  "CVE-2024-1234",
			Severity:                    "critical",
			SeverityOrder:               4,
			Details:                     "Remote code execution vulnerability",
			ReferenceUrls:               []string{"https://access.redhat.com/security/cve/CVE-2024-1234"},
			PackageName:                 "spring-core",
			FixedVersions:               []string{"5.3.18.rhlw-00003"},
			RepoName:                    "lightwell/java/remediated",
			RepositoryConfigurationUuid: repoUUID,
			CreatedAt:                   time.Now(),
			TotalCount:                  1,
		},
	}

	s.mockQuerier.On("ListAdvisories", mock.Anything, mock.MatchedBy(func(arg store.ListAdvisoriesParams) bool {
		return arg.PageLimit == int32(DefaultLimit) && arg.PageOffset == 0
	})).Return(rows, nil)

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

	s.mockQuerier.On("ListAdvisories", mock.Anything, mock.MatchedBy(func(arg store.ListAdvisoriesParams) bool {
		return arg.PackageName != nil && *arg.PackageName == "spring" &&
			arg.SeverityMin == pgtype.Int2{Int16: 3, Valid: true} &&
			arg.PageLimit == 10 && arg.PageOffset == 5
	})).Return([]store.ListAdvisoriesRow{}, nil)

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

	path := fmt.Sprintf("%s/lightwell/advisories?severity_min=bogus", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (s *LightwellAdvisorySuite) TestListAdvisoriesFilterByRepoName() {
	t := s.T()

	s.mockQuerier.On("ListAdvisories", mock.Anything, mock.MatchedBy(func(arg store.ListAdvisoriesParams) bool {
		return arg.RepoName != nil && *arg.RepoName == "java-remediated"
	})).Return([]store.ListAdvisoriesRow{}, nil)

	path := fmt.Sprintf("%s/lightwell/advisories?repository=java-remediated", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (s *LightwellAdvisorySuite) TestListAdvisoriesEmptyResult() {
	t := s.T()

	s.mockQuerier.On("ListAdvisories", mock.Anything, mock.Anything).Return([]store.ListAdvisoriesRow{}, nil)

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

// MockQuerier implements store.Querier for testing
type MockQuerier struct {
	mock.Mock
}

func (m *MockQuerier) ListAdvisories(ctx context.Context, arg store.ListAdvisoriesParams) ([]store.ListAdvisoriesRow, error) {
	args := m.Called(ctx, arg)
	val, _ := args.Get(0).([]store.ListAdvisoriesRow)
	return val, args.Error(1)
}

func (m *MockQuerier) CountAdvisoriesByRepo(ctx context.Context, repositoryConfigUuid uuid.UUID) (int64, error) {
	args := m.Called(ctx, repositoryConfigUuid)
	val, _ := args.Get(0).(int64)
	return val, args.Error(1)
}

func (m *MockQuerier) ListAdvisoriesByPackage(ctx context.Context, packageName string) ([]store.ListAdvisoriesByPackageRow, error) {
	args := m.Called(ctx, packageName)
	val, _ := args.Get(0).([]store.ListAdvisoriesByPackageRow)
	return val, args.Error(1)
}

func (m *MockQuerier) ListAdvisoriesByCveID(ctx context.Context, cveID string) ([]store.ListAdvisoriesByCveIDRow, error) {
	args := m.Called(ctx, cveID)
	val, _ := args.Get(0).([]store.ListAdvisoriesByCveIDRow)
	return val, args.Error(1)
}

func (m *MockQuerier) CountAggregates(ctx context.Context, arg store.CountAggregatesParams) (store.CountAggregatesRow, error) {
	args := m.Called(ctx, arg)
	val, _ := args.Get(0).(store.CountAggregatesRow)
	return val, args.Error(1)
}

func (m *MockQuerier) CountByStage(ctx context.Context, arg store.CountByStageParams) ([]store.CountByStageRow, error) {
	args := m.Called(ctx, arg)
	val, _ := args.Get(0).([]store.CountByStageRow)
	return val, args.Error(1)
}

func (m *MockQuerier) ListCustomerIds(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	val, _ := args.Get(0).([]string)
	return val, args.Error(1)
}

func (m *MockQuerier) ListVulnerabilities(ctx context.Context, arg store.ListVulnerabilitiesParams) ([]store.LightwellVulnerability, error) {
	args := m.Called(ctx, arg)
	val, _ := args.Get(0).([]store.LightwellVulnerability)
	return val, args.Error(1)
}
