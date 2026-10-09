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
