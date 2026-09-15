package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
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

type MavenPackagesSuite struct {
	Suite
	ctx      context.Context
	server   *http.Server
	identity identity.XRHID
	cancel   context.CancelFunc
}

func (s *MavenPackagesSuite) SetupTest() {
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
		Addr:              "127.0.0.1:8101",
		Handler:           router,
		IdleTimeout:       1 * time.Minute,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	// force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"

	// Initialize Tang for Maven package listing
	err = config.ConfigureTang()
	require.NoError(s.T(), err)
}

func (s *MavenPackagesSuite) TearDownTest() {
	s.cancel()
	err := s.server.Shutdown(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Could not shutdown server")
	}
	s.Suite.TearDownTest()
}

func (s *MavenPackagesSuite) serveRouter(req *http.Request) (int, []byte, error) {
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(s.T(), err)

	return response.StatusCode, body, err
}

func (s *MavenPackagesSuite) getZestClient() (context.Context, *zest.APIClient, error) {
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

func TestMavenPackagesSuite(t *testing.T) {
	suite.Run(t, new(MavenPackagesSuite))
}

func (s *MavenPackagesSuite) TestMavenPackagesAPI() {
	orgId := fmt.Sprintf("MavenPackages-%v", rand.Int())

	// randomize the identity for multiple test runs
	s.identity = test_handler.MockIdentity
	s.identity.Identity.OrgID = orgId

	t := s.T()

	// Create a Maven repository pointing to Maven Central
	mavenRepo := s.createMavenRepository(config.LightwellOrg)

	// Pull-through GET skips MavenPackage creation; upload + modify runs finalize with packages.
	s.seedMavenPackages(mavenRepo)

	// Test the packages API endpoint
	packages := s.listPackages(mavenRepo.repo.UUID)
	require.NotNil(t, packages.Results)
	require.NotEmpty(t, packages.Results)
	firstPackage := packages.Results[0]
	assert.NotEmpty(t, firstPackage.Group)
	assert.NotEmpty(t, firstPackage.Name)

	// Test the package versions endpoint using data from the list
	versions := s.listPackageVersions(mavenRepo.repo.UUID, firstPackage.Group, firstPackage.Name)
	assert.Equal(t, firstPackage.Group, versions.Group)
	assert.Equal(t, firstPackage.Name, versions.Name)
	require.NotEmpty(t, versions.Versions)
	for _, v := range versions.Versions {
		assert.NotEmpty(t, v.Version)
		require.NotEmpty(t, v.Builds)
		assert.NotEmpty(t, v.Builds[0].CreatedAt)
	}

	// Test the package detail endpoint using data from the list
	require.NotEmpty(t, firstPackage.Versions)
	detail := s.getPackageDetail(mavenRepo.repo.UUID, firstPackage.Group, firstPackage.Name, firstPackage.Versions[0])
	assert.Equal(t, firstPackage.Group, detail.Group)
	assert.Equal(t, firstPackage.Name, detail.Name)
	assert.Equal(t, firstPackage.Versions[0], detail.Version)
}

type mavenPulpRepository struct {
	repo           api.RepositoryResponse
	repositoryHref string
	remoteHref     string
}

func (s *MavenPackagesSuite) createMavenRepository(orgId string) mavenPulpRepository {
	t := s.T()

	// Create the repository directly in the database (API doesn't support Maven content type)
	repo := models.Repository{
		URL:         "https://repo.maven.apache.org/maven2/",
		Public:      false,
		Origin:      config.OriginExternal,
		ContentType: config.ContentTypeMaven,
	}

	res := db.DB.Where("url = ?", repo.URL).First(&repo)
	if res.Error != nil {
		res = db.DB.Create(&repo)
		require.NoError(t, res.Error)
	}

	res = db.DB.Where("repository_uuid = ?", repo.UUID).Delete(&models.RepositoryConfiguration{})
	assert.NoError(t, res.Error)

	// Create the repository configuration
	repoConfig := models.RepositoryConfiguration{
		Name:           fmt.Sprintf("maven-repo-%v", rand.Int()),
		OrgID:          orgId,
		AccountID:      orgId,
		RepositoryUUID: repo.UUID,
		Snapshot:       false,
	}
	res = db.DB.Create(&repoConfig)
	require.NoError(t, res.Error)

	// Create Pulp infrastructure directly using Zest client for Maven
	domainName, err := s.dao.Domain.FetchOrCreateDomain(s.ctx, orgId)
	require.NoError(t, err)

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	// Create domain in Pulp
	_, err = pulpClient.LookupOrCreateDomain(s.ctx, domainName)
	require.NoError(t, err)

	// Get the raw Zest client to access Maven APIs
	ctx, zestClient, err := s.getZestClient()
	require.NoError(t, err)

	// Create Maven remote
	mavenRemote := zest.NewMavenMavenRemote(fmt.Sprintf("maven-remote-%v", rand.Int()), repo.URL)
	remoteResp, httpResp, err := zestClient.RemotesMavenAPI.RemotesMavenMavenCreate(ctx, domainName).
		MavenMavenRemote(*mavenRemote).
		Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	require.NoError(t, err)
	require.NotNil(t, remoteResp.PulpHref)

	// Create Maven repository
	mavenRepo := zest.NewMavenMavenRepository(fmt.Sprintf("maven-repo-%v", rand.Int()))
	if remoteResp.PulpHref != nil {
		mavenRepo.Remote = *zest.NewNullableString(remoteResp.PulpHref)
	}
	mavenRepoResp, httpResp, err := zestClient.RepositoriesMavenAPI.RepositoriesMavenMavenCreate(ctx, domainName).
		MavenMavenRepository(*mavenRepo).
		Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	require.NoError(t, err)
	require.NotNil(t, mavenRepoResp.PulpHref)

	// Create Maven distribution (no publication needed for Maven)
	distPath := fmt.Sprintf("%s/latest-%v", repo.UUID, rand.Int())
	mavenDist := zest.NewMavenMavenDistribution(distPath, distPath)
	mavenDist.Repository = *zest.NewNullableString(mavenRepoResp.PulpHref)
	mavenDist.Remote = *zest.NewNullableString(remoteResp.PulpHref)
	distTaskResp, httpResp, err := zestClient.DistributionsMavenAPI.DistributionsMavenMavenCreate(ctx, domainName).
		MavenMavenDistribution(*mavenDist).
		Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	require.NoError(t, err)
	require.NotEmpty(t, distTaskResp.Task)

	// Wait for distribution creation to complete
	distTask, err := pulpClient.PollTask(s.ctx, distTaskResp.Task)
	require.NoError(t, err)
	require.NotNil(t, distTask.State)
	require.Equal(t, "completed", *distTask.State)

	// Update repository config with the distribution info
	baseURL, err := pulpClient.GetContentPath()
	require.NoError(t, err)

	res = db.DB.Model(&repo).Updates(map[string]any{
		"published_distribution_base_path": distPath,
		"published_distribution_url":       fmt.Sprintf("%s%s/%s", baseURL, domainName, distPath),
	})
	assert.NoError(t, res.Error)

	// Fetch and return the updated config
	apiRepoResp := s.dao.RepositoryConfig.InternalOnly_FetchRepoConfigsForRepoUUID(context.Background(), repo.UUID)
	require.NotEmpty(t, apiRepoResp)

	return mavenPulpRepository{
		repo:           apiRepoResp[0],
		repositoryHref: *mavenRepoResp.PulpHref,
		remoteHref:     *remoteResp.PulpHref,
	}
}

type mavenSeedPOM struct {
	relativePath string
	body         string
}

func mavenSeedPOMs() []mavenSeedPOM {
	return []mavenSeedPOM{
		{
			relativePath: "blissed/blissed/1.0-beta-3/blissed-1.0-beta-3.pom",
			body: `<?xml version="1.0"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>blissed</groupId>
  <artifactId>blissed</artifactId>
  <version>1.0-beta-3</version>
</project>
`,
		},
		{
			relativePath: "avalon-util/avalon-util-exception/1.0.0/avalon-util-exception-1.0.0.pom",
			body: `<?xml version="1.0"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>avalon-util</groupId>
  <artifactId>avalon-util-exception</artifactId>
  <version>1.0.0</version>
</project>
`,
		},
	}
}

func (s *MavenPackagesSuite) seedMavenPackages(mavenRepo mavenPulpRepository) {
	t := s.T()

	domainName, err := s.dao.Domain.FetchOrCreateDomain(s.ctx, mavenRepo.repo.OrgID)
	require.NoError(t, err)

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	ctx, zestClient, err := s.getZestClient()
	require.NoError(t, err)

	contentHrefs := make([]string, 0, len(mavenSeedPOMs()))
	for _, pom := range mavenSeedPOMs() {
		file, err := os.CreateTemp(t.TempDir(), filepath.Base(pom.relativePath))
		require.NoError(t, err)
		_, err = file.WriteString(pom.body)
		require.NoError(t, err)
		_, err = file.Seek(0, 0)
		require.NoError(t, err)

		uploadResp, httpResp, err := zestClient.ContentArtifactAPI.ContentMavenArtifactUpload(ctx, domainName).
			RelativePath(pom.relativePath).
			File(file).
			Execute()
		_ = file.Close()
		if httpResp != nil {
			defer httpResp.Body.Close()
		}
		require.NoError(t, err, "upload %s", pom.relativePath)
		require.NotNil(t, uploadResp.PulpHref)
		contentHrefs = append(contentHrefs, *uploadResp.PulpHref)
	}

	modify := zest.NewRepositoryAddRemoveContent()
	modify.SetAddContentUnits(contentHrefs)

	repositoryHref := strings.TrimPrefix(mavenRepo.repositoryHref, "/")
	taskResp, httpResp, err := zestClient.RepositoriesMavenAPI.RepositoriesMavenMavenModify(ctx, repositoryHref).
		RepositoryAddRemoveContent(*modify).
		Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	require.NoError(t, err)
	require.NotEmpty(t, taskResp.Task)

	task, err := pulpClient.PollTask(s.ctx, strings.TrimPrefix(taskResp.Task, "/"))
	require.NoError(t, err)
	require.NotNil(t, task.State)
	require.Equal(t, "completed", *task.State)
}

func (s *MavenPackagesSuite) listPackages(repoUUID string) api.PackageResponse {
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

	assert.Len(t, resp.Results, 2)
	names := []string{}
	for _, pkg := range resp.Results {
		names = append(names, pkg.Name)
	}
	assert.Contains(t, names, "avalon-util-exception")
	assert.Contains(t, names, "blissed")

	return resp
}

func (s *MavenPackagesSuite) listPackageVersions(repoUUID, group, name string) api.MavenPackageVersionsResponse {
	t := s.T()

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s", api.FullRootPath(), repoUUID, group, name)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", "application/json")

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code, string(body))

	var resp api.MavenPackageVersionsResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	return resp
}

