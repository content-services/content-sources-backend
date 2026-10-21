package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/models"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type NpmPackagesSuite struct {
	Suite
	ctx      context.Context
	server   *http.Server
	identity identity.XRHID
	cancel   context.CancelFunc
}

func (s *NpmPackagesSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ctx, s.cancel = context.WithCancel(context.Background())

	err := db.Connect()
	require.NoError(s.T(), err)

	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))

	handler.RegisterRoutes(s.ctx, router)

	s.server = &http.Server{
		Addr:              "127.0.0.1:8103",
		Handler:           router,
		IdleTimeout:       1 * time.Minute,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	err = config.ConfigureTang()
	require.NoError(s.T(), err)
}

func (s *NpmPackagesSuite) TearDownTest() {
	s.cancel()
	err := s.server.Shutdown(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Could not shutdown server")
	}
	s.Suite.TearDownTest()
}

func (s *NpmPackagesSuite) serveRouter(req *http.Request) (int, []byte, error) {
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(s.T(), err)

	return response.StatusCode, body, err
}

func TestNpmPackagesSuite(t *testing.T) {
	suite.Run(t, new(NpmPackagesSuite))
}

func (s *NpmPackagesSuite) TestNpmPackageVersionsAPI() {
	s.identity = test_handler.MockIdentity
	s.identity.Identity.OrgID = fmt.Sprintf("NpmPackages-%v", rand.Int())

	t := s.T()
	repo := s.createNpmRepository(config.LightwellOrg)
	s.seedNpmPackageMirror(repo.UUID)

	versions := s.getPackageVersions(repo.UUID, "@types", "is-odd")
	assert.Equal(t, "@types", versions.Scope)
	assert.Equal(t, "is-odd", versions.Name)
	require.Len(t, versions.Versions, 3)
	assert.Equal(t, "3.0.0", versions.Versions[0].Version)
	assert.Equal(t, "2024-04-01T00:00:00Z", versions.Versions[0].CreatedAt)
	assert.Equal(t, []string{"3.0.0"}, versions.Versions[0].UpstreamVersions)
	assert.Equal(t, []api.ReleaseInfo{
		{Version: "3.0.0", Release: "rhlw.3", CreatedAt: "2024-04-01T00:00:00Z"},
		{Version: "3.0.0", Release: "rhlw.1", CreatedAt: "2024-01-01T00:00:00Z"},
	}, versions.Versions[0].Builds)
	assert.Empty(t, versions.Versions[0].Tarball.Filename)
	assert.Equal(t, "3.0.10", versions.Versions[1].Version)
	require.Len(t, versions.Versions[1].Builds, 1)
	assert.Equal(t, "1.2.3", versions.Versions[2].Version)
	assert.Equal(t, []string{""}, versions.Versions[2].UpstreamVersions)

	unscoped := s.getPackageVersions(repo.UUID, "-", "left-pad")
	assert.Equal(t, "-", unscoped.Scope)
	assert.Equal(t, "left-pad", unscoped.Name)
	require.Len(t, unscoped.Versions, 1)
	assert.Equal(t, "1.0.0", unscoped.Versions[0].Version)

	missing := s.getPackageVersions(repo.UUID, "@types", "missing")
	assert.Equal(t, "@types", missing.Scope)
	assert.Equal(t, "missing", missing.Name)
	assert.Empty(t, missing.Versions)
}

func (s *NpmPackagesSuite) TestNpmPackageDetailAPI() {
	s.identity = test_handler.MockIdentity
	s.identity.Identity.OrgID = fmt.Sprintf("NpmPackages-%v", rand.Int())

	t := s.T()
	repo := s.createNpmRepository(config.LightwellOrg)
	s.seedNpmPackageMirror(repo.UUID)

	detail := s.getPackageDetail(repo.UUID, "@types", "is-odd", "3.0.0")
	assert.Equal(t, "@types", detail.Scope)
	assert.Equal(t, "is-odd", detail.Name)
	assert.Equal(t, "3.0.0", detail.Version)
	assert.Equal(t, "2024-04-01T00:00:00Z", detail.CreatedAt)
	assert.Equal(t, []string{"3.0.0"}, detail.UpstreamVersions)
	assert.Equal(t, []api.ReleaseInfo{
		{Version: "3.0.0", Release: "rhlw.3", CreatedAt: "2024-04-01T00:00:00Z"},
		{Version: "3.0.0", Release: "rhlw.1", CreatedAt: "2024-01-01T00:00:00Z"},
	}, detail.Builds)
	assert.Empty(t, detail.Tarball.Filename)
	assert.Empty(t, detail.Tarball.Sha256)

	prefixed := s.getPackageDetail(repo.UUID, "@types", "is-odd", "3.0.10")
	require.Len(t, prefixed.Builds, 1)
	assert.Equal(t, "3.0.10", prefixed.Builds[0].Version)

	stored := s.getPackageDetail(repo.UUID, "@types", "is-odd", "1.2.3")
	require.Len(t, stored.Builds, 1)
	assert.Equal(t, "1.2.3", stored.Version)
	assert.Equal(t, []string{""}, stored.UpstreamVersions)
	assert.Equal(t, "", stored.Builds[0].Release)

	missing := s.getPackageDetail(repo.UUID, "@types", "is-odd", "9.9.9")
	assert.Equal(t, "9.9.9", missing.Version)
	assert.Empty(t, missing.Builds)
	assert.Empty(t, missing.UpstreamVersions)
}

