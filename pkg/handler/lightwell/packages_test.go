package lightwell

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
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

func (s *LightwellPackagesSuite) stubLightwellAccess() {
	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{"lightwell-maven"}, nil).Maybe()
}

// --- /packages tests ---

func (s *LightwellPackagesSuite) TestListPackagesReturnsDaoRows() {
	t := s.T()
	s.stubLightwellAccess()

	daoRows := []dao.LightwellPackageRow{
		{
			RepositoryConfigurationUUID: "repo-uuid-1",
			RepositoryName:              "lightwell/maven/remediated",
			Ecosystem:                   config.ContentTypeMaven,
			Name:                        "jackson-databind",
			Group:                       "com.fasterxml.jackson.core",
			Versions:                    []string{"2.15.3.rhlw-00001", "2.14.2"},
			Releases:                    []string{"rhlw-00001", "rhlw-00001"},
			PublishedAts:                []string{"2024-06-01T12:00:00Z", "2024-05-01T12:00:00Z"},
			UpstreamVersions:            []string{"2.15.3", "2.14.2"},
			TotalCount:                  2,
		},
		{
			RepositoryConfigurationUUID: "repo-uuid-2",
			RepositoryName:              "lightwell/python/remediated",
			Ecosystem:                   config.ContentTypePython,
			Name:                        "requests",
			Group:                       "",
			Versions:                    []string{"2.31.0"},
			Releases:                    []string{"rhlw-00002"},
			PublishedAts:                []string{"2024-05-10T08:00:00Z"},
			UpstreamVersions:            []string{"2.31.0"},
			TotalCount:                  2,
		},
	}

	s.reg.LightwellPackage.On("ListPackages", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellPackagesOptions) bool {
		return opts.Limit == int32(handler.DefaultLimit) && opts.Offset == 0 &&
			len(opts.EntitledFeatures) == 1 && opts.EntitledFeatures[0] == "lightwell-maven"
	})).Return(daoRows, int64(2), nil)

	path := fmt.Sprintf("%s/packages", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var resp api.LightwellPackageCollectionResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	assert.Equal(t, int64(2), resp.Meta.Count)
	assert.Len(t, resp.Data, 2)

	// First package
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
	assert.NotContains(t, string(body), `"upstream_version"`)

	// Second package
	assert.Equal(t, "requests", resp.Data[1].Name)
	assert.Equal(t, "", resp.Data[1].Group)
	assert.Equal(t, config.ContentTypePython, resp.Data[1].Ecosystem)
}

func (s *LightwellPackagesSuite) TestListPackagesForwardsFilters() {
	t := s.T()
	s.stubLightwellAccess()

	s.reg.LightwellPackage.On("ListPackages", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellPackagesOptions) bool {
		return opts.Ecosystem != nil && *opts.Ecosystem == config.ContentTypeMaven &&
			opts.Name != nil && *opts.Name == "jackson" &&
			opts.Repository != nil && *opts.Repository == "repo1" &&
			opts.SecurityLevel != nil && *opts.SecurityLevel == "validated" &&
			opts.Limit == 10 && opts.Offset == 5 &&
			len(opts.EntitledFeatures) == 1 && opts.EntitledFeatures[0] == "lightwell-maven"
	})).Return([]dao.LightwellPackageRow{}, int64(0), nil)

	path := fmt.Sprintf("%s/packages?ecosystem=maven&name=jackson&repository=repo1&security_level=validated&limit=10&offset=5", LightwellAPIPath)
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

