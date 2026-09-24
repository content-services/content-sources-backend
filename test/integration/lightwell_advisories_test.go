package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/models"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type LightwellAdvisoriesSuite struct {
	suite.Suite
	dao        *dao.DaoRegistry
	fsMock     *feature_service_client.MockFeatureServiceClient
	repoName   string
	repoUUID   string
	configUUID string
}

func TestLightwellAdvisoriesSuite(t *testing.T) {
	suite.Run(t, new(LightwellAdvisoriesSuite))
}

func (s *LightwellAdvisoriesSuite) SetupSuite() {
	if db.LightwellQueries == nil {
		s.Require().NoError(db.Connect())
	}
	s.dao = dao.GetDaoRegistry(db.DB)
}

func (s *LightwellAdvisoriesSuite) SetupTest() {
	s.Require().NotNil(db.LightwellQueries)
	s.fsMock = feature_service_client.NewMockFeatureServiceClient(s.T())

	suffix := time.Now().UnixNano()
	s.repoName = fmt.Sprintf("lightwell/java/int-advisories-%d", suffix)

	repo := models.Repository{
		Origin:                  config.OriginLightwell,
		ContentType:             config.ContentTypeMaven,
		LastIntrospectionStatus: config.StatusValid,
	}
	s.Require().NoError(db.DB.Create(&repo).Error)
	s.repoUUID = repo.UUID

	repoConfig := models.RepositoryConfiguration{
		Name:           s.repoName,
		OrgID:          config.LightwellOrg,
		RepositoryUUID: repo.UUID,
		FeatureName:    "lightwell-network",
	}
	s.Require().NoError(db.DB.Create(&repoConfig).Error)
	s.configUUID = repoConfig.UUID

	published := time.Date(2026, 9, 17, 18, 39, 42, 0, time.UTC)
	err := s.dao.LightwellAdvisory.SyncForRepository(context.Background(), s.configUUID, s.repoName, []dao.LightwellAdvisoryInput{
		{
			AdvisoryID:     "x_RHLW-CVE-2015-6748-1.7.2",
			Severity:       "6.1",
			Summary:        "Improper Neutralization of Input During Web Page Generation in Jsoup",
			Details:        "Cross-site scripting (XSS) vulnerability in jsoup before 1.8.3.",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.7.2",
			FixedVersions:  []string{"1.7.2.rhlw-00001"},
			Checksum:       "checksum-jsoup",
			Published:      &published,
			Modified:       &published,
			Aliases:        []string{"GHSA-48rh-qgjr-xfj6", "CVE-2015-6748"},
			SchemaVersion:  "1.6.8",
			Source:         "pnc-build",
		},
		{
			AdvisoryID:     "x_RHLW-LW-2026-4255-2.11.0",
			Severity:       "4.0",
			Summary:        "Other advisory",
			Details:        "Unrelated package advisory",
			PackageName:    "com.example:other",
			PackageVersion: "2.11.0",
			FixedVersions:  []string{"2.11.0.rhlw-00000"},
			Checksum:       "checksum-other",
			Aliases:        []string{"LW-2026-4255"},
			SchemaVersion:  "1.6.8",
			Source:         "pnc-build",
		},
	})
	s.Require().NoError(err)
}

func (s *LightwellAdvisoriesSuite) TearDownTest() {
	if s.configUUID != "" {
		_ = db.DB.Where("repository_configuration_uuid = ?", s.configUUID).Delete(&models.LightwellAdvisory{}).Error
		_ = db.DB.Where("uuid = ?", s.configUUID).Delete(&models.RepositoryConfiguration{}).Error
	}
	if s.repoUUID != "" {
		_ = db.DB.Where("uuid = ?", s.repoUUID).Delete(&models.Repository{}).Error
	}
}

func (s *LightwellAdvisoriesSuite) stubLightwellAccess() {
	s.fsMock.On("GetEntitledFeatures", mock.Anything, test_handler.MockOrgId).
		Return([]string{"lightwell-network"}, nil)
}

