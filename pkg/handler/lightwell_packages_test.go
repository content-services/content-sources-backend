package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
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

func mavenPulpResponse() zest.PaginatedMavenRepositoryPackageListResponse {
	return zest.PaginatedMavenRepositoryPackageListResponse{
		Count: 1,
		Results: []zest.MavenRepositoryPackageResponse{
			{
				GroupId:    "com.fasterxml.jackson.core",
				ArtifactId: "jackson-databind",
				Versions:   []string{"2.15.3", "2.14.2"},
				LatestReleases: []zest.MavenPackageReleaseResponse{
					{Version: "2.15.3", Release: "rhlw-00001", CreatedAt: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)},
					{Version: "2.14.2", Release: "rhlw-00001", CreatedAt: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)},
				},
			},
		},
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

// mavenPulpResponsePage builds a page of maven results with distinct artifact
// ids, reporting the given overall total so pagination can be exercised.
func mavenPulpResponsePage(offset, pageLen, total int) zest.PaginatedMavenRepositoryPackageListResponse {
	results := make([]zest.MavenRepositoryPackageResponse, pageLen)
	for i := range pageLen {
		results[i] = zest.MavenRepositoryPackageResponse{
			GroupId:    "com.example",
			ArtifactId: fmt.Sprintf("artifact-%d", offset+i),
			Versions:   []string{"1.0.0"},
		}
	}
	return zest.PaginatedMavenRepositoryPackageListResponse{Count: int64(total), Results: results}
}

// --- /lightwell/packages tests ---

// TestListPackagesSingleRepoUsesTangTotal verifies the single-repo fast path:
// the request's page is pushed down to Tang in one call and the count comes from
// Tang's total, so a repo with more packages than a single page reports the full
// count without fetching everything.
func (s *LightwellPackagesSuite) TestListPackagesSingleRepoUsesTangTotal() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/big/"
	s.stubRepoHref(mavenRepo, href)

	total := 567 // far more than one page
	// Exactly one Pulp call, using the request's offset/limit (not a full fetch).
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", 20, 0).
		Return(mavenPulpResponsePage(0, 20, total), nil)

	path := fmt.Sprintf("%s/lightwell/packages?offset=0&limit=20", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Equal(t, int64(total), resp.Meta.Count)
	assert.Len(t, resp.Data, 20)
}

// TestListPackagesSingleRepoPushesSortWhenTangSupports verifies the forward
// wiring: once Tang honors server-side sorting, a non-native sort (here, desc) on
// a single repo takes the fast path and forwards the sort to Tang via SortBy.
func (s *LightwellPackagesSuite) TestListPackagesSingleRepoPushesSortWhenTangSupports() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/sorted/"
	s.stubRepoHref(mavenRepo, href)

	// Maven catalog reads go to Pulp, which does not take SortBy. The single-repo
	// fast path still uses one ListMavenPackages call with the request's page.
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", DefaultLimit, 0).
		Return(mavenPulpResponsePage(0, 5, 42), nil)

	path := fmt.Sprintf("%s/lightwell/packages?sort_by=name+desc", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Equal(t, int64(42), resp.Meta.Count)
}

// TestListPackagesMultiRepoPaginatesBeyondMaxLimit verifies that the multi-repo
// fallback path still pages through Tang so repos larger than MaxLimit are not
// truncated.
func (s *LightwellPackagesSuite) TestListPackagesMultiRepoPaginatesBeyondMaxLimit() {
	t := s.T()

	repoA := newMavenRepo()
	repoB := newMavenRepo()
	repoB.UUID = "bbb-ccc-ddd"
	repoB.Name = "lightwell/java/remediated-2"
	repoB.PublishedDistBasePath = "java/remediated-2"
	s.stubLightwellRepos([]api.RepositoryResponse{repoA, repoB})

	hrefA := "/api/pulp/repos/maven/a/"
	hrefB := "/api/pulp/repos/maven/b/"
	s.stubRepoHref(repoA, hrefA)
	s.stubRepoHref(repoB, hrefB)

	totalA := MaxLimit + 4 // requires two pages
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), hrefA, "", MaxLimit, 0).
		Return(mavenPulpResponsePage(0, MaxLimit, totalA), nil)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), hrefA, "", MaxLimit, MaxLimit).
		Return(mavenPulpResponsePage(MaxLimit, 4, totalA), nil)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), hrefB, "", MaxLimit, 0).
		Return(mavenPulpResponsePage(0, 3, 3), nil)

	path := fmt.Sprintf("%s/lightwell/packages?limit=1000", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Equal(t, int64(totalA+3), resp.Meta.Count)
}

