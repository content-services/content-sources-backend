package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/models"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	testPythonPackageName = "shelf-reader"
	testPythonPackageURL  = "https://pypi.org/"
)

type PythonPackagesSuite struct {
	Suite
	ctx      context.Context
	server   *http.Server
	identity identity.XRHID
	cancel   context.CancelFunc
}

func (s *PythonPackagesSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ctx, s.cancel = context.WithCancel(context.Background())

	config.Get().Features.Snapshots.Enabled = true

	err := db.Connect()
	require.NoError(s.T(), err)

	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))

	handler.RegisterRoutes(s.ctx, router)

	s.server = &http.Server{
		Addr:              "127.0.0.1:8102",
		Handler:           router,
		IdleTimeout:       1 * time.Minute,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	config.Get().Clients.Pulp.StorageType = "local"

	err = config.ConfigureTang()
	require.NoError(s.T(), err)
}

func (s *PythonPackagesSuite) TearDownTest() {
	s.cancel()
	err := s.server.Shutdown(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Could not shutdown server")
	}
	s.Suite.TearDownTest()
}

func (s *PythonPackagesSuite) serveRouter(req *http.Request) (int, []byte, error) {
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(s.T(), err)

	return response.StatusCode, body, err
}

func (s *PythonPackagesSuite) getZestClient() (context.Context, *zest.APIClient, error) {
	ctx2 := context.WithValue(s.ctx, zest.ContextServerIndex, 0)
	httpClient, err := config.GetHTTPClient(&config.PulpCertUser{}, config.Get().Clients.Pulp.Username == "")
	if err != nil {
		return nil, nil, err
	}

	pulpConfig := zest.NewConfiguration()
	pulpConfig.HTTPClient = &httpClient
	pulpConfig.Servers = zest.ServerConfigurations{zest.ServerConfiguration{
		URL: config.Get().Clients.Pulp.Server,
	}}
	client := zest.NewAPIClient(pulpConfig)

	if config.Get().Clients.Pulp.Username != "" {
		ctx2 = context.WithValue(ctx2, zest.ContextBasicAuth, zest.BasicAuth{
			UserName: config.Get().Clients.Pulp.Username,
			Password: config.Get().Clients.Pulp.Password,
		})
	}

	return ctx2, client, nil
}

func TestPythonPackagesSuite(t *testing.T) {
	suite.Run(t, new(PythonPackagesSuite))
}

func (s *PythonPackagesSuite) TestPythonPackagesAPI() {
	orgId := fmt.Sprintf("PythonPackages-%v", rand.Int())

	s.identity = test_handler.MockIdentity
	s.identity.Identity.OrgID = orgId

	t := s.T()

	repo := s.createPythonRepository(config.LightwellOrg)
	packages := s.listPackages(repo.UUID)

	assert.NotNil(t, packages)
	assert.GreaterOrEqual(t, packages.Total, 1)
	require.NotEmpty(t, packages.Results)

	var shelfReader *api.PackageItem
	for i := range packages.Results {
		if packages.Results[i].Name == testPythonPackageName {
			shelfReader = &packages.Results[i]
			break
		}
	}
	require.NotNil(t, shelfReader, "expected shelf-reader package in results")
	assert.Empty(t, shelfReader.Group)
	assert.Contains(t, shelfReader.Versions, "0.1")
	require.NotEmpty(t, shelfReader.LatestReleases)

	foundVersion := false
	for _, release := range shelfReader.LatestReleases {
		if release.Version == "0.1" {
			foundVersion = true
			assert.NotEmpty(t, release.CreatedAt)
			assert.Empty(t, release.Release)
		}
	}
	assert.True(t, foundVersion)
}