func (s *LightwellAdvisoriesSuite) serve(req *http.Request) (int, []byte) {
	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	var fsClient feature_service_client.FeatureServiceClient = s.fsMock
	handler.RegisterLightwellAdvisoryRoutes(router.Group(api.FullRootPath()), s.dao, &fsClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	response := rr.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	s.Require().NoError(err)
	return response.StatusCode, body
}

func (s *LightwellAdvisoriesSuite) get(query url.Values) *http.Request {
	path := fmt.Sprintf("%s/lightwell/advisories", api.FullRootPath())
	if query != nil {
		path = path + "?" + query.Encode()
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(s.T()))
	return req
}

func (s *LightwellAdvisoriesSuite) list(query url.Values) api.LightwellAdvisoryCollectionResponse {
	code, body := s.serve(s.get(query))
	s.Equal(http.StatusOK, code)

	var resp api.LightwellAdvisoryCollectionResponse
	s.Require().NoError(json.Unmarshal(body, &resp))
	s.NotNil(resp.Data)
	return resp
}

func (s *LightwellAdvisoriesSuite) repoQuery() url.Values {
	q := url.Values{}
	q.Set("repository", s.repoName)
	return q
}

func (s *LightwellAdvisoriesSuite) TestListAdvisories() {
	s.stubLightwellAccess()

	code, body := s.serve(s.get(s.repoQuery()))
	s.Equal(http.StatusOK, code)
	s.NotContains(string(body), `"osv_url"`)

	var resp api.LightwellAdvisoryCollectionResponse
	s.Require().NoError(json.Unmarshal(body, &resp))
	s.Equal(int64(2), resp.Meta.Count)
	s.Len(resp.Data, 2)

	byID := map[string]api.LightwellAdvisoryResponse{}
	for _, row := range resp.Data {
		byID[row.AdvisoryID] = row
	}

	jsoup := byID["x_RHLW-CVE-2015-6748-1.7.2"]
	s.Equal("CVE-2015-6748", jsoup.AdvisoryName)
	s.Equal("6.1", jsoup.Severity)
	s.Equal(float32(6.1), jsoup.SeverityScore)
	s.Equal("Improper Neutralization of Input During Web Page Generation in Jsoup", jsoup.Summary)
	s.Equal("Cross-site scripting (XSS) vulnerability in jsoup before 1.8.3.", jsoup.Details)
	s.Equal("org.jsoup:jsoup", jsoup.PackageName)
	s.Equal("1.7.2", jsoup.PackageVersion)
	s.Equal([]string{"1.7.2.rhlw-00001"}, jsoup.FixedVersions)
	s.Equal(s.repoName, jsoup.Repository)
	s.Equal([]string{"GHSA-48rh-qgjr-xfj6", "CVE-2015-6748"}, jsoup.Aliases)
	s.Equal("1.6.8", jsoup.SchemaVersion)
	s.Equal("pnc-build", jsoup.Source)
	s.Require().NotNil(jsoup.Published)
	s.False(jsoup.CreatedAt.IsZero())
	s.False(jsoup.UpdatedAt.IsZero())

	other := byID["x_RHLW-LW-2026-4255-2.11.0"]
	s.Equal("LW-2026-4255", other.AdvisoryName)
	s.Equal("com.example:other", other.PackageName)
}

func (s *LightwellAdvisoriesSuite) TestListAdvisoriesFilters() {
	s.stubLightwellAccess()

	q := s.repoQuery()
	q.Set("name", "GHSA-48rh")
	resp := s.list(q)
	s.Equal(int64(1), resp.Meta.Count)
	s.Require().Len(resp.Data, 1)
	s.Equal("org.jsoup:jsoup", resp.Data[0].PackageName)

	q = s.repoQuery()
	q.Set("name", "CVE-2015-6748")
	resp = s.list(q)
	s.Equal(int64(1), resp.Meta.Count)
	s.Equal("x_RHLW-CVE-2015-6748-1.7.2", resp.Data[0].AdvisoryID)

	q = s.repoQuery()
	q.Set("package_version", "1.7.2")
	resp = s.list(q)
	s.Equal(int64(1), resp.Meta.Count)
	s.Equal("org.jsoup:jsoup", resp.Data[0].PackageName)

	q = s.repoQuery()
	q.Set("name", "CVE-2015-6748")
	q.Set("package_version", "1.7.2")
	q.Set("package_name", "jsoup")
	resp = s.list(q)
	s.Equal(int64(1), resp.Meta.Count)
	s.Equal("org.jsoup:jsoup", resp.Data[0].PackageName)

	q = s.repoQuery()
	q.Set("name", "CVE-2015-6748")
	q.Set("package_version", "2.11.0")
	resp = s.list(q)
	s.Equal(int64(0), resp.Meta.Count)
	s.Empty(resp.Data)

	q = s.repoQuery()
	q.Set("cve_id", "CVE-2015-6748")
	resp = s.list(q)
	s.Equal(int64(0), resp.Meta.Count)
	s.Empty(resp.Data)

	q = s.repoQuery()
	q.Set("cve_id", "x_RHLW-CVE-2015-6748-1.7.2")
	resp = s.list(q)
	s.Equal(int64(1), resp.Meta.Count)
	s.Equal("org.jsoup:jsoup", resp.Data[0].PackageName)
}

func (s *LightwellAdvisoriesSuite) TestListAdvisoriesLatestRelease() {
	s.stubLightwellAccess()

	err := s.dao.LightwellAdvisory.SyncForRepository(context.Background(), s.configUUID, s.repoName, []dao.LightwellAdvisoryInput{
		{
			AdvisoryID:     "x_RHLW-CVE-2015-0001-1.2.3",
			Severity:       "6.1",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.2.3",
			FixedVersions:  []string{"1.2.3.rhlw.00003"},
			Checksum:       "checksum-jsoup-base",
		},
		{
			AdvisoryID:     "x_RHLW-CVE-2015-0002-1.2.3",
			Severity:       "5.0",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.2.3",
			FixedVersions:  []string{"1.2.3.rhlw.00003.n00001"},
			Checksum:       "checksum-jsoup-novel",
		},
		{
			AdvisoryID:     "x_RHLW-CVE-2015-0003-1.2.3",
			Severity:       "4.0",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.2.3",
			FixedVersions:  []string{"1.2.3.rhlw.00003.n00001.hf00001"},
			Checksum:       "checksum-jsoup-hotfix",
		},
		{
			AdvisoryID:     "x_RHLW-CVE-2015-0004-1.2.3",
			Severity:       "7.0",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.2.3",
			FixedVersions:  []string{"1.2.3.rhlw.00004"},
			Checksum:       "checksum-jsoup-next",
		},
		{
			AdvisoryID:     "x_RHLW-LW-2026-4255-2.11.0",
			Severity:       "4.0",
			PackageName:    "com.example:other",
			PackageVersion: "2.11.0",
			FixedVersions:  []string{"2.11.0.rhlw-00000"},
			Checksum:       "checksum-other",
		},
	})
	s.Require().NoError(err)

	q := s.repoQuery()
	q.Set("package_name", "jsoup")
	q.Set("package_version", "1.2.3")
	resp := s.list(q)
	s.Equal(int64(4), resp.Meta.Count)

	q.Set("latest_release", "true")
	resp = s.list(q)
	s.Equal(int64(1), resp.Meta.Count)
	s.Require().Len(resp.Data, 1)
	s.Equal("x_RHLW-CVE-2015-0004-1.2.3", resp.Data[0].AdvisoryID)

	code, body := s.serve(s.get(url.Values{"latest_release": []string{"true"}}))
	s.Equal(http.StatusBadRequest, code)
	s.Contains(string(body), "latest_release requires package_name and package_version")
}

func (s *LightwellAdvisoriesSuite) TestListAdvisoriesNoFeatureAccess() {
	s.fsMock.On("GetEntitledFeatures", mock.Anything, test_handler.MockOrgId).
		Return([]string{"RHEL-OS-x86_64"}, nil)

	resp := s.list(s.repoQuery())
	s.Equal(int64(0), resp.Meta.Count)
	s.Empty(resp.Data)
}

func (s *LightwellAdvisoriesSuite) TestListAdvisoriesUpdatesAfterResync() {
	s.stubLightwellAccess()

	resp := s.list(s.repoQuery())
	s.Equal(int64(2), resp.Meta.Count)
	jsoup := findAdvisory(resp.Data, "x_RHLW-CVE-2015-6748-1.7.2")
	s.Require().NotNil(jsoup)
	s.Equal("Improper Neutralization of Input During Web Page Generation in Jsoup", jsoup.Summary)

	updatedPublished := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	err := s.dao.LightwellAdvisory.SyncForRepository(context.Background(), s.configUUID, s.repoName, []dao.LightwellAdvisoryInput{
		{
			AdvisoryID:     "x_RHLW-CVE-2015-6748-1.7.2",
			Severity:       "9.8",
			Summary:        "Updated jsoup summary",
			Details:        "Updated jsoup details",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.8.3",
			FixedVersions:  []string{"1.8.3.rhlw-00002"},
			Checksum:       "checksum-jsoup-updated",
			Published:      &updatedPublished,
			Modified:       &updatedPublished,
			Aliases:        []string{"GHSA-48rh-qgjr-xfj6", "CVE-2015-6748", "CVE-EXTRA"},
			SchemaVersion:  "1.7.0",
			Source:         "manual",
		},
		{
			AdvisoryID:     "x_RHLW-LW-2026-4255-2.11.0",
			Severity:       "4.0",
			Summary:        "Other advisory",
			Details:        "Unrelated package advisory",
			PackageName:    "com.example:other",
			PackageVersion: "2.11.0",
			FixedVersions:  []string{"2.11.0.rhlw-00000"},
			Checksum:       "checksum-other",
			Aliases:        []string{"LW-2026-4255"},
			SchemaVersion:  "1.6.8",
			Source:         "pnc-build",
		},
	})
	s.Require().NoError(err)

	resp = s.list(s.repoQuery())
	s.Equal(int64(2), resp.Meta.Count)
	jsoup = findAdvisory(resp.Data, "x_RHLW-CVE-2015-6748-1.7.2")
	s.Require().NotNil(jsoup)
	s.Equal("Updated jsoup summary", jsoup.Summary)
	s.Equal("Updated jsoup details", jsoup.Details)
	s.Equal("9.8", jsoup.Severity)
	s.Equal(float32(9.8), jsoup.SeverityScore)
	s.Equal("1.8.3", jsoup.PackageVersion)
	s.Equal([]string{"1.8.3.rhlw-00002"}, jsoup.FixedVersions)
	s.Equal("manual", jsoup.Source)
	s.Equal("1.7.0", jsoup.SchemaVersion)
	s.Equal([]string{"GHSA-48rh-qgjr-xfj6", "CVE-2015-6748", "CVE-EXTRA"}, jsoup.Aliases)
	s.Require().NotNil(jsoup.Published)
	s.True(updatedPublished.Equal(*jsoup.Published))
}

func findAdvisory(rows []api.LightwellAdvisoryResponse, advisoryID string) *api.LightwellAdvisoryResponse {
	for i := range rows {
		if rows[i].AdvisoryID == advisoryID {
			return &rows[i]
		}
	}
	return nil
}