func (s *MavenPackagesSuite) getPackageDetail(repoUUID, group, name, version string) api.MavenPackageDetailResponse {
	t := s.T()

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s/%s", api.FullRootPath(), repoUUID, group, name, version)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", "application/json")

	code, body, err := s.serveRouter(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code, string(body))

	var resp api.MavenPackageDetailResponse
	err = json.Unmarshal(body, &resp)
	require.NoError(t, err)

	return resp
}

func (s *MavenPackagesSuite) TestContentCountsForMavenRepository() {
	// Reuse the setup from TestMavenPackagesAPI to avoid duplicate repository creation
	orgId := fmt.Sprintf("ContentCounts-%v", rand.Int())

	// randomize the identity for multiple test runs
	s.identity = test_handler.MockIdentity
	s.identity.Identity.OrgID = orgId

	t := s.T()

	// Create a Maven repository
	mavenRepo := s.createMavenRepository(config.LightwellOrg)
	repo := mavenRepo.repo

	s.seedMavenPackages(mavenRepo)

	// Get domain and pulp client
	domainName, err := s.dao.Domain.FetchOrCreateDomain(s.ctx, config.LightwellOrg)
	require.NoError(t, err)
	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	// Test GetContentCounts function

	// Test UpdateContentCounts function
	err = external_repos.UpdateContentCounts(
		s.ctx,
		s.dao,
		pulpClient,
		*config.Tang,
		domainName,
		false,
	)

	require.NoError(t, err)

	// Verify the repository was updated in the database
	updatedRepo, err := s.dao.RepositoryConfig.Fetch(s.ctx, repo.OrgID, repo.UUID)
	require.NoError(t, err)
	assert.Equal(t, 2, updatedRepo.PackageCount, "Package count should be updated in database")
	assert.Equal(t, 2, updatedRepo.BuildCount, "Build count should be updated in database")
	assert.Equal(t, 2, updatedRepo.VersionCount, "Version count should be updated in database")
}
