package lightwell

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type LightwellAdvisorySuite struct {
	LightwellSuite
}

func TestLightwellAdvisorySuite(t *testing.T) {
	suite.Run(t, new(LightwellAdvisorySuite))
}

func (s *LightwellAdvisorySuite) stubLightwellAccess() {
	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{"lightwell-network"}, nil).Maybe()
}

func (s *LightwellAdvisorySuite) TestAdvisoriesRoute() {
	t := s.T()

	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{}, nil).Once()

	path := fmt.Sprintf("%s/advisories", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	assert.Nil(t, err)
	// Route should be registered - we don't care about the exact response code
	assert.NotEqual(t, http.StatusNotFound, code, "Route should be registered")
}

func (s *LightwellAdvisorySuite) TestListAdvisories() {
	t := s.T()
	s.stubLightwellAccess()

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
		return opts.Limit == int32(handler.DefaultLimit) && opts.Offset == 0 &&
			len(opts.EntitledFeatures) == 1 && opts.EntitledFeatures[0] == "lightwell-network"
	})).Return(data, int64(1), nil)

	path := fmt.Sprintf("%s/advisories", LightwellAPIPath)
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
	s.stubLightwellAccess()

	s.reg.LightwellAdvisory.On("ListAdvisories", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellAdvisoriesOptions) bool {
		return opts.PackageName != nil && *opts.PackageName == "spring" &&
			opts.SeverityMin == "important" &&
			opts.Limit == 10 && opts.Offset == 5 &&
			len(opts.EntitledFeatures) == 1 && opts.EntitledFeatures[0] == "lightwell-network"
	})).Return([]api.LightwellAdvisoryResponse{}, int64(0), nil)

	path := fmt.Sprintf("%s/advisories?package_name=spring&severity_min=important&limit=10&offset=5", LightwellAPIPath)
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
	s.stubLightwellAccess()

	s.reg.LightwellAdvisory.On("ListAdvisories", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellAdvisoriesOptions) bool {
		return opts.SeverityMin == "bogus" &&
			len(opts.EntitledFeatures) == 1 && opts.EntitledFeatures[0] == "lightwell-network"
	})).Return(nil, int64(0), fmt.Errorf("invalid severity_min: bogus (must be a label like critical/important/moderate/low or a numeric score)"))

	path := fmt.Sprintf("%s/advisories?severity_min=bogus", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (s *LightwellAdvisorySuite) TestListAdvisoriesFilterByRepoName() {
	t := s.T()
	s.stubLightwellAccess()

	s.reg.LightwellAdvisory.On("ListAdvisories", test.MockCtx(), mock.MatchedBy(func(opts dao.ListLightwellAdvisoriesOptions) bool {
		return opts.RepoName != nil && *opts.RepoName == "java-remediated" &&
			len(opts.EntitledFeatures) == 1 && opts.EntitledFeatures[0] == "lightwell-network"
	})).Return([]api.LightwellAdvisoryResponse{}, int64(0), nil)

	path := fmt.Sprintf("%s/advisories?repository=java-remediated", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (s *LightwellAdvisorySuite) TestListAdvisoriesNoFeatureAccess() {
	t := s.T()

	s.fsClient.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).
		Return([]string{"RHEL-OS-x86_64"}, nil)

	path := fmt.Sprintf("%s/advisories", LightwellAPIPath)
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

func (s *LightwellAdvisorySuite) TestListAdvisoriesEmptyResult() {
	t := s.T()
	s.stubLightwellAccess()

	s.reg.LightwellAdvisory.On("ListAdvisories", test.MockCtx(), mock.Anything).
		Return([]api.LightwellAdvisoryResponse{}, int64(0), nil)

	path := fmt.Sprintf("%s/advisories", LightwellAPIPath)
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
