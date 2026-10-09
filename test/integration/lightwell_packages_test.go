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
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/external_repos/commands"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/models"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type LightwellPackagesSuite struct {
	suite.Suite
	dao    *dao.DaoRegistry
	fsMock *feature_service_client.MockFeatureServiceClient

	mavenRepoUUID   string
	mavenConfigUUID string
	mavenName       string
	mavenGroup      string
	mavenArtifact   string

	pythonRepoUUID   string
	pythonConfigUUID string
	pythonName       string
	pythonPackage    string
}

func TestLightwellPackagesSuite(t *testing.T) {
	suite.Run(t, new(LightwellPackagesSuite))
}

func (s *LightwellPackagesSuite) SetupSuite() {
	if db.LightwellQueries == nil {
		s.Require().NoError(db.Connect())
	}
	s.dao = dao.GetDaoRegistry(db.DB)
}

func (s *LightwellPackagesSuite) SetupTest() {
	s.Require().NotNil(db.LightwellQueries)
	s.fsMock = feature_service_client.NewMockFeatureServiceClient(s.T())
	s.fsMock.On("GetEntitledFeatures", mock.Anything, test_handler.MockOrgId).
		Return([]string{"lightwell-maven", "lightwell-python"}, nil)
}

func (s *LightwellPackagesSuite) TearDownTest() {
	for _, id := range []string{s.mavenConfigUUID, s.pythonConfigUUID} {
		if id == "" {
			continue
		}
		_ = db.DB.Where("repository_configuration_uuid = ?", id).Delete(&models.LightwellPackageVersion{}).Error
		_ = db.DB.Where("repository_configuration_uuid = ?", id).Delete(&models.LightwellPackage{}).Error
		_ = db.DB.Where("uuid = ?", id).Delete(&models.RepositoryConfiguration{}).Error
	}
	for _, id := range []string{s.mavenRepoUUID, s.pythonRepoUUID} {
		if id == "" {
			continue
		}
		_ = db.DB.Where("uuid = ?", id).Delete(&models.Repository{}).Error
	}
	if s.mavenGroup != "" {
		_ = db.DB.Where("group_id = ? AND name = ?", s.mavenGroup, s.mavenArtifact).Delete(&models.MavenPackage{}).Error
	}
}

