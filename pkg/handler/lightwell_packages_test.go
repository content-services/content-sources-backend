package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
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
}

func TestLightwellPackagesSuite(t *testing.T) {
	suite.Run(t, new(LightwellPackagesSuite))
}

func (s *LightwellPackagesSuite) SetupTest() {
	s.reg = dao.GetMockDaoRegistry(s.T())
	s.tangClient = tangy.NewMockTangy(s.T())
	s.pulpClient = pulp_client.NewMockPulpClient(s.T())
}

func (s *LightwellPackagesSuite) serveRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	pathPrefix := router.Group(api.FullRootPath())
	RegisterLightwellPackageRoutes(pathPrefix, s.reg.ToDaoRegistry(), s.tangClient, s.pulpClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

// stubLightwellRepos sets up the DAO mock to return the given repos for a List call with origin=lightwell.
func (s *LightwellPackagesSuite) stubLightwellRepos(repos []api.RepositoryResponse) {
	s.reg.RepositoryConfig.On(
		"List", test.MockCtx(), test_handler.MockOrgId,
		mock.MatchedBy(func(p api.PaginationData) bool { return p.Limit == MaxLimit }),
		mock.MatchedBy(func(f api.FilterData) bool { return f.Origin == config.OriginLightwell }),
	).Return(api.RepositoryCollectionResponse{Data: repos}, int64(len(repos)), nil)
}

func (s *LightwellPackagesSuite) stubRepoHref(repo api.RepositoryResponse, href string) {
	domainName := "test-domain"
	s.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), repo.OrgID).Return(domainName, nil).Maybe()
	s.pulpClient.On("WithDomain", domainName).Return(s.pulpClient).Maybe()
	s.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), repo.PublishedDistBasePath).Return(&href, nil).Maybe()
}

func newMavenRepo() api.RepositoryResponse {
	return api.RepositoryResponse{
		UUID:                  "aaa-bbb-ccc",
		Name:                  "lightwell/java/remediated",
		ContentType:           config.ContentTypeMaven,
		Origin:                config.OriginLightwell,
		SecurityLevel:         "remediated",
		PublishedDistBasePath: "java/remediated",
		OrgID:                 test_handler.MockOrgId,
	}
}

func newPythonRepo() api.RepositoryResponse {
	return api.RepositoryResponse{
		UUID:                  "ddd-eee-fff",
		Name:                  "lightwell/python/remediated",
		ContentType:           config.ContentTypePython,
		Origin:                config.OriginLightwell,
		SecurityLevel:         "remediated",
		PublishedDistBasePath: "python/remediated",
		OrgID:                 test_handler.MockOrgId,
	}
}

func mavenTangResponse() tangy.MavenPackageListResponse {
	return tangy.MavenPackageListResponse{
		Results: []tangy.MavenPackageListItem{
			{
				GroupID:    "com.fasterxml.jackson.core",
				ArtifactID: "jackson-databind",
				Versions:   []string{"2.15.3.rhlw-00001", "2.14.2.rhlw-00001"},
				LatestReleases: []tangy.MavenReleaseInfo{
					{Version: "2.15.3.rhlw-00001", Release: "rhlw-00001", CreatedAt: "2024-06-01T12:00:00Z"},
				},
			},
		},
		Total: 1, Limit: 200, Offset: 0,
	}
}

func newNpmRepo() api.RepositoryResponse {
	return api.RepositoryResponse{
		UUID:                  "ggg-hhh-iii",
		Name:                  "lightwell/npm/remediated",
		ContentType:           config.ContentTypeNpm,
		Origin:                config.OriginLightwell,
		SecurityLevel:         "remediated",
		PublishedDistBasePath: "npm/remediated",
		OrgID:                 test_handler.MockOrgId,
	}
}

func npmScopedTangResponse() tangy.NpmPackageListResponse {
	return tangy.NpmPackageListResponse{
		Results: []tangy.NpmPackageListItem{
			{
				Name:     "@types/is-odd",
				Versions: []string{"3.0.0.rhlw-00001"},
				LatestVersions: []tangy.NpmVersionInfo{
					{Version: "3.0.0.rhlw-00001", CreatedAt: "2024-07-01T10:00:00Z"},
				},
			},
		},
		Total: 1, Limit: 200, Offset: 0,
	}
}

