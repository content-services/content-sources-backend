package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type LightwellPackagesSuite struct {
	suite.Suite
	reg        *dao.MockDaoRegistry
	tangClient *tangy.MockTangy
	pulpClient *pulp_client.MockPulpClient
	fsClient   *feature_service_client.MockFeatureServiceClient
}

func TestLightwellPackagesSuite(t *testing.T) {
	suite.Run(t, new(LightwellPackagesSuite))
}

func (s *LightwellPackagesSuite) SetupTest() {
	s.reg = dao.GetMockDaoRegistry(s.T())
	s.tangClient = tangy.NewMockTangy(s.T())
	s.pulpClient = pulp_client.NewMockPulpClient(s.T())
	s.fsClient = feature_service_client.NewMockFeatureServiceClient(s.T())
}

func (s *LightwellPackagesSuite) serveRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	pathPrefix := router.Group(api.FullRootPath())
	var fsClient feature_service_client.FeatureServiceClient = s.fsClient
	RegisterLightwellPackageRoutes(pathPrefix, s.reg.ToDaoRegistry(), s.tangClient, s.pulpClient, &fsClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (s *LightwellPackagesSuite) stubLightwellAccess() {
	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{"lightwell-network"}, nil).Maybe()
}

// --- /lightwell/packages tests ---

func (s *LightwellPackagesSuite) TestListPackages() {
	t := s.T()
	s.stubLightwellAccess()

	daoRows := []dao.LightwellPackageRow{
		{
			RepositoryConfigurationUUID: "repo-uuid-1",
			RepositoryName:              "lightwell/maven/remediated",
			Ecosystem:                   config.ContentTypeMaven,
			Name:                        "jackson-databind",
			Group:                       "com.fasterxml.jackson.core",
			Versions:                    []string{"2.15.3", "2.14.2"},
			Releases:                    []string{"rhlw-00001", "rhlw-00001"},
			PublishedAts:                []string{"2024-06-01T12:00:00Z", "2024-05-01T12:00:00Z"},
			TotalCount:                  1,
		},
	}

	s.reg.LightwellPackage.On("ListPackages", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellPackagesOptions) bool {
		return opts.Limit == int32(DefaultLimit) && opts.Offset == 0 &&
			len(opts.EntitledFeatures) == 1 && opts.EntitledFeatures[0] == "lightwell-network"
	})).Return(daoRows, int64(1), nil)

	path := fmt.Sprintf("%s/lightwell/packages", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(1), resp.Meta.Count)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "jackson-databind", resp.Data[0].Name)
	assert.Equal(t, "com.fasterxml.jackson.core", resp.Data[0].Group)
	assert.Equal(t, config.ContentTypeMaven, resp.Data[0].Ecosystem)
	assert.Equal(t, "lightwell/maven/remediated", resp.Data[0].Repository)
	assert.Equal(t, "repo-uuid-1", resp.Data[0].RepositoryUUID)
	assert.Equal(t, []string{"2.15.3", "2.14.2"}, resp.Data[0].Versions)
	assert.Len(t, resp.Data[0].LatestReleases, 2)
	assert.Equal(t, "2.15.3", resp.Data[0].LatestReleases[0].Version)
	assert.Equal(t, "rhlw-00001", resp.Data[0].LatestReleases[0].Release)
	assert.Equal(t, "2024-06-01T12:00:00Z", resp.Data[0].LatestReleases[0].CreatedAt)
}

func (s *LightwellPackagesSuite) TestListPackagesWithFilters() {
	t := s.T()
	s.stubLightwellAccess()

	s.reg.LightwellPackage.On("ListPackages", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellPackagesOptions) bool {
		return opts.Ecosystem != nil && *opts.Ecosystem == config.ContentTypeMaven &&
			opts.Name != nil && *opts.Name == "jackson" &&
			opts.Repository != nil && *opts.Repository == "lightwell/maven/remediated" &&
			opts.SecurityLevel != nil && *opts.SecurityLevel == "remediated" &&
			opts.Limit == 10 && opts.Offset == 5 &&
			len(opts.EntitledFeatures) == 1 && opts.EntitledFeatures[0] == "lightwell-network"
	})).Return([]dao.LightwellPackageRow{}, int64(0), nil)

	path := fmt.Sprintf("%s/lightwell/packages?ecosystem=maven&name=jackson&repository=lightwell/maven/remediated&security_level=remediated&limit=10&offset=5", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(0), resp.Meta.Count)
	assert.Empty(t, resp.Data)
}

// TestListPackagesDemoDefaultsToProduction verifies that, without a demo query
// param, the DAO is called with Demo=false (production repos only).
func (s *LightwellPackagesSuite) TestListPackagesDemoDefaultsToProduction() {
	t := s.T()
	s.stubLightwellAccess()

	s.reg.LightwellPackage.On("ListPackages", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellPackagesOptions) bool {
		return !opts.Demo
	})).Return([]dao.LightwellPackageRow{}, int64(0), nil)

	path := fmt.Sprintf("%s/lightwell/packages", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
}