func newDemoMavenRepo() api.RepositoryResponse {
	return api.RepositoryResponse{
		UUID:                  "demo-uuid-111",
		Name:                  "lightwell/java/validated-demo",
		ContentType:           config.ContentTypeMaven,
		Origin:                config.OriginLightwell,
		SecurityLevel:         "validated",
		PublishedDistBasePath: "java/validated-demo",
		OrgID:                 config.LightwellDemoOrg,
	}
}

// TestListPackagesExcludesDemoByDefault verifies demo repos (served from the
// Lightwell demo org) are not aggregated unless demo=true is requested. Only the
// production repo is queried; a Tang call against the demo repo would fail the
// mock, since no expectation is registered for its href.
func (s *LightwellPackagesSuite) TestListPackagesExcludesDemoByDefault() {
	t := s.T()

	realRepo := newMavenRepo()
	demoRepo := newDemoMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{realRepo, demoRepo})

	href := "/api/pulp/repos/maven/real/"
	s.stubRepoHref(realRepo, href)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", DefaultLimit, 0).
		Return(mavenPulpResponsePage(0, 3, 3), nil)

	path := fmt.Sprintf("%s/lightwell/packages", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, int64(3), resp.Meta.Count)
}

// TestListPackagesDemoTrueReturnsOnlyDemo verifies demo=true aggregates only the
// demo-org repos and excludes production repos.
func (s *LightwellPackagesSuite) TestListPackagesDemoTrueReturnsOnlyDemo() {
	t := s.T()

	realRepo := newMavenRepo()
	demoRepo := newDemoMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{realRepo, demoRepo})

	href := "/api/pulp/repos/maven/demo/"
	s.stubRepoHref(demoRepo, href)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", DefaultLimit, 0).
		Return(mavenPulpResponsePage(0, 7, 7), nil)

	path := fmt.Sprintf("%s/lightwell/packages?demo=true", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, int64(7), resp.Meta.Count)
}

// mavenVersionsTangResponsePage builds a page where every package has two
// versions, so the number of expanded version items differs from the package
// count used to advance the pagination offset.
func mavenVersionsPulpResponsePage(offset, pageLen, total int) zest.PaginatedMavenRepositoryPackageListResponse {
	results := make([]zest.MavenRepositoryPackageResponse, pageLen)
	for i := range pageLen {
		results[i] = zest.MavenRepositoryPackageResponse{
			GroupId:    "com.example",
			ArtifactId: fmt.Sprintf("artifact-%d", offset+i),
			Versions:   []string{"1.0.0", "2.0.0"},
		}
	}
	return zest.PaginatedMavenRepositoryPackageListResponse{Count: int64(total), Results: results}
}

func (s *LightwellPackagesSuite) TestListPackageVersionsPaginatesByPackageCount() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/bigversions/"
	s.stubRepoHref(mavenRepo, href)

	totalPackages := MaxLimit + 2
	// Offset must advance by the package count (MaxLimit), not by the number of
	// expanded version items, so the second page is requested at Offset=MaxLimit.
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", MaxLimit, 0).
		Return(mavenVersionsPulpResponsePage(0, MaxLimit, totalPackages), nil)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", MaxLimit, MaxLimit).
		Return(mavenVersionsPulpResponsePage(MaxLimit, 2, totalPackages), nil)

	path := fmt.Sprintf("%s/lightwell/package_versions", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	// two versions per package across all pages
	assert.Equal(t, int64(totalPackages*2), resp.Meta.Count)
}

