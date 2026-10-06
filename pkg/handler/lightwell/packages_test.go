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
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
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

func (s *LightwellPackagesSuite) stubRepos(repos []api.RepositoryResponse) {
	s.reg.RepositoryConfig.On(
		"List", test.MockCtx(), test_handler.MockOrgId,
		mock.MatchedBy(func(p api.PaginationData) bool { return p.Limit == handler.MaxLimit }),
		mock.MatchedBy(func(f api.FilterData) bool { return f.Origin == config.OriginLightwell }),
	).Return(api.RepositoryCollectionResponse{Data: repos}, int64(len(repos)), nil)
}

func (s *LightwellPackagesSuite) stubRepoHref(repo api.RepositoryResponse, href string) {
	s.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), repo.OrgID).Return("test-domain", nil).Maybe()
	s.pulpClient.On("WithDomain", "test-domain").Return(s.pulpClient).Maybe()
	s.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), repo.PublishedDistBasePath).Return(&href, nil).Maybe()
}

func mavenRepo() api.RepositoryResponse {
	return api.RepositoryResponse{
		UUID:                  "aaa-bbb-ccc",
		Name:                  "lightwell/java/remediated",
		ContentType:           config.ContentTypeMaven,
		Origin:                config.OriginLightwell,
		OrgID:                 config.LightwellOrg,
		PublishedDistBasePath: "java/remediated",
	}
}

func mavenPackages() zest.PaginatedMavenRepositoryPackageListResponse {
	return zest.PaginatedMavenRepositoryPackageListResponse{
		Count: 1,
		Results: []zest.MavenRepositoryPackageResponse{{
			GroupId:    "com.fasterxml.jackson.core",
			ArtifactId: "jackson-databind",
			Versions:   []string{"2.15.3"},
			LatestReleases: []zest.MavenPackageReleaseResponse{
				{Version: "2.15.3", Release: "rhlw-00001", CreatedAt: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)},
			},
		}},
	}
}

func (s *LightwellPackagesSuite) TestListPackagesReadsPulp() {
	t := s.T()
	repo := mavenRepo()
	href := "/api/pulp/repos/maven/1/"
	s.stubRepos([]api.RepositoryResponse{repo})
	s.stubRepoHref(repo, href)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", handler.DefaultLimit, 0).
		Return(mavenPackages(), nil)

	path := fmt.Sprintf("%s/packages", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "jackson-databind", resp.Data[0].Name)
	assert.Equal(t, "com.fasterxml.jackson.core", resp.Data[0].Group)
	s.reg.LightwellPackage.AssertNotCalled(t, "ListPackages", mock.Anything, mock.Anything)
}

func (s *LightwellPackagesSuite) TestListPackagesInvalidEcosystem() {
	t := s.T()

	path := fmt.Sprintf("%s/packages?ecosystem=bogus", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	s.reg.LightwellPackage.AssertNotCalled(t, "ListPackages", mock.Anything, mock.Anything)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsReadsPulp() {
	t := s.T()
	repo := mavenRepo()
	href := "/api/pulp/repos/maven/1/"
	s.stubRepos([]api.RepositoryResponse{repo})
	s.stubRepoHref(repo, href)
	s.pulpClient.On("ListMavenPackages", test.MockCtx(), href, "", handler.MaxLimit, 0).
		Return(mavenPackages(), nil)

	path := fmt.Sprintf("%s/package_versions", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageVersionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "2.15.3", resp.Data[0].Version)
	assert.Equal(t, "pkg:maven/com.fasterxml.jackson.core/jackson-databind@2.15.3", resp.Data[0].Purl)
	s.reg.LightwellPackage.AssertNotCalled(t, "ListPackageVersions", mock.Anything, mock.Anything)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsInvalidEcosystem() {
	t := s.T()

	path := fmt.Sprintf("%s/package_versions?ecosystem=bogus", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}