// TestListPackagesDemoTrue verifies that demo=true is passed through to the DAO
// so only demo-org repos are returned.
func (s *LightwellPackagesSuite) TestListPackagesDemoTrue() {
	t := s.T()
	s.stubLightwellAccess()

	s.reg.LightwellPackage.On("ListPackages", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellPackagesOptions) bool {
		return opts.Demo
	})).Return([]dao.LightwellPackageRow{}, int64(0), nil)

	path := fmt.Sprintf("%s/lightwell/packages?demo=true", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (s *LightwellPackagesSuite) TestListPackagesFeatureServiceError() {
	t := s.T()

	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{}, fmt.Errorf("feature service unavailable"))

	path := fmt.Sprintf("%s/lightwell/packages", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(0), resp.Meta.Count)
	assert.Empty(t, resp.Data)

	// DAO should NOT be called
	s.reg.LightwellPackage.AssertNotCalled(t, "ListPackages", mock.Anything, mock.Anything)
}

func (s *LightwellPackagesSuite) TestListPackagesNoLightwellFeatures() {
	t := s.T()

	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{"some-other-feature"}, nil)

	path := fmt.Sprintf("%s/lightwell/packages", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(0), resp.Meta.Count)
	assert.Empty(t, resp.Data)

	// DAO should NOT be called
	s.reg.LightwellPackage.AssertNotCalled(t, "ListPackages", mock.Anything, mock.Anything)
}