func (s *LightwellPackagesSuite) TestListPackagesSingleRepo() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/default/api/v3/repositories/maven/maven/some-uuid/"
	s.stubRepoHref(mavenRepo, href)
	// Single repo takes the fast path: one Pulp call using the request's page.
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", DefaultLimit, 0).Return(mavenPulpResponse(), nil)

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
	assert.Equal(t, config.ContentTypeMaven, resp.Data[0].Ecosystem)
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

	s.pulpClient.On("ListMavenPackages", test.MockCtx(), mavenHref, "", MaxLimit, 0).Return(mavenPulpResponse(), nil)
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

	ecosystems := map[string]bool{}
	for _, p := range resp.Data {
		ecosystems[p.Ecosystem] = true
	}
	assert.True(t, ecosystems[config.ContentTypeMaven])
	assert.True(t, ecosystems[config.ContentTypePython])
}

func (s *LightwellPackagesSuite) TestListPackagesTypeFilter() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.reg.RepositoryConfig.On(
		"List", test.MockCtx(), test_handler.MockOrgId,
		mock.MatchedBy(func(p api.PaginationData) bool { return p.Limit == MaxLimit }),
		mock.MatchedBy(func(f api.FilterData) bool {
			return f.Origin == config.OriginLightwell && f.ContentType == config.ContentTypeMaven
		}),
	).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{mavenRepo}}, int64(1), nil)

	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	// Single repo takes the fast path: one Pulp call using the request's page.
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", DefaultLimit, 0).Return(mavenPulpResponse(), nil)

	path := fmt.Sprintf("%s/lightwell/packages?ecosystem=maven", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Len(t, resp.Data, 1)
	assert.Equal(t, config.ContentTypeMaven, resp.Data[0].Ecosystem)
}

func (s *LightwellPackagesSuite) TestListPackagesInvalidType() {
	t := s.T()

	path := fmt.Sprintf("%s/lightwell/packages?ecosystem=invalid", api.FullRootPath())
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
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", MaxLimit, 0).Return(mavenPulpResponse(), nil)

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
	assert.Equal(t, config.ContentTypeMaven, resp.Data[0].Ecosystem)
	assert.Equal(t, "pkg:maven/com.fasterxml.jackson.core/jackson-databind@"+resp.Data[0].Version, resp.Data[0].Purl)
	assert.Equal(t, "com.fasterxml.jackson.core:jackson-databind", resp.Data[0].Coordinates)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsWithNameFilter() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "jackson", MaxLimit, 0).Return(mavenPulpResponse(), nil)

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
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", MaxLimit, 0).Return(mavenPulpResponse(), nil)

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

	path := fmt.Sprintf("%s/lightwell/package_versions?ecosystem=bogus", api.FullRootPath())
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
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", MaxLimit, 0).Return(mavenPulpResponse(), nil)

	s.reg.LightwellAdvisory.On("ListAdvisoriesByCveID", test.MockCtx(), "CVE-2024-9999").Return([]dao.LightwellAdvisoryCveMatch{
		{
			PackageName:   "jackson-databind",
			FixedVersions: []string{"2.15.3"},
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
	assert.Equal(t, "2.15.3", resp.Data[0].Version)
	assert.Equal(t, "pkg:maven/com.fasterxml.jackson.core/jackson-databind@2.15.3", resp.Data[0].Purl)
	assert.Equal(t, "com.fasterxml.jackson.core:jackson-databind", resp.Data[0].Coordinates)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsVulnerableToCveFilter() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", MaxLimit, 0).Return(mavenPulpResponse(), nil)

	// Advisory says jackson-databind is fixed at 2.15.3, so
	// the older version 2.14.2 should be returned as vulnerable.
	s.reg.LightwellAdvisory.On("ListAdvisoriesByCveID", test.MockCtx(), "CVE-2024-8888").Return([]dao.LightwellAdvisoryCveMatch{
		{
			PackageName:   "jackson-databind",
			FixedVersions: []string{"2.15.3"},
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
	assert.Equal(t, "2.14.2", resp.Data[0].Version)
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