func npmUnscopedTangResponse() tangy.NpmPackageListResponse {
	return tangy.NpmPackageListResponse{
		Results: []tangy.NpmPackageListItem{
			{
				Name:     "lodash",
				Versions: []string{"4.17.21.rhlw-00001"},
				LatestVersions: []tangy.NpmVersionInfo{
					{Version: "4.17.21.rhlw-00001", CreatedAt: "2024-07-02T10:00:00Z"},
				},
			},
		},
		Total: 1, Limit: 200, Offset: 0,
	}
}

func pythonTangResponse() tangy.PythonPackageListResponse {
	return tangy.PythonPackageListResponse{
		Results: []tangy.PythonPackageListItem{
			{
				Name:           "requests",
				NameNormalized: "requests",
				Versions:       []string{"2.31.0.rhlw-00001"},
				LatestVersions: []tangy.PythonVersionInfo{
					{Version: "2.31.0.rhlw-00001", CreatedAt: "2024-05-10T08:00:00Z"},
				},
			},
		},
		Total: 1, Limit: 200, Offset: 0,
	}
}

// --- /lightwell/packages tests ---

func (s *LightwellPackagesSuite) TestListPackagesSingleRepo() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/default/api/v3/repositories/maven/maven/some-uuid/"
	s.stubRepoHref(mavenRepo, href)
	s.tangClient.On("MavenPackageList", test.MockCtx(), href,
		tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(mavenTangResponse(), nil)

	path := fmt.Sprintf("%s/lightwell/packages", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Equal(t, int64(1), resp.Meta.Count)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "jackson-databind", resp.Data[0].Name)
	assert.Equal(t, "com.fasterxml.jackson.core", resp.Data[0].Group)
	assert.Equal(t, config.ContentTypeMaven, resp.Data[0].ContentType)
	assert.Equal(t, "lightwell/java/remediated", resp.Data[0].Repository)
	assert.Equal(t, 2, len(resp.Data[0].Versions))
}