func (s *LightwellPackagesSuite) TestListPackagesEntitlementError() {
	t := s.T()

	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{}, fmt.Errorf("feature service unavailable"))

	path := fmt.Sprintf("%s/packages", LightwellAPIPath)
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
		Return([]string{"other-feature"}, nil)

	path := fmt.Sprintf("%s/packages", LightwellAPIPath)
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

	path := fmt.Sprintf("%s/packages?ecosystem=bogus", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (s *LightwellPackagesSuite) TestListPackagesNoTangPulpCalls() {
	t := s.T()
	s.stubLightwellAccess()

	s.reg.LightwellPackage.On("ListPackages", test.MockCtx(), mock.Anything).
		Return([]dao.LightwellPackageRow{}, int64(0), nil)

	path := fmt.Sprintf("%s/packages", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	_, _, err := s.serveRouter(req)
	require.NoError(t, err)

	// Verify Tang and Pulp clients receive NO calls
	s.tangClient.AssertNotCalled(t, "PythonPackageList", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	s.tangClient.AssertNotCalled(t, "NpmPackageList", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	s.pulpClient.AssertNotCalled(t, "ListMavenPackages", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// --- /package_versions tests ---

func (s *LightwellPackagesSuite) TestListPackageVersionsReturnsDaoRows() {
	t := s.T()
	s.stubLightwellAccess()

	daoRows := []dao.LightwellPackageVersionRow{
		{
			RepositoryConfigurationUUID: "repo-uuid-1",
			RepositoryName:              "lightwell/maven/remediated",
			Ecosystem:                   config.ContentTypeMaven,
			Name:                        "jackson-databind",
			Group:                       "com.fasterxml.jackson.core",
			Version:                     "2.15.3.rhlw-00001",
			Release:                     "rhlw-00001",
			PublishedAt:                 "2024-06-01T12:00:00Z",
			UpstreamVersion:             "2.15.3",
			Purl:                        "pkg:maven/com.fasterxml.jackson.core/jackson-databind@2.15.3",
			TotalCount:                  1,
		},
	}

	s.reg.LightwellPackage.On("ListPackageVersions", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellPackageVersionsOptions) bool {
		return opts.Limit == int32(handler.DefaultLimit) && opts.Offset == 0 &&
			len(opts.EntitledFeatures) == 1 && opts.EntitledFeatures[0] == "lightwell-maven"
	})).Return(daoRows, int64(1), nil)

	path := fmt.Sprintf("%s/package_versions", LightwellAPIPath)
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
	assert.NotContains(t, string(body), `"upstream_version"`)
	assert.Equal(t, config.ContentTypeMaven, resp.Data[0].Ecosystem)
	assert.Equal(t, "lightwell/maven/remediated", resp.Data[0].Repository)
	assert.Equal(t, "repo-uuid-1", resp.Data[0].RepositoryUUID)
	assert.Equal(t, "rhlw-00001", resp.Data[0].Release)
	assert.Equal(t, "2024-06-01T12:00:00Z", resp.Data[0].CreatedAt)
	assert.Equal(t, "pkg:maven/com.fasterxml.jackson.core/jackson-databind@2.15.3", resp.Data[0].Purl)
	assert.Equal(t, "com.fasterxml.jackson.core:jackson-databind", resp.Data[0].Coordinates)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsForwardsCveFilters() {
	t := s.T()
	s.stubLightwellAccess()

	s.reg.LightwellPackage.On("ListPackageVersions", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellPackageVersionsOptions) bool {
		return opts.ResolvesCveID != nil && *opts.ResolvesCveID == "CVE-1" &&
			opts.VulnerableToCveID != nil && *opts.VulnerableToCveID == "CVE-2" &&
			len(opts.EntitledFeatures) == 1 && opts.EntitledFeatures[0] == "lightwell-maven"
	})).Return([]dao.LightwellPackageVersionRow{}, int64(0), nil)

	path := fmt.Sprintf("%s/package_versions?resolves_cve_id=CVE-1&vulnerable_to_cve_id=CVE-2", LightwellAPIPath)
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

func (s *LightwellPackagesSuite) TestListPackageVersionsEntitlementError() {
	t := s.T()

	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{}, fmt.Errorf("feature service unavailable"))

	path := fmt.Sprintf("%s/package_versions", LightwellAPIPath)
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
		Return([]string{"other-feature"}, nil)

	path := fmt.Sprintf("%s/package_versions", LightwellAPIPath)
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

func (s *LightwellPackagesSuite) TestListRepoPackagesReadsMirror() {
	t := s.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440020"

	s.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).
		Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{{
			UUID:        repoUUID,
			ContentType: config.ContentTypeMaven,
		}}}, int64(1), nil)
	s.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, "lib", "last_updated", "asc", 100, 0).
		Return(api.PackageResponse{
			Results: []api.PackageItem{{
				Group: "org.example",
				Name:  "lib",
			}},
			Total:  1,
			Limit:  100,
			Offset: 0,
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages?search=lib&sort_by=last_updated:asc", LightwellAPIPath, repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PackageResponse
	require.NoError(t, json.Unmarshal(body, &response))
	require.Len(t, response.Results, 1)
	assert.Equal(t, "lib", response.Results[0].Name)
}

func (s *LightwellPackagesSuite) TestListPackageVersionsInvalidEcosystem() {
	t := s.T()
	s.stubLightwellAccess()

	path := fmt.Sprintf("%s/package_versions?ecosystem=bogus", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (s *LightwellPackagesSuite) TestGetPythonPackageDetailReadsMirror() {
	t := s.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"
	packageName := "django"
	packageVersion := "5.0"

	s.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).
		Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{{
			UUID:        repoUUID,
			ContentType: config.ContentTypePython,
		}}}, int64(1), nil)
	s.reg.LightwellPackage.On("GetPythonPackageDetail", test.MockCtx(), repoUUID, packageName, packageVersion).
		Return(api.PythonPackageDetailResponse{
			Name:    packageName,
			Version: packageVersion,
			Builds: []api.ReleaseInfo{
				{Version: packageVersion, Release: "rhlw.3", CreatedAt: "2024-02-01T00:00:00Z"},
			},
			Summary:     "A high-level Python web framework",
			Description: "Django is a high-level Python web framework.",
			LastUpdated: "2024-02-01T00:00:00Z",
			License:     "BSD-3-Clause",
			Author: api.PythonPackageAuthor{
				Name:  "Django Software Foundation",
				Email: "foundation@djangoproject.com",
			},
			ProjectURL:      "https://www.djangoproject.com/",
			UpstreamVersion: "5.0",
			Distributions:   []api.PythonDistribution{},
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s/%s", LightwellAPIPath, repoUUID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PythonPackageDetailResponse
	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, packageName, response.Name)
	assert.Equal(t, packageVersion, response.Version)
	assert.Equal(t, "5.0", response.UpstreamVersion)
	require.Len(t, response.Builds, 1)
	assert.Equal(t, "rhlw.3", response.Builds[0].Release)
	assert.Empty(t, response.Distributions)
}

func (s *LightwellPackagesSuite) TestGetMavenPackageDetailReadsMirror() {
	t := s.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440013"
	groupID := "org.example"
	packageName := "lib"
	packageVersion := "5.3.18"

	s.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).
		Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{{
			UUID:        repoUUID,
			ContentType: config.ContentTypeMaven,
		}}}, int64(1), nil)
	s.reg.LightwellPackage.On("GetMavenPackageDetail", test.MockCtx(), repoUUID, groupID, packageName, packageVersion).
		Return(api.MavenPackageDetailResponse{
			Group:   groupID,
			Name:    packageName,
			Version: packageVersion,
			Builds: []api.ReleaseInfo{
				{Version: packageVersion, Release: "rhlw-00003", CreatedAt: "2024-03-03T00:00:00Z"},
			},
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s/%s", LightwellAPIPath, repoUUID, groupID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.MavenPackageDetailResponse
	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, groupID, response.Group)
	assert.Equal(t, packageName, response.Name)
	assert.Equal(t, packageVersion, response.Version)
	require.Len(t, response.Builds, 1)
	assert.Equal(t, "rhlw-00003", response.Builds[0].Release)
}