func (s *LightwellPackagesSuite) TestMavenImportAndList() {
	suffix := time.Now().UnixNano()
	s.mavenName = fmt.Sprintf("java/validated-int-%d", suffix)
	s.mavenGroup = fmt.Sprintf("org.example.int%d", suffix)
	s.mavenArtifact = "widget"
	basePath := fmt.Sprintf("int/maven/%d", suffix)
	repoHref := fmt.Sprintf("/pulp/api/v3/repositories/maven/maven/%d/", suffix)
	versionHref := repoHref + "versions/1/"

	s.mavenRepoUUID, s.mavenConfigUUID = s.createLightwellRepo(s.mavenName, config.ContentTypeMaven, "lightwell-maven", basePath)

	summary := "Widget summary"
	license := "Apache-2.0"
	author := "Example Org"
	projectURL := "https://example.com/widget"
	s.Require().NoError(db.DB.Create(&models.MavenPackage{
		GroupID:    s.mavenGroup,
		Name:       s.mavenArtifact,
		Summary:    &summary,
		License:    &license,
		Author:     &author,
		ProjectURL: &projectURL,
	}).Error)

	pulp := pulp_client.NewMockPulpClient(s.T())
	pulp.On("ResolveRepositoryFromBasePath", mock.Anything, basePath).Return(&repoHref, nil).Twice()
	pulp.On("GetLatestVersionHref", mock.Anything, repoHref).Return(&versionHref, nil).Twice()
	pulp.On("ListMavenFlatPackages", mock.Anything, repoHref).Return(zest.PaginatedMavenRepositoryFlatPackageResponseList{
		Count: 2,
		Results: []zest.MavenRepositoryFlatPackageResponse{
			{
				GroupId:     s.mavenGroup,
				ArtifactId:  s.mavenArtifact,
				Version:     "1.2.3",
				LastUpdated: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				Description: "from pulp",
			},
			{
				GroupId:     s.mavenGroup,
				ArtifactId:  s.mavenArtifact,
				Version:     "1.2.3.rhlw-00001",
				LastUpdated: time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC),
				Description: "from pulp",
			},
		},
	}, nil).Once()

	repo := s.repoToImport(s.mavenConfigUUID, s.mavenName, config.ContentTypeMaven, basePath)
	s.Require().NoError(commands.ImportLightwellRepo(context.Background(), db.DB, pulp, repo, false))
	// A later run loads the stored href. When it still matches Pulp, the catalog is not read again.
	repo.LastImportRepositoryVersion = versionHref
	s.Require().NoError(commands.ImportLightwellRepo(context.Background(), db.DB, pulp, repo, false))

	var stored models.RepositoryConfiguration
	s.Require().NoError(db.DB.Where("uuid = ?", s.mavenConfigUUID).First(&stored).Error)
	s.Equal(versionHref, stored.LastImportRepositoryVersion)

	var versions []models.LightwellPackageVersion
	s.Require().NoError(db.DB.Where("repository_configuration_uuid = ?", s.mavenConfigUUID).Order("version").Find(&versions).Error)
	s.Require().Len(versions, 2)
	for _, ver := range versions {
		s.Equal("1.2.3", ver.UpstreamVersion)
		s.Equal(summary, ver.Summary)
		s.Equal(summary, ver.Description)
		s.Equal(license, ver.License)
		s.Equal(author, ver.Author)
		s.Equal("", ver.AuthorEmail)
		s.Equal(projectURL, ver.ProjectURL)
	}
	s.Equal("1.2.3", versions[0].Version)
	s.Equal("", versions[0].Release)
	s.Equal("1.2.3.rhlw-00001", versions[1].Version)
	s.Equal("rhlw-00001", versions[1].Release)

	q := url.Values{}
	q.Set("repository", s.mavenName)
	q.Set("ecosystem", config.ContentTypeMaven)
	q.Set("name", s.mavenArtifact)

	code, body := s.serve(s.get("/lightwell/packages", q))
	s.Equal(http.StatusOK, code)
	s.assertNoMetadata(body)

	var packages api.LightwellPackageCollectionResponse
	s.Require().NoError(json.Unmarshal(body, &packages))
	s.Equal(int64(1), packages.Meta.Count)
	s.Require().Len(packages.Data, 1)
	pkg := packages.Data[0]
	s.Equal(s.mavenArtifact, pkg.Name)
	s.Equal(s.mavenGroup, pkg.Group)
	s.Equal(config.ContentTypeMaven, pkg.Ecosystem)
	s.Equal(s.mavenName, pkg.Repository)
	s.Equal(s.mavenConfigUUID, pkg.RepositoryUUID)
	s.Equal([]string{"1.2.3", "1.2.3"}, pkg.Versions)
	s.Require().Len(pkg.LatestReleases, 2)
	s.Equal("1.2.3", pkg.LatestReleases[0].Version)
	s.Equal("", pkg.LatestReleases[0].Release)
	s.Equal("2020-01-01T00:00:00Z", pkg.LatestReleases[0].CreatedAt)
	s.Equal("1.2.3", pkg.LatestReleases[1].Version)
	s.Equal("rhlw-00001", pkg.LatestReleases[1].Release)

	code, body = s.serve(s.get("/lightwell/package_versions", q))
	s.Equal(http.StatusOK, code)
	s.assertNoMetadata(body)

	var versionResp api.LightwellPackageVersionCollectionResponse
	s.Require().NoError(json.Unmarshal(body, &versionResp))
	s.Equal(int64(2), versionResp.Meta.Count)
	s.Require().Len(versionResp.Data, 2)
	s.Equal("1.2.3", versionResp.Data[0].Version)
	s.Equal("", versionResp.Data[0].Release)
	s.Equal(fmt.Sprintf("pkg:maven/%s/%s@1.2.3", s.mavenGroup, s.mavenArtifact), versionResp.Data[0].Purl)
	s.Equal(s.mavenGroup+":"+s.mavenArtifact, versionResp.Data[0].Coordinates)
	s.Equal("1.2.3", versionResp.Data[1].Version)
	s.Equal("rhlw-00001", versionResp.Data[1].Release)
	s.NotContains(string(body), `"release":""`)
}