func (s *NpmPackagesSuite) createNpmRepository(orgId string) api.RepositoryResponse {
	t := s.T()

	repo := models.Repository{
		URL:         fmt.Sprintf("https://registry.npmjs.org/lightwell-test-%d", rand.Int()),
		Public:      false,
		Origin:      config.OriginExternal,
		ContentType: config.ContentTypeNpm,
	}
	require.NoError(t, db.DB.Create(&repo).Error)

	repoConfig := models.RepositoryConfiguration{
		Name:           fmt.Sprintf("npm-repo-%v", rand.Int()),
		OrgID:          orgId,
		AccountID:      orgId,
		RepositoryUUID: repo.UUID,
		Snapshot:       false,
	}
	require.NoError(t, db.DB.Create(&repoConfig).Error)

	t.Cleanup(func() {
		_ = db.DB.Where("repository_uuid = ?", repo.UUID).Delete(&models.RepositoryConfiguration{}).Error
		_ = db.DB.Delete(&models.Repository{}, "uuid = ?", repo.UUID).Error
	})

	apiRepoResp := s.dao.RepositoryConfig.InternalOnly_FetchRepoConfigsForRepoUUID(context.Background(), repo.UUID)
	require.NotEmpty(t, apiRepoResp)
	return apiRepoResp[0]
}

func (s *NpmPackagesSuite) seedNpmPackageMirror(repoConfigUUID string) {
	t := s.T()

	scoped := models.LightwellPackage{
		RepositoryConfigurationUUID: repoConfigUUID,
		Name:                        "is-odd",
		Group:                       "@types",
	}
	require.NoError(t, db.DB.Create(&scoped).Error)

	unscoped := models.LightwellPackage{
		RepositoryConfigurationUUID: repoConfigUUID,
		Name:                        "left-pad",
		Group:                       "-",
	}
	require.NoError(t, db.DB.Create(&unscoped).Error)

	rows := []models.LightwellPackageVersion{
		{
			LightwellPackageUUID:        scoped.UUID,
			RepositoryConfigurationUUID: repoConfigUUID,
			Version:                     "3.0.0+rhlw.1",
			Release:                     "rhlw.1",
			UpstreamVersion:             "3.0.0",
			PublishedAt:                 "2024-01-01T00:00:00Z",
		},
		{
			LightwellPackageUUID:        scoped.UUID,
			RepositoryConfigurationUUID: repoConfigUUID,
			Version:                     "3.0.0+rhlw.3",
			Release:                     "rhlw.3",
			UpstreamVersion:             "3.0.0",
			PublishedAt:                 "2024-04-01T00:00:00Z",
		},
		{
			LightwellPackageUUID:        scoped.UUID,
			RepositoryConfigurationUUID: repoConfigUUID,
			Version:                     "3.0.10+rhlw.1",
			Release:                     "rhlw.1",
			UpstreamVersion:             "3.0.10",
			PublishedAt:                 "2024-02-01T00:00:00Z",
		},
		{
			LightwellPackageUUID:        scoped.UUID,
			RepositoryConfigurationUUID: repoConfigUUID,
			Version:                     "1.2.3",
			UpstreamVersion:             "",
			PublishedAt:                 "2023-12-01T00:00:00Z",
		},
		{
			LightwellPackageUUID:        unscoped.UUID,
			RepositoryConfigurationUUID: repoConfigUUID,
			Version:                     "1.0.0",
			UpstreamVersion:             "1.0.0",
			PublishedAt:                 "2024-05-01T00:00:00Z",
		},
	}
	for i := range rows {
		require.NoError(t, db.DB.Create(&rows[i]).Error)
	}
}

func (s *NpmPackagesSuite) getPackageVersions(repoUUID, scope, name string) api.NpmPackageVersionsResponse {
	t := s.T()

	path := fmt.Sprintf("%s/repositories/%s/js_packages/%s/%s", api.FullRootPath(), repoUUID, scope, name)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", "application/json")

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code, string(body))

	var resp api.NpmPackageVersionsResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)
	return resp
}

func (s *NpmPackagesSuite) getPackageDetail(repoUUID, scope, name, version string) api.NpmPackageDetailResponse {
	t := s.T()

	path := fmt.Sprintf("%s/repositories/%s/js_packages/%s/%s/%s", api.FullRootPath(), repoUUID, scope, name, version)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", "application/json")

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code, string(body))

	var resp api.NpmPackageDetailResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)
	return resp
}