func (s *LightwellPackagesSuite) TestListPackagesMultiRepo() {
	t := s.T()

	mavenRepo := newMavenRepo()
	pythonRepo := newPythonRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo, pythonRepo})

	mavenHref := "/api/pulp/repos/maven/1/"
	pythonHref := "/api/pulp/repos/python/1/"
	s.stubRepoHref(mavenRepo, mavenHref)
	s.stubRepoHref(pythonRepo, pythonHref)

	s.tangClient.On("MavenPackageList", test.MockCtx(), mavenHref,
		tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(mavenTangResponse(), nil)
	s.tangClient.On("PythonPackageList", test.MockCtx(), pythonHref,
		tangy.PythonPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(pythonTangResponse(), nil)

	path := fmt.Sprintf("%s/lightwell/packages", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Equal(t, int64(2), resp.Meta.Count)
	assert.Len(t, resp.Data, 2)

	contentTypes := map[string]bool{}
	for _, p := range resp.Data {
		contentTypes[p.ContentType] = true
	}
	assert.True(t, contentTypes[config.ContentTypeMaven])
	assert.True(t, contentTypes[config.ContentTypePython])
}

func (s *LightwellPackagesSuite) TestListPackagesTypeFilter() {
	t := s.T()

	mavenRepo := newMavenRepo()
	// Only maven repo should be returned when filtering by content_type=maven
	s.reg.RepositoryConfig.On(
		"List", test.MockCtx(), test_handler.MockOrgId,
		mock.MatchedBy(func(p api.PaginationData) bool { return p.Limit == MaxLimit }),
		mock.MatchedBy(func(f api.FilterData) bool {
			return f.Origin == config.OriginLightwell && f.ContentType == config.ContentTypeMaven
		}),
	).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{mavenRepo}}, int64(1), nil)

	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.tangClient.On("MavenPackageList", test.MockCtx(), href,
		tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(mavenTangResponse(), nil)

	path := fmt.Sprintf("%s/lightwell/packages?content_type=maven", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Len(t, resp.Data, 1)
	assert.Equal(t, config.ContentTypeMaven, resp.Data[0].ContentType)
}

func (s *LightwellPackagesSuite) TestListPackagesInvalidType() {
	t := s.T()

	path := fmt.Sprintf("%s/lightwell/packages?content_type=invalid", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (s *LightwellPackagesSuite) TestListPackagesEmptyResult() {
	t := s.T()

	s.stubLightwellRepos([]api.RepositoryResponse{})

	path := fmt.Sprintf("%s/lightwell/packages", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Equal(t, int64(0), resp.Meta.Count)
	assert.NotNil(t, resp.Data)
	assert.Empty(t, resp.Data)
}

// --- /lightwell/package_versions tests ---

func (s *LightwellPackagesSuite) TestListPackageVersionsSingleRepo() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.tangClient.On("MavenPackageList", test.MockCtx(), href,
		tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(mavenTangResponse(), nil)

	path := fmt.Sprintf("%s/lightwell/package_versions", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Equal(t, int64(2), resp.Meta.Count) // 2 versions for jackson-databind
	assert.Len(t, resp.Data, 2)
	assert.Equal(t, "jackson-databind", resp.Data[0].Name)
	assert.Equal(t, config.ContentTypeMaven, resp.Data[0].ContentType)
	assert.Equal(t, "pkg:maven/com.fasterxml.jackson.core/jackson-databind@"+resp.Data[0].Version, resp.Data[0].Purl)
	assert.Equal(t, "com.fasterxml.jackson.core:jackson-databind", resp.Data[0].Coordinates)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsWithNameFilter() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.tangClient.On("MavenPackageList", test.MockCtx(), href,
		tangy.MavenPackageListFilters{Search: "jackson"},
		tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(mavenTangResponse(), nil)

	path := fmt.Sprintf("%s/lightwell/package_versions?name=jackson", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Len(t, resp.Data, 2)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsPagination() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.tangClient.On("MavenPackageList", test.MockCtx(), href,
		tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(mavenTangResponse(), nil)

	// Request with limit=1&offset=0 — should get 1 of 2 versions
	path := fmt.Sprintf("%s/lightwell/package_versions?limit=1&offset=0", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Equal(t, int64(2), resp.Meta.Count) // total is 2
	assert.Len(t, resp.Data, 1)                // page is 1
	assert.NotEmpty(t, resp.Links.Next)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsInvalidType() {
	t := s.T()

	path := fmt.Sprintf("%s/lightwell/package_versions?content_type=bogus", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsEmptyResult() {
	t := s.T()

	s.stubLightwellRepos([]api.RepositoryResponse{})

	path := fmt.Sprintf("%s/lightwell/package_versions", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, int64(0), resp.Meta.Count)
	assert.NotNil(t, resp.Data)
	assert.Empty(t, resp.Data)
}

// --- resolves_cve_id / vulnerable_to_cve_id filter tests ---

func (s *LightwellPackagesSuite) TestListPackageVersionsResolvesCveFilter() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.tangClient.On("MavenPackageList", test.MockCtx(), href,
		tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(mavenTangResponse(), nil)

	s.reg.LightwellAdvisory.On("ListAdvisoriesByCveID", test.MockCtx(), "CVE-2024-9999").Return([]dao.LightwellAdvisoryCveMatch{
		{
			PackageName:   "jackson-databind",
			FixedVersions: []string{"2.15.3.rhlw-00001"},
			RepoName:      "lightwell/java/remediated",
			Severity:      "critical",
		},
	}, nil)

	path := fmt.Sprintf("%s/lightwell/package_versions?resolves_cve_id=CVE-2024-9999", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Equal(t, int64(1), resp.Meta.Count)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "jackson-databind", resp.Data[0].Name)
	assert.Equal(t, "2.15.3.rhlw-00001", resp.Data[0].Version)
	assert.Equal(t, "pkg:maven/com.fasterxml.jackson.core/jackson-databind@2.15.3.rhlw-00001", resp.Data[0].Purl)
	assert.Equal(t, "com.fasterxml.jackson.core:jackson-databind", resp.Data[0].Coordinates)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsVulnerableToCveFilter() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.tangClient.On("MavenPackageList", test.MockCtx(), href,
		tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(mavenTangResponse(), nil)

	// Advisory says jackson-databind is fixed at 2.15.3.rhlw-00001, so
	// the older version 2.14.2.rhlw-00001 should be returned as vulnerable.
	s.reg.LightwellAdvisory.On("ListAdvisoriesByCveID", test.MockCtx(), "CVE-2024-8888").Return([]dao.LightwellAdvisoryCveMatch{
		{
			PackageName:   "jackson-databind",
			FixedVersions: []string{"2.15.3.rhlw-00001"},
			RepoName:      "lightwell/java/remediated",
			Severity:      "important",
		},
	}, nil)

	path := fmt.Sprintf("%s/lightwell/package_versions?vulnerable_to_cve_id=CVE-2024-8888", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Equal(t, int64(1), resp.Meta.Count)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "jackson-databind", resp.Data[0].Name)
	assert.Equal(t, "2.14.2.rhlw-00001", resp.Data[0].Version)
}

// --- nested repo-scoped alias tests ---

func (s *LightwellPackagesSuite) TestNestedRepoPackagesAlias() {
	t := s.T()

	mavenRepo := newMavenRepo()
	mavenRepo.Name = "java-remediated"
	s.reg.RepositoryConfig.On(
		"List", test.MockCtx(), test_handler.MockOrgId,
		mock.MatchedBy(func(p api.PaginationData) bool { return p.Limit == MaxLimit }),
		mock.MatchedBy(func(f api.FilterData) bool { return f.Origin == config.OriginLightwell }),
	).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{mavenRepo}}, int64(1), nil)

	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.tangClient.On("MavenPackageList", test.MockCtx(), href,
		tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(mavenTangResponse(), nil)

	path := fmt.Sprintf("%s/lightwell/repositories/java-remediated/packages", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Equal(t, int64(1), resp.Meta.Count)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "jackson-databind", resp.Data[0].Name)
}

func (s *LightwellPackagesSuite) TestNestedRepoPackageVersionsAlias() {
	t := s.T()

	mavenRepo := newMavenRepo()
	mavenRepo.Name = "java-remediated"
	s.reg.RepositoryConfig.On(
		"List", test.MockCtx(), test_handler.MockOrgId,
		mock.MatchedBy(func(p api.PaginationData) bool { return p.Limit == MaxLimit }),
		mock.MatchedBy(func(f api.FilterData) bool { return f.Origin == config.OriginLightwell }),
	).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{mavenRepo}}, int64(1), nil)

	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.tangClient.On("MavenPackageList", test.MockCtx(), href,
		tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(mavenTangResponse(), nil)

	path := fmt.Sprintf("%s/lightwell/repositories/java-remediated/package_versions", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Equal(t, int64(2), resp.Meta.Count)
	assert.Len(t, resp.Data, 2)
}

// --- npm PURL / coordinates tests ---

func (s *LightwellPackagesSuite) TestListPackageVersionsNpmScoped() {
	t := s.T()

	npmRepo := newNpmRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{npmRepo})
	href := "/api/pulp/repos/npm/1/"
	s.stubRepoHref(npmRepo, href)
	s.tangClient.On("NpmPackageList", test.MockCtx(), href,
		tangy.NpmPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(npmScopedTangResponse(), nil)

	path := fmt.Sprintf("%s/lightwell/package_versions", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "is-odd", resp.Data[0].Name)
	assert.Equal(t, "@types", resp.Data[0].Group)
	assert.Equal(t, "pkg:npm/%40types/is-odd@3.0.0.rhlw-00001", resp.Data[0].Purl)
	assert.Equal(t, "@types/is-odd", resp.Data[0].Coordinates)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsNpmUnscoped() {
	t := s.T()

	npmRepo := newNpmRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{npmRepo})
	href := "/api/pulp/repos/npm/1/"
	s.stubRepoHref(npmRepo, href)
	s.tangClient.On("NpmPackageList", test.MockCtx(), href,
		tangy.NpmPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(npmUnscopedTangResponse(), nil)

	path := fmt.Sprintf("%s/lightwell/package_versions", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "lodash", resp.Data[0].Name)
	assert.Equal(t, "-", resp.Data[0].Group)
	assert.Equal(t, "pkg:npm/lodash@4.17.21.rhlw-00001", resp.Data[0].Purl)
	assert.Equal(t, "lodash", resp.Data[0].Coordinates)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsPythonPurl() {
	t := s.T()

	pythonRepo := newPythonRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{pythonRepo})
	href := "/api/pulp/repos/python/1/"
	s.stubRepoHref(pythonRepo, href)
	s.tangClient.On("PythonPackageList", test.MockCtx(), href,
		tangy.PythonPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: MaxLimit},
	).Return(pythonTangResponse(), nil)

	path := fmt.Sprintf("%s/lightwell/package_versions", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "requests", resp.Data[0].Name)
	assert.Equal(t, "pkg:pypi/requests@2.31.0.rhlw-00001", resp.Data[0].Purl)
	assert.Equal(t, "requests", resp.Data[0].Coordinates)
}