func (s *PythonPackagesSuite) createPythonRepository(orgId string) api.RepositoryResponse {
	t := s.T()

	repo := models.Repository{
		URL:         testPythonPackageURL,
		Public:      false,
		Origin:      config.OriginExternal,
		ContentType: config.ContentTypePython,
	}

	res := db.DB.Where("url = ? AND content_type = ?", repo.URL, repo.ContentType).First(&repo)
	if res.Error != nil {
		res = db.DB.Create(&repo)
		require.NoError(t, res.Error)
	}

	res = db.DB.Where("repository_uuid = ?", repo.UUID).Delete(&models.RepositoryConfiguration{})
	assert.NoError(t, res.Error)

	repoConfig := models.RepositoryConfiguration{
		Name:           fmt.Sprintf("python-repo-%v", rand.Int()),
		OrgID:          orgId,
		AccountID:      orgId,
		RepositoryUUID: repo.UUID,
		Snapshot:       false,
	}
	res = db.DB.Create(&repoConfig)
	require.NoError(t, res.Error)

	domainName, err := s.dao.Domain.FetchOrCreateDomain(s.ctx, orgId)
	require.NoError(t, err)

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	_, err = pulpClient.LookupOrCreateDomain(s.ctx, domainName)
	require.NoError(t, err)

	ctx, zestClient, err := s.getZestClient()
	require.NoError(t, err)

	pythonRemote := zest.NewPythonPythonRemote(fmt.Sprintf("python-remote-%v", rand.Int()), repo.URL)
	policy := zest.POLICY692ENUM_IMMEDIATE
	pythonRemote.Policy = &policy
	pythonRemote.Includes = []string{testPythonPackageName}

	remoteResp, httpResp, err := zestClient.RemotesPythonAPI.RemotesPythonPythonCreate(ctx, domainName).
		PythonPythonRemote(*pythonRemote).
		Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	require.NoError(t, err)
	require.NotNil(t, remoteResp.PulpHref)

	pythonRepo := zest.NewPythonPythonRepository(fmt.Sprintf("python-repo-%v", rand.Int()))
	pythonRepo.SetRemote(*remoteResp.PulpHref)

	repoResp, httpResp, err := zestClient.RepositoriesPythonAPI.RepositoriesPythonPythonCreate(ctx, domainName).
		PythonPythonRepository(*pythonRepo).
		Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	require.NoError(t, err)
	require.NotNil(t, repoResp.PulpHref)

	syncURL := zest.NewRepositorySyncURL()
	syncURL.SetRemote(*remoteResp.PulpHref)
	mirror := true
	syncURL.SetMirror(mirror)

	// Zest prepends server URL + "/" to the href; strip leading slash to avoid "//api/pulp/..." URLs.
	repoHref := strings.TrimPrefix(*repoResp.PulpHref, "/")
	syncTaskResp, httpResp, err := zestClient.RepositoriesPythonAPI.RepositoriesPythonPythonSync(ctx, repoHref).
		RepositorySyncURL(*syncURL).
		Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	require.NoError(t, err)
	require.NotEmpty(t, syncTaskResp.Task)

	syncTask, err := pulpClient.PollTask(s.ctx, strings.TrimPrefix(syncTaskResp.Task, "/"))
	require.NoError(t, err)
	require.NotNil(t, syncTask.State)
	require.Equal(t, "completed", *syncTask.State)

	distPath := fmt.Sprintf("%s/latest-%v", repo.UUID, rand.Int())
	pythonDist := zest.NewPythonPythonDistribution(distPath, distPath)
	pythonDist.SetRepository(*repoResp.PulpHref)
	pythonDist.SetRemote(*remoteResp.PulpHref)

	distTaskResp, httpResp, err := zestClient.DistributionsPypiAPI.DistributionsPythonPypiCreate(ctx, domainName).
		PythonPythonDistribution(*pythonDist).
		Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	require.NoError(t, err)
	require.NotEmpty(t, distTaskResp.Task)

	distTask, err := pulpClient.PollTask(s.ctx, strings.TrimPrefix(distTaskResp.Task, "/"))
	require.NoError(t, err)
	require.NotNil(t, distTask.State)
	require.Equal(t, "completed", *distTask.State)

	baseURL, err := pulpClient.GetContentPath()
	require.NoError(t, err)

	res = db.DB.Model(&repo).Updates(map[string]any{
		"published_distribution_base_path": distPath,
		"published_distribution_url":       fmt.Sprintf("%s%s/%s", baseURL, domainName, distPath),
	})
	assert.NoError(t, res.Error)

	apiRepoResp := s.dao.RepositoryConfig.InternalOnly_FetchRepoConfigsForRepoUUID(context.Background(), repo.UUID)
	require.NotEmpty(t, apiRepoResp)

	return apiRepoResp[0]
}

func (s *PythonPackagesSuite) listPackages(repoUUID string) api.PackageResponse {
	t := s.T()

	path := api.FullRootPath() + "/repositories/" + repoUUID + "/packages"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", "application/json")

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code, string(body))

	var resp api.PackageResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	return resp
}