func (s *LightwellPackagesSuite) TestListMavenPackageVersionsReadsMirror() {
	t := s.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440013"
	groupID := "org.example"
	packageName := "lib"

	s.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).
		Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{{
			UUID:        repoUUID,
			ContentType: config.ContentTypeMaven,
		}}}, int64(1), nil)
	s.reg.LightwellPackage.On("ListMavenPackageVersions", test.MockCtx(), repoUUID, groupID, packageName).
		Return(api.MavenPackageVersionsResponse{
			Group: groupID,
			Name:  packageName,
			Versions: []api.MavenPackageDetailResponse{
				{
					Group:   groupID,
					Name:    packageName,
					Version: "5.3.18",
					Builds: []api.ReleaseInfo{
						{Version: "5.3.18", Release: "rhlw-00003", CreatedAt: "2024-03-03T00:00:00Z"},
						{Version: "5.3.18", Release: "rhlw-00001", CreatedAt: "2024-03-01T00:00:00Z"},
					},
				},
			},
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s", LightwellAPIPath, repoUUID, groupID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.MavenPackageVersionsResponse
	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, groupID, response.Group)
	assert.Equal(t, packageName, response.Name)
	require.Len(t, response.Versions, 1)
	assert.Equal(t, "5.3.18", response.Versions[0].Version)
	require.Len(t, response.Versions[0].Builds, 2)
	assert.Equal(t, "rhlw-00003", response.Versions[0].Builds[0].Release)
	assert.Equal(t, "rhlw-00001", response.Versions[0].Builds[1].Release)
}

func (s *LightwellPackagesSuite) TestGetPythonPackageVersionsReadsMirror() {
	t := s.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"
	packageName := "django"

	s.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).
		Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{{
			UUID:        repoUUID,
			ContentType: config.ContentTypePython,
		}}}, int64(1), nil)
	s.reg.LightwellPackage.On("ListPythonPackageVersions", test.MockCtx(), repoUUID, packageName).
		Return(api.PythonPackageVersionsResponse{
			Name: packageName,
			Versions: []api.PythonPackageDetailResponse{
				{
					Name:            packageName,
					Version:         "4.25.7+rhlw.3",
					LastUpdated:     "2024-01-01T12:00:00Z",
					UpstreamVersion: "4.25.7",
					Distributions:   []api.PythonDistribution{},
				},
			},
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s", LightwellAPIPath, repoUUID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PythonPackageVersionsResponse
	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, packageName, response.Name)
	require.Len(t, response.Versions, 1)
	assert.Equal(t, "4.25.7+rhlw.3", response.Versions[0].Version)
	assert.Equal(t, "4.25.7", response.Versions[0].UpstreamVersion)
	assert.Empty(t, response.Versions[0].Distributions)
}