func (s *LightwellPackagesSuite) TestListPackagesInvalidEcosystem() {
	t := s.T()
	s.stubLightwellAccess()

	path := fmt.Sprintf("%s/lightwell/packages?ecosystem=invalid", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (s *LightwellPackagesSuite) TestListPackagesNoTangCalls() {
	t := s.T()
	s.stubLightwellAccess()

	s.reg.LightwellPackage.On("ListPackages", test.MockCtx(), mock.Anything).
		Return([]dao.LightwellPackageRow{}, int64(0), nil)

	path := fmt.Sprintf("%s/lightwell/packages", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	_, _, err := s.serveRouter(req)
	require.NoError(t, err)

	// Verify Tang and Pulp clients receive NO calls
	s.tangClient.AssertNotCalled(t, "PythonPackageList", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	s.tangClient.AssertNotCalled(t, "NpmPackageList", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	s.pulpClient.AssertNotCalled(t, "ListMavenPackages", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// --- /lightwell/package_versions tests ---

func (s *LightwellPackagesSuite) TestListPackageVersions() {
	t := s.T()
	s.stubLightwellAccess()

	daoRows := []dao.LightwellPackageVersionRow{
		{
			RepositoryConfigurationUUID: "repo-uuid-1",
			RepositoryName:              "lightwell/maven/remediated",
			Ecosystem:                   config.ContentTypeMaven,
			Name:                        "jackson-databind",
			Group:                       "com.fasterxml.jackson.core",
			Version:                     "2.15.3",
			Release:                     "rhlw-00001",
			PublishedAt:                 "2024-06-01T12:00:00Z",
			Purl:                        "pkg:maven/com.fasterxml.jackson.core/jackson-databind@2.15.3",
			TotalCount:                  1,
		},
	}

	s.reg.LightwellPackage.On("ListPackageVersions", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellPackageVersionsOptions) bool {
		return opts.Limit == int32(DefaultLimit) && opts.Offset == 0 &&
			len(opts.EntitledFeatures) == 1 && opts.EntitledFeatures[0] == "lightwell-network"
	})).Return(daoRows, int64(1), nil)

	path := fmt.Sprintf("%s/lightwell/package_versions", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(1), resp.Meta.Count)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "jackson-databind", resp.Data[0].Name)
	assert.Equal(t, "com.fasterxml.jackson.core", resp.Data[0].Group)
	assert.Equal(t, "2.15.3", resp.Data[0].Version)
	assert.Equal(t, config.ContentTypeMaven, resp.Data[0].Ecosystem)
	assert.Equal(t, "lightwell/maven/remediated", resp.Data[0].Repository)
	assert.Equal(t, "repo-uuid-1", resp.Data[0].RepositoryUUID)
	assert.Equal(t, "rhlw-00001", resp.Data[0].Release)
	assert.Equal(t, "2024-06-01T12:00:00Z", resp.Data[0].CreatedAt)
	assert.Equal(t, "pkg:maven/com.fasterxml.jackson.core/jackson-databind@2.15.3", resp.Data[0].Purl)
	assert.Equal(t, "com.fasterxml.jackson.core:jackson-databind", resp.Data[0].Coordinates)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsWithCVEFilters() {
	t := s.T()
	s.stubLightwellAccess()

	s.reg.LightwellPackage.On("ListPackageVersions", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellPackageVersionsOptions) bool {
		return opts.ResolvesCveID != nil && *opts.ResolvesCveID == "CVE-2024-1234" &&
			opts.VulnerableToCveID != nil && *opts.VulnerableToCveID == "CVE-2024-5678" &&
			len(opts.EntitledFeatures) == 1 && opts.EntitledFeatures[0] == "lightwell-network"
	})).Return([]dao.LightwellPackageVersionRow{}, int64(0), nil)

	path := fmt.Sprintf("%s/lightwell/package_versions?resolves_cve_id=CVE-2024-1234&vulnerable_to_cve_id=CVE-2024-5678", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(0), resp.Meta.Count)
	assert.Empty(t, resp.Data)
}

// TestListPackageVersionsDemoTrue verifies demo=true is passed through to the DAO.
func (s *LightwellPackagesSuite) TestListPackageVersionsDemoTrue() {
	t := s.T()
	s.stubLightwellAccess()

	s.reg.LightwellPackage.On("ListPackageVersions", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellPackageVersionsOptions) bool {
		return opts.Demo
	})).Return([]dao.LightwellPackageVersionRow{}, int64(0), nil)

	path := fmt.Sprintf("%s/lightwell/package_versions?demo=true", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
}

// TestListPackageVersionsDemoDefaultsToProduction verifies the default (no demo
// param) calls the DAO with Demo=false.
func (s *LightwellPackagesSuite) TestListPackageVersionsDemoDefaultsToProduction() {
	t := s.T()
	s.stubLightwellAccess()

	s.reg.LightwellPackage.On("ListPackageVersions", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellPackageVersionsOptions) bool {
		return !opts.Demo
	})).Return([]dao.LightwellPackageVersionRow{}, int64(0), nil)

	path := fmt.Sprintf("%s/lightwell/package_versions", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsFeatureServiceError() {
	t := s.T()

	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{}, fmt.Errorf("feature service unavailable"))

	path := fmt.Sprintf("%s/lightwell/package_versions", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(0), resp.Meta.Count)
	assert.Empty(t, resp.Data)

	// DAO should NOT be called
	s.reg.LightwellPackage.AssertNotCalled(t, "ListPackageVersions", mock.Anything, mock.Anything)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsNoLightwellFeatures() {
	t := s.T()

	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{"some-other-feature"}, nil)

	path := fmt.Sprintf("%s/lightwell/package_versions", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(0), resp.Meta.Count)
	assert.Empty(t, resp.Data)

	// DAO should NOT be called
	s.reg.LightwellPackage.AssertNotCalled(t, "ListPackageVersions", mock.Anything, mock.Anything)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsCoordinatesComputed() {
	t := s.T()
	s.stubLightwellAccess()

	// Test all three ecosystems to verify coordinates are computed correctly
	daoRows := []dao.LightwellPackageVersionRow{
		{
			RepositoryConfigurationUUID: "repo-uuid-1",
			RepositoryName:              "lightwell/maven/remediated",
			Ecosystem:                   config.ContentTypeMaven,
			Name:                        "jackson-databind",
			Group:                       "com.fasterxml.jackson.core",
			Version:                     "2.15.3",
			Purl:                        "pkg:maven/com.fasterxml.jackson.core/jackson-databind@2.15.3",
			TotalCount:                  3,
		},
		{
			RepositoryConfigurationUUID: "repo-uuid-2",
			RepositoryName:              "lightwell/python/remediated",
			Ecosystem:                   config.ContentTypePython,
			Name:                        "requests",
			Group:                       "",
			Version:                     "2.31.0",
			Purl:                        "pkg:pypi/requests@2.31.0",
			TotalCount:                  3,
		},
		{
			RepositoryConfigurationUUID: "repo-uuid-3",
			RepositoryName:              "lightwell/npm/remediated",
			Ecosystem:                   config.ContentTypeNpm,
			Name:                        "lodash",
			Group:                       "@types",
			Version:                     "4.17.0",
			Purl:                        "pkg:npm/%40types/lodash@4.17.0",
			TotalCount:                  3,
		},
	}

	s.reg.LightwellPackage.On("ListPackageVersions", test.MockCtx(), mock.Anything).
		Return(daoRows, int64(3), nil)

	path := fmt.Sprintf("%s/lightwell/package_versions", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Len(t, resp.Data, 3)
	assert.Equal(t, "com.fasterxml.jackson.core:jackson-databind", resp.Data[0].Coordinates)
	assert.Equal(t, "requests", resp.Data[1].Coordinates)
	assert.Equal(t, "@types/lodash", resp.Data[2].Coordinates)
}