func (s *LightwellPackagesSuite) TestPythonImportAndList() {
	suffix := time.Now().UnixNano()
	s.pythonName = fmt.Sprintf("python/validated-int-%d", suffix)
	s.pythonPackage = fmt.Sprintf("pyint%d", suffix)
	basePath := fmt.Sprintf("int/python/%d", suffix)
	repoHref := fmt.Sprintf("/pulp/api/v3/repositories/python/python/%d/", suffix)
	versionHref := repoHref + "versions/1/"

	s.pythonRepoUUID, s.pythonConfigUUID = s.createLightwellRepo(s.pythonName, config.ContentTypePython, "lightwell-python", basePath)

	origTang := config.Tang
	mockTang := tangy.NewMockTangy(s.T())
	var tangClient tangy.Tangy = mockTang
	config.Tang = &tangClient
	s.T().Cleanup(func() { config.Tang = origTang })

	mockTang.On("PythonPackageDetailList", mock.Anything, repoHref, tangy.PageOptions{
		Offset: 0,
		Limit:  tangy.PythonPackageDetailListMaxLimit,
	}).Return(tangy.PythonPackageDetailListResponse{
		Total: 1,
		Results: []tangy.PythonPackageDetailListItem{{
			NameNormalized: s.pythonPackage,
			Versions: []tangy.PythonPackageVersionDetail{{
				Version:           "1.2.3+rhlw.1",
				LastUpdated:       "2021-01-01T00:00:00Z",
				Summary:           "HTTP library",
				Description:       "HTTP library for humans",
				LicenseExpression: "Apache-2.0",
				License:           "Apache Software License",
				Author:            "Kenneth",
				AuthorEmail:       "kenneth@example.com",
				ProjectURL:        "https://example.com/requests",
			}},
		}},
	}, nil).Once()

	pulp := pulp_client.NewMockPulpClient(s.T())
	pulp.On("ResolveRepositoryFromBasePath", mock.Anything, basePath).Return(&repoHref, nil)
	pulp.On("GetLatestVersionHref", mock.Anything, repoHref).Return(&versionHref, nil)

	repo := s.repoToImport(s.pythonConfigUUID, s.pythonName, config.ContentTypePython, basePath)
	s.Require().NoError(commands.ImportLightwellRepo(context.Background(), db.DB, pulp, repo, false))

	var versions []models.LightwellPackageVersion
	s.Require().NoError(db.DB.Where("repository_configuration_uuid = ?", s.pythonConfigUUID).Find(&versions).Error)
	s.Require().Len(versions, 1)
	s.Equal("1.2.3+rhlw.1", versions[0].Version)
	s.Equal("1.2.3", versions[0].UpstreamVersion)
	s.Equal("rhlw.1", versions[0].Release)
	s.Equal("HTTP library", versions[0].Summary)
	s.Equal("HTTP library for humans", versions[0].Description)
	s.Equal("Apache-2.0", versions[0].License)
	s.Equal("Kenneth", versions[0].Author)
	s.Equal("kenneth@example.com", versions[0].AuthorEmail)
	s.Equal("https://example.com/requests", versions[0].ProjectURL)

	q := url.Values{}
	q.Set("repository", s.pythonName)
	q.Set("ecosystem", config.ContentTypePython)
	q.Set("name", s.pythonPackage)

	code, body := s.serve(s.get("/lightwell/packages", q))
	s.Equal(http.StatusOK, code)
	s.assertNoMetadata(body)

	var packages api.LightwellPackageCollectionResponse
	s.Require().NoError(json.Unmarshal(body, &packages))
	s.Equal(int64(1), packages.Meta.Count)
	s.Require().Len(packages.Data, 1)
	s.Equal(s.pythonPackage, packages.Data[0].Name)
	s.Equal("", packages.Data[0].Group)
	s.Equal([]string{"1.2.3"}, packages.Data[0].Versions)
	s.Require().Len(packages.Data[0].LatestReleases, 1)
	s.Equal("1.2.3", packages.Data[0].LatestReleases[0].Version)
	s.Equal("rhlw.1", packages.Data[0].LatestReleases[0].Release)

	code, body = s.serve(s.get("/lightwell/package_versions", q))
	s.Equal(http.StatusOK, code)
	s.assertNoMetadata(body)

	var versionResp api.LightwellPackageVersionCollectionResponse
	s.Require().NoError(json.Unmarshal(body, &versionResp))
	s.Equal(int64(1), versionResp.Meta.Count)
	s.Require().Len(versionResp.Data, 1)
	row := versionResp.Data[0]
	s.Equal("1.2.3", row.Version)
	s.Equal("rhlw.1", row.Release)
	s.Equal(s.pythonPackage, row.Coordinates)
	s.Equal("pkg:pypi/"+s.pythonPackage+"@1.2.3+rhlw.1", row.Purl)
	s.Equal("2021-01-01T00:00:00Z", row.CreatedAt)
}