func (s *LightwellPackagesSuite) TestGetPythonPackageVersionsNotFound() {
	t := s.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"

	s.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).
		Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{{
			UUID:        repoUUID,
			ContentType: config.ContentTypePython,
		}}}, int64(1), nil)
	s.reg.LightwellPackage.On("ListPythonPackageVersions", test.MockCtx(), repoUUID, "missing").
		Return(api.PythonPackageVersionsResponse{}, &ce.DaoError{NotFound: true, Message: "Package not found"})

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s", LightwellAPIPath, repoUUID, "missing")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (s *LightwellPackagesSuite) TestGetPythonPackageDetailNotFound() {
	t := s.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"

	s.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).
		Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{{
			UUID:        repoUUID,
			ContentType: config.ContentTypePython,
		}}}, int64(1), nil)
	s.reg.LightwellPackage.On("GetPythonPackageDetail", test.MockCtx(), repoUUID, "django", "9.9.9").
		Return(api.PythonPackageDetailResponse{
			Name:          "django",
			Version:       "9.9.9",
			Builds:        []api.ReleaseInfo{},
			Distributions: []api.PythonDistribution{},
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s/%s", LightwellAPIPath, repoUUID, "django", "9.9.9")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PythonPackageDetailResponse
	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, "django", response.Name)
	assert.Equal(t, "9.9.9", response.Version)
	assert.Equal(t, []api.ReleaseInfo{}, response.Builds)
}

func (s *LightwellPackagesSuite) TestGetNpmPackageVersionsReadsMirror() {
	t := s.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440014"
	scope := "@types"
	packageName := "is-odd"

	s.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).
		Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{{
			UUID:        repoUUID,
			ContentType: config.ContentTypeNpm,
		}}}, int64(1), nil)
	s.reg.LightwellPackage.On("ListNpmPackageVersions", test.MockCtx(), repoUUID, scope, packageName).
		Return(api.NpmPackageVersionsResponse{
			Scope: scope,
			Name:  packageName,
			Versions: []api.NpmPackageDetailResponse{
				{
					Scope:           scope,
					Name:            packageName,
					Version:         "3.0.0",
					LastUpdated:     "2024-04-01T00:00:00Z",
					UpstreamVersion: "3.0.0",
					Builds: []api.ReleaseInfo{
						{Version: "3.0.0", Release: "rhlw.3", CreatedAt: "2024-04-01T00:00:00Z"},
					},
				},
			},
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/npm_packages/%s/%s", LightwellAPIPath, repoUUID, scope, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.NpmPackageVersionsResponse
	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, scope, response.Scope)
	assert.Equal(t, packageName, response.Name)
	require.Len(t, response.Versions, 1)
	assert.Equal(t, "3.0.0", response.Versions[0].Version)
	require.Len(t, response.Versions[0].Builds, 1)
	assert.Equal(t, "rhlw.3", response.Versions[0].Builds[0].Release)
}

func (s *LightwellPackagesSuite) TestGetNpmPackageDetailReadsMirror() {
	t := s.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440014"
	scope := "@types"
	packageName := "is-odd"
	packageVersion := "3.0.0"

	s.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).
		Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{{
			UUID:        repoUUID,
			ContentType: config.ContentTypeNpm,
		}}}, int64(1), nil)
	s.reg.LightwellPackage.On("GetNpmPackageDetail", test.MockCtx(), repoUUID, scope, packageName, packageVersion).
		Return(api.NpmPackageDetailResponse{
			Scope:           scope,
			Name:            packageName,
			Version:         packageVersion,
			LastUpdated:     "2024-04-01T00:00:00Z",
			UpstreamVersion: "3.0.0",
			Builds: []api.ReleaseInfo{
				{Version: packageVersion, Release: "rhlw.3", CreatedAt: "2024-04-01T00:00:00Z"},
			},
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/npm_packages/%s/%s/%s", LightwellAPIPath, repoUUID, scope, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.NpmPackageDetailResponse
	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, scope, response.Scope)
	assert.Equal(t, packageName, response.Name)
	assert.Equal(t, packageVersion, response.Version)
	assert.Equal(t, "2024-04-01T00:00:00Z", response.LastUpdated)
	assert.Equal(t, "3.0.0", response.UpstreamVersion)
	require.Len(t, response.Builds, 1)
	assert.Equal(t, "rhlw.3", response.Builds[0].Release)
	assert.Empty(t, response.Tarball.Filename)
}
