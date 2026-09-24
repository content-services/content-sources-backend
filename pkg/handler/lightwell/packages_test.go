package lightwell

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type LightwellPackagesSuite struct {
	LightwellSuite
}

func TestLightwellPackagesSuite(t *testing.T) {
	suite.Run(t, new(LightwellPackagesSuite))
}

// stubLightwellRepos sets up the DAO mock to return the given repos for a List call with origin=lightwell.
func (s *LightwellPackagesSuite) stubLightwellRepos(repos []api.RepositoryResponse) {
	s.reg.RepositoryConfig.On(
		"List", test.MockCtx(), test_handler.MockOrgId,
		mock.MatchedBy(func(p api.PaginationData) bool { return p.Limit == handler.MaxLimit }),
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

// --- /packages tests ---

func (s *LightwellPackagesSuite) TestPackagesRoute() {
	t := s.T()

	s.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId,
		api.PaginationData{Limit: 200, Offset: 0},
		api.FilterData{Origin: config.OriginLightwell}).
		Return(api.RepositoryCollectionResponse{}, int64(0), nil).Once()

	path := fmt.Sprintf("%s/packages", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	assert.Nil(t, err)
	// Route should be registered - we don't care about the exact response code
	assert.NotEqual(t, http.StatusNotFound, code, "Route should be registered")
}

func (s *LightwellPackagesSuite) TestListPackagesSingleRepo() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/default/api/v3/repositories/maven/maven/some-uuid/"
	s.stubRepoHref(mavenRepo, href)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", handler.DefaultLimit, 0).
		Return(mavenPulpResponse(), nil)

	path := fmt.Sprintf("%s/packages", LightwellAPIPath)
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

	s.pulpClient.On("ListMavenPackages", test.MockCtx(), mavenHref, "", handler.MaxLimit, 0).
		Return(mavenPulpResponse(), nil)
	s.tangClient.On("PythonPackageList", test.MockCtx(), pythonHref,
		tangy.PythonPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: handler.MaxLimit},
	).Return(pythonTangResponse(), nil)

	path := fmt.Sprintf("%s/packages", LightwellAPIPath)
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
	// Only maven repo should be returned when filtering by ecosystem=maven
	s.reg.RepositoryConfig.On(
		"List", test.MockCtx(), test_handler.MockOrgId,
		mock.MatchedBy(func(p api.PaginationData) bool { return p.Limit == handler.MaxLimit }),
		mock.MatchedBy(func(f api.FilterData) bool {
			return f.Origin == config.OriginLightwell && f.ContentType == config.ContentTypeMaven
		}),
	).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{mavenRepo}}, int64(1), nil)

	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", handler.DefaultLimit, 0).
		Return(mavenPulpResponse(), nil)

	path := fmt.Sprintf("%s/packages?ecosystem=maven", LightwellAPIPath)
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

	path := fmt.Sprintf("%s/packages?ecosystem=invalid", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (s *LightwellPackagesSuite) TestListPackagesEmptyResult() {
	t := s.T()

	s.stubLightwellRepos([]api.RepositoryResponse{})

	path := fmt.Sprintf("%s/packages", LightwellAPIPath)
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

// --- /package_versions tests ---

func (s *LightwellPackagesSuite) TestPackageVersionsRoute() {
	t := s.T()

	s.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId,
		api.PaginationData{Limit: 200, Offset: 0},
		api.FilterData{Origin: config.OriginLightwell}).
		Return(api.RepositoryCollectionResponse{}, int64(0), nil).Once()

	path := fmt.Sprintf("%s/package_versions", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	assert.Nil(t, err)
	assert.NotEqual(t, http.StatusNotFound, code, "Route should be registered")
}

func (s *LightwellPackagesSuite) TestListPackageVersionsSingleRepo() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", handler.MaxLimit, 0).
		Return(mavenPulpResponse(), nil)

	path := fmt.Sprintf("%s/package_versions", LightwellAPIPath)
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
}

func (s *LightwellPackagesSuite) TestListPackageVersionsWithNameFilter() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "jackson", handler.MaxLimit, 0).
		Return(mavenPulpResponse(), nil)

	path := fmt.Sprintf("%s/package_versions?name=jackson", LightwellAPIPath)
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
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", handler.MaxLimit, 0).
		Return(mavenPulpResponse(), nil)

	// Request with limit=1&offset=0 — should get 1 of 2 versions
	path := fmt.Sprintf("%s/package_versions?limit=1&offset=0", LightwellAPIPath)
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

	path := fmt.Sprintf("%s/package_versions?ecosystem=bogus", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsEmptyResult() {
	t := s.T()

	s.stubLightwellRepos([]api.RepositoryResponse{})

	path := fmt.Sprintf("%s/package_versions", LightwellAPIPath)
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
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", handler.MaxLimit, 0).
		Return(mavenPulpResponse(), nil)

	s.reg.LightwellAdvisory.On("ListAdvisoriesByCveID", test.MockCtx(), "CVE-2024-9999").Return([]dao.LightwellAdvisoryCveMatch{
		{
			PackageName:   "jackson-databind",
			FixedVersions: []string{"2.15.3"},
			RepoName:      "lightwell/java/remediated",
			Severity:      "critical",
		},
	}, nil)

	path := fmt.Sprintf("%s/package_versions?resolves_cve_id=CVE-2024-9999", LightwellAPIPath)
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
}

func (s *LightwellPackagesSuite) TestListPackageVersionsVulnerableToCveFilter() {
	t := s.T()

	mavenRepo := newMavenRepo()
	s.stubLightwellRepos([]api.RepositoryResponse{mavenRepo})
	href := "/api/pulp/repos/maven/1/"
	s.stubRepoHref(mavenRepo, href)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", handler.MaxLimit, 0).
		Return(mavenPulpResponse(), nil)

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

	path := fmt.Sprintf("%s/package_versions?vulnerable_to_cve_id=CVE-2024-8888", LightwellAPIPath)
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