func (s *LightwellPackagesSuite) createLightwellRepo(name, contentType, feature, basePath string) (string, string) {
	repo := models.Repository{
		Origin:                  config.OriginLightwell,
		ContentType:             contentType,
		SecurityLevel:           "validated",
		PublishedDistBasePath:   basePath,
		LastIntrospectionStatus: config.StatusValid,
	}
	s.Require().NoError(db.DB.Create(&repo).Error)

	repoConfig := models.RepositoryConfiguration{
		Name:           name,
		OrgID:          config.LightwellOrg,
		RepositoryUUID: repo.UUID,
		FeatureName:    feature,
	}
	s.Require().NoError(db.DB.Create(&repoConfig).Error)
	return repo.UUID, repoConfig.UUID
}

func (s *LightwellPackagesSuite) repoToImport(configUUID, name, contentType, basePath string) dao.LightwellRepoToImport {
	return dao.LightwellRepoToImport{
		RepoConfigUUID: configUUID,
		OrgID:          config.LightwellOrg,
		Name:           name,
		ContentType:    contentType,
		BasePath:       basePath,
	}
}

func (s *LightwellPackagesSuite) serve(req *http.Request) (int, []byte) {
	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	var fsClient feature_service_client.FeatureServiceClient = s.fsMock
	handler.RegisterLightwellPackageRoutes(router.Group(api.FullRootPath()), s.dao, nil, nil, &fsClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	response := rr.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	s.Require().NoError(err)
	return response.StatusCode, body
}

func (s *LightwellPackagesSuite) get(path string, query url.Values) *http.Request {
	full := fmt.Sprintf("%s%s", api.FullRootPath(), path)
	if query != nil {
		full = full + "?" + query.Encode()
	}
	req := httptest.NewRequest(http.MethodGet, full, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(s.T()))
	return req
}

func (s *LightwellPackagesSuite) assertNoMetadata(body []byte) {
	raw := string(body)
	for _, key := range []string{`"summary"`, `"description"`, `"author"`, `"author_email"`, `"license"`, `"project_url"`, `"upstream_version"`} {
		s.NotContains(raw, key)
	}
}
