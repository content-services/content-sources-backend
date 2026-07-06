package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PackagesSuite struct {
	suite.Suite
	reg        *dao.MockDaoRegistry
	tangClient *tangy.MockTangy
	pulpClient *pulp_client.MockPulpClient
}

func TestPackagesSuite(t *testing.T) {
	suite.Run(t, new(PackagesSuite))
}

func (suite *PackagesSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
	suite.tangClient = tangy.NewMockTangy(suite.T())
	suite.pulpClient = pulp_client.NewMockPulpClient(suite.T())
}

func (suite *PackagesSuite) servePackagesRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	RegisterPackageRoutes(pathPrefix, suite.reg.ToDaoRegistry(), suite.tangClient, suite.pulpClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *PackagesSuite) TestListPackagesMavenSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	basePath := "java/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/maven/maven/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	tangResp := tangy.MavenPackageListResponse{
		Results: []tangy.MavenPackageListItem{
			{
				GroupID:    "io.smallrye.reactive",
				ArtifactID: "smallrye-mutiny-vertx-core",
				Versions:   []string{"3.16.0", "3.15.0"},
				LatestReleases: []tangy.MavenReleaseInfo{
					{
						Version:   "3.15.0",
						Release:   "rhlw-3001",
						CreatedAt: "2024-01-15T10:30:00Z",
					},
					{
						Version:   "3.16.0",
						Release:   "rhlw-4000",
						CreatedAt: "2024-02-01T14:20:00Z",
					},
				},
			},
		},
		Total:  1,
		Limit:  100,
		Offset: 0,
	}

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), config.LightwellOrg, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), config.LightwellOrg).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("MavenPackageList", test.MockCtx(), repositoryHref, tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: 100}).Return(tangResp, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PackageResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Results))
	assert.Equal(t, "io.smallrye.reactive", response.Results[0].Group)
	assert.Equal(t, "smallrye-mutiny-vertx-core", response.Results[0].Name)
	assert.Equal(t, 2, len(response.Results[0].Versions))
	assert.Equal(t, 2, len(response.Results[0].LatestReleases))
	assert.Equal(t, 1, response.Total)
	assert.Equal(t, 100, response.Limit)
	assert.Equal(t, 0, response.Offset)
}

func (suite *PackagesSuite) TestListPackagesMavenWithFilter() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440007"
	basePath := "java/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/maven/maven/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	search := "smallrye"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	tangResp := tangy.MavenPackageListResponse{
		Results: []tangy.MavenPackageListItem{
			{
				GroupID:    "io.smallrye.reactive",
				ArtifactID: "smallrye-mutiny-vertx-core",
				Versions:   []string{"3.16.0"},
				LatestReleases: []tangy.MavenReleaseInfo{
					{
						Version:   "3.16.0",
						Release:   "rhlw-4000",
						CreatedAt: "2024-02-01T14:20:00Z",
					},
				},
			},
		},
		Total:  1,
		Limit:  100,
		Offset: 0,
	}

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), config.LightwellOrg, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), config.LightwellOrg).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("MavenPackageList", test.MockCtx(), repositoryHref, tangy.MavenPackageListFilters{Search: search}, tangy.PageOptions{Offset: 0, Limit: 100}).Return(tangResp, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0&search=%s", api.FullRootPath(), repoUUID, search)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PackageResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Results))
	assert.Equal(t, "io.smallrye.reactive", response.Results[0].Group)
	assert.Equal(t, "smallrye-mutiny-vertx-core", response.Results[0].Name)
	assert.Equal(t, 1, response.Total)
}

func (suite *PackagesSuite) TestListMavenPackageVersionsSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	basePath := "java/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/maven/maven/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	groupID := "io.smallrye.reactive"
	packageName := "smallrye-mutiny-vertx-core"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	buildListResp := tangy.MavenBuildListResponse{
		Results: []tangy.MavenBuildListItem{
			{
				GroupID:    groupID,
				ArtifactID: packageName,
				Version:    "3.16.0",
				Release:    "rhlw-4000",
				CreatedAt:  "2024-02-01T14:20:00Z",
			},
			{
				GroupID:    groupID,
				ArtifactID: packageName,
				Version:    "3.15.0",
				Release:    "rhlw-3001",
				CreatedAt:  "2024-01-15T10:30:00Z",
			},
		},
		Total:  2,
		Limit:  500,
		Offset: 0,
	}

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("MavenBuildList", test.MockCtx(), repositoryHref, groupID, packageName, "", tangy.PageOptions{}).Return(buildListResp, nil)

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s", api.FullRootPath(), repoUUID, groupID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.MavenPackageVersionsResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, groupID, response.Group)
	assert.Equal(t, packageName, response.Name)
	assert.Equal(t, 2, len(response.Versions))
	assert.Equal(t, "3.16.0", response.Versions[0].Version)
	assert.Equal(t, "rhlw-4000", response.Versions[0].Release)
	assert.Equal(t, "3.15.0", response.Versions[1].Version)
	assert.Equal(t, "rhlw-3001", response.Versions[1].Release)
}

func (suite *PackagesSuite) TestListMavenPackageVersionsNonMavenRepo() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440001"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: "rpm",
	}

	orgID := config.LightwellOrg
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s", api.FullRootPath(), repoUUID, "some.group", "some-package")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *PackagesSuite) TestListMavenPackageVersionsRepoNotFound() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440003"

	daoError := ce.DaoError{
		NotFound: true,
		Message:  "Repository not found",
	}

	orgID := config.LightwellOrg
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, &daoError)

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s", api.FullRootPath(), repoUUID, "some.group", "some-package")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestListMavenPackageVersionsTangError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	basePath := "java/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/maven/maven/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	groupID := "io.smallrye.reactive"
	packageName := "smallrye-mutiny-vertx-core"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("MavenBuildList", test.MockCtx(), repositoryHref, groupID, packageName, "", tangy.PageOptions{}).Return(tangy.MavenBuildListResponse{}, fmt.Errorf("failed to fetch versions"))

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s", api.FullRootPath(), repoUUID, groupID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *PackagesSuite) TestListMavenPackageVersionsEmpty() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	basePath := "java/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/maven/maven/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	groupID := "io.smallrye.reactive"
	packageName := "smallrye-mutiny-vertx-core"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	buildListResp := tangy.MavenBuildListResponse{
		Results: []tangy.MavenBuildListItem{},
		Total:   0,
		Limit:   500,
		Offset:  0,
	}

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("MavenBuildList", test.MockCtx(), repositoryHref, groupID, packageName, "", tangy.PageOptions{}).Return(buildListResp, nil)

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s", api.FullRootPath(), repoUUID, groupID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.MavenPackageVersionsResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, groupID, response.Group)
	assert.Equal(t, packageName, response.Name)
	assert.Equal(t, 0, len(response.Versions))
}

func (suite *PackagesSuite) TestListPackagesPythonSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440005"
	basePath := "python/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/python/python/018c1c95-4281-76eb-b277-842cbad524f5/"
	domainName := "test-domain"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypePython,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	tangResp := tangy.PythonPackageListResponse{
		Results: []tangy.PythonPackageListItem{
			{
				Name:           "shelf-reader",
				NameNormalized: "shelf-reader",
				Versions:       []string{"0.1", "0.2"},
				LatestVersions: []tangy.PythonVersionInfo{
					{
						Version:   "0.1",
						CreatedAt: "2024-01-15T10:30:00Z",
					},
					{
						Version:   "0.2",
						CreatedAt: "2024-02-01T14:20:00Z",
					},
				},
			},
		},
		Total:  1,
		Limit:  100,
		Offset: 0,
	}

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), config.LightwellOrg, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), config.LightwellOrg).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("PythonPackageList", test.MockCtx(), repositoryHref, tangy.PythonPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: 100}).Return(tangResp, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PackageResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Results))
	assert.Empty(t, response.Results[0].Group)
	assert.Equal(t, "shelf-reader", response.Results[0].Name)
	assert.Equal(t, 2, len(response.Results[0].Versions))
	assert.Equal(t, 2, len(response.Results[0].LatestReleases))
	assert.Equal(t, "0.1", response.Results[0].LatestReleases[0].Version)
	assert.Equal(t, "2024-01-15T10:30:00Z", response.Results[0].LatestReleases[0].CreatedAt)
	assert.Empty(t, response.Results[0].LatestReleases[0].Release)
	assert.Equal(t, 1, response.Total)
	assert.Equal(t, 100, response.Limit)
	assert.Equal(t, 0, response.Offset)
}

func (suite *PackagesSuite) TestListPackagesPythonWithFilter() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440008"
	basePath := "python/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/python/python/018c1c95-4281-76eb-b277-842cbad524f5/"
	domainName := "test-domain"
	search := "shelf"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypePython,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	tangResp := tangy.PythonPackageListResponse{
		Results: []tangy.PythonPackageListItem{
			{
				Name:           "shelf-reader",
				NameNormalized: "shelf-reader",
				Versions:       []string{"0.1"},
				LatestVersions: []tangy.PythonVersionInfo{
					{
						Version:   "0.1",
						CreatedAt: "2024-01-15T10:30:00Z",
					},
				},
			},
		},
		Total:  1,
		Limit:  100,
		Offset: 0,
	}

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), config.LightwellOrg, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), config.LightwellOrg).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("PythonPackageList", test.MockCtx(), repositoryHref, tangy.PythonPackageListFilters{Search: search}, tangy.PageOptions{Offset: 0, Limit: 100}).Return(tangResp, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0&search=%s", api.FullRootPath(), repoUUID, search)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PackageResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Results))
	assert.Empty(t, response.Results[0].Group)
	assert.Equal(t, "shelf-reader", response.Results[0].Name)
	assert.Equal(t, 1, len(response.Results[0].Versions))
	assert.Equal(t, 1, response.Total)
}

func (suite *PackagesSuite) TestListPackagesPythonTangClientError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440006"
	basePath := "python/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/python/python/018c1c95-4281-76eb-b277-842cbad524f5/"
	domainName := "test-domain"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypePython,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), config.LightwellOrg, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), config.LightwellOrg).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("PythonPackageList", test.MockCtx(), repositoryHref, tangy.PythonPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: 100}).Return(tangy.PythonPackageListResponse{}, fmt.Errorf("failed to fetch packages"))

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *PackagesSuite) TestListPackagesNonMavenReturnsEmpty() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440001"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: "rpm", // Non-Maven content type
	}

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), config.LightwellOrg, repoUUID).Return(repo, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PackageResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(response.Results))
	assert.Equal(t, 0, response.Total)
	assert.Equal(t, 100, response.Limit)
	assert.Equal(t, 0, response.Offset)
}

func (suite *PackagesSuite) TestListPackagesMissingDistBasePath() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: "", // Empty distribution base path
	}

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), config.LightwellOrg, repoUUID).Return(repo, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *PackagesSuite) TestListPackagesRepositoryNotFound() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440003"

	daoError := ce.DaoError{
		NotFound: true,
		Message:  "Repository not found",
	}

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), config.LightwellOrg, repoUUID).Return(api.RepositoryResponse{}, &daoError)

	path := fmt.Sprintf("%s/repositories/%s/packages", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestListPackagesTangClientError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440004"
	basePath := "java/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/maven/maven/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), config.LightwellOrg, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), config.LightwellOrg).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("MavenPackageList", test.MockCtx(), repositoryHref, tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: 0, Limit: 100}).Return(tangy.MavenPackageListResponse{}, fmt.Errorf("failed to fetch packages"))

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *PackagesSuite) TestGetPackageDetailSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	basePath := "java/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/maven/maven/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	groupID := "io.smallrye.reactive"
	packageName := "smallrye-mutiny-vertx-core"
	packageVersion := "3.15.0"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	buildListResp := tangy.MavenBuildListResponse{
		Results: []tangy.MavenBuildListItem{
			{
				GroupID:    groupID,
				ArtifactID: packageName,
				Version:    "3.15.0",
				Release:    "rhlw-3001",
				CreatedAt:  "2024-01-15T10:30:00Z",
			},
			{
				GroupID:    groupID,
				ArtifactID: packageName,
				Version:    "3.16.0",
				Release:    "rhlw-4000",
				CreatedAt:  "2024-02-01T14:20:00Z",
			},
		},
		Total:  2,
		Limit:  100,
		Offset: 0,
	}

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("MavenBuildList", test.MockCtx(), repositoryHref, groupID, packageName, packageVersion, tangy.PageOptions{Offset: 0, Limit: 100}).Return(buildListResp, nil)
	suite.reg.MavenPackages.On("Fetch", test.MockCtx(), groupID, packageName).Return(nil, nil)
	suite.reg.MavenPackages.On("Create", test.MockCtx(), mock.Anything).Return(nil).Maybe()

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s/%s?limit=100&offset=0", api.FullRootPath(), repoUUID, groupID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.MavenPackageDetailResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, groupID, response.Group)
	assert.Equal(t, packageName, response.Name)
	assert.Equal(t, packageVersion, response.Version)
	assert.Equal(t, 2, len(response.Builds))
	assert.Equal(t, "3.15.0", response.Builds[0].Version)
	assert.Equal(t, "rhlw-3001", response.Builds[0].Release)
	assert.Equal(t, "2024-01-15T10:30:00Z", response.Builds[0].CreatedAt)
	assert.Equal(t, "3.16.0", response.Builds[1].Version)
	assert.Equal(t, "rhlw-4000", response.Builds[1].Release)
	assert.Equal(t, "2024-02-01T14:20:00Z", response.Builds[1].CreatedAt)
}

func (suite *PackagesSuite) TestGetPackageDetailReturnsCachedMetadata() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	basePath := "java/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/maven/maven/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	groupID := "commons-io"
	packageName := "commons-io"
	packageVersion := "2.11.0"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	buildListResp := tangy.MavenBuildListResponse{
		Results: []tangy.MavenBuildListItem{
			{
				GroupID:    groupID,
				ArtifactID: packageName,
				Version:    packageVersion,
				CreatedAt:  "2024-01-15T10:30:00Z",
			},
		},
		Total:  1,
		Limit:  100,
		Offset: 0,
	}

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("MavenBuildList", test.MockCtx(), repositoryHref, groupID, packageName, packageVersion, tangy.PageOptions{Offset: 0, Limit: 100}).Return(buildListResp, nil)
	suite.reg.MavenPackages.On("Fetch", test.MockCtx(), groupID, packageName).Return(&models.MavenPackage{
		GroupID:    groupID,
		Name:       packageName,
		Summary:    utils.Ptr("The Apache Commons IO library contains utility classes."),
		License:    utils.Ptr("Apache-2.0"),
		ProjectURL: utils.Ptr("https://commons.apache.org/proper/commons-io/"),
		Author:     utils.Ptr("The Apache Software Foundation"),
	}, nil)

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s/%s?limit=100&offset=0", api.FullRootPath(), repoUUID, groupID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.MavenPackageDetailResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	require.NotNil(t, response.Summary)
	require.NotNil(t, response.License)
	require.NotNil(t, response.ProjectURL)
	require.NotNil(t, response.Author)
	assert.Equal(t, "The Apache Commons IO library contains utility classes.", *response.Summary)
	assert.Equal(t, "Apache-2.0", *response.License)
	assert.Equal(t, "https://commons.apache.org/proper/commons-io/", *response.ProjectURL)
	assert.Equal(t, "The Apache Software Foundation", *response.Author)
}

func (suite *PackagesSuite) TestGetPackageDetailMetadataFetchError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	basePath := "java/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/maven/maven/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	groupID := "commons-io"
	packageName := "commons-io"
	packageVersion := "2.11.0"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	buildListResp := tangy.MavenBuildListResponse{
		Results: []tangy.MavenBuildListItem{},
		Total:   0,
		Limit:   100,
		Offset:  0,
	}

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("MavenBuildList", test.MockCtx(), repositoryHref, groupID, packageName, packageVersion, tangy.PageOptions{Offset: 0, Limit: 100}).Return(buildListResp, nil)
	suite.reg.MavenPackages.On("Fetch", test.MockCtx(), groupID, packageName).Return(nil, fmt.Errorf("database unavailable"))

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s/%s?limit=100&offset=0", api.FullRootPath(), repoUUID, groupID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *PackagesSuite) TestGetPackageDetailNonMavenRepo() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440001"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: "rpm",
	}

	orgID := config.LightwellOrg
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s/%s", api.FullRootPath(), repoUUID, "some.group", "some-package", "1.0.0")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *PackagesSuite) TestGetPackageDetailRepoNotFound() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440003"

	daoError := ce.DaoError{
		NotFound: true,
		Message:  "Repository not found",
	}

	orgID := config.LightwellOrg
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, &daoError)

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s/%s", api.FullRootPath(), repoUUID, "some.group", "some-package", "1.0.0")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestGetPackageDetailTangBuildListError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	basePath := "java/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/maven/maven/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	groupID := "io.smallrye.reactive"
	packageName := "smallrye-mutiny-vertx-core"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("MavenBuildList", test.MockCtx(), repositoryHref, groupID, packageName, "3.15.0", tangy.PageOptions{Offset: 0, Limit: 100}).Return(tangy.MavenBuildListResponse{}, fmt.Errorf("failed to fetch builds"))

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s/%s?limit=100&offset=0", api.FullRootPath(), repoUUID, groupID, packageName, "3.15.0")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *PackagesSuite) TestGetPackageDetailEmptyBuilds() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	basePath := "java/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/maven/maven/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	groupID := "io.smallrye.reactive"
	packageName := "smallrye-mutiny-vertx-core"
	packageVersion := "3.15.0"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	buildListResp := tangy.MavenBuildListResponse{
		Results: []tangy.MavenBuildListItem{},
		Total:   0,
		Limit:   100,
		Offset:  0,
	}

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("MavenBuildList", test.MockCtx(), repositoryHref, groupID, packageName, packageVersion, tangy.PageOptions{Offset: 0, Limit: 100}).Return(buildListResp, nil)
	suite.reg.MavenPackages.On("Fetch", test.MockCtx(), groupID, packageName).Return(nil, nil)
	suite.reg.MavenPackages.On("Create", test.MockCtx(), mock.Anything).Return(nil).Maybe()

	path := fmt.Sprintf("%s/repositories/%s/maven_packages/%s/%s/%s?limit=100&offset=0", api.FullRootPath(), repoUUID, groupID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.MavenPackageDetailResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, groupID, response.Group)
	assert.Equal(t, packageName, response.Name)
	assert.Equal(t, packageVersion, response.Version)
	assert.Equal(t, 0, len(response.Builds))
}

func (suite *PackagesSuite) TestGetPythonPackageVersionsSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"
	basePath := "python/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/python/python/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	packageName := "django"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypePython,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	tangDetails := []tangy.PythonPackageDetail{
		{
			Name:           "Django",
			NameNormalized: packageName,
			Version:        "4.2",
			Summary:        "A high-level Python web framework",
			LastUpdated:    "2024-01-01T12:00:00Z",
			Versions:       []string{"4.2", "5.0"},
			Distributions: []tangy.PythonDistributionListItem{
				{
					Filename:    "django-4.2-py3-none-any.whl",
					PackageType: "bdist_wheel",
				},
			},
		},
		{
			Name:           "Django",
			NameNormalized: packageName,
			Version:        "5.0",
			Summary:        "A high-level Python web framework",
			LastUpdated:    "2024-02-01T12:00:00Z",
			Versions:       []string{"4.2", "5.0"},
			Distributions: []tangy.PythonDistributionListItem{
				{
					Filename:    "django-5.0-py3-none-any.whl",
					PackageType: "bdist_wheel",
				},
			},
		},
	}

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("PythonPackageVersionsGet", test.MockCtx(), repositoryHref, packageName).Return(tangDetails, nil)

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s", api.FullRootPath(), repoUUID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PythonPackageVersionsResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, packageName, response.Name)
	require.Len(t, response.Versions, 2)
	assert.Equal(t, "4.2", response.Versions[0].Version)
	assert.Equal(t, "5.0", response.Versions[1].Version)
}

func (suite *PackagesSuite) TestGetPythonPackageVersionsNonPythonRepo() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440001"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := config.LightwellOrg
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s", api.FullRootPath(), repoUUID, "django")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *PackagesSuite) TestGetPythonPackageVersionsNotFound() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"
	basePath := "python/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/python/python/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	packageName := "nonexistent"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypePython,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("PythonPackageVersionsGet", test.MockCtx(), repositoryHref, packageName).Return([]tangy.PythonPackageDetail{}, tangy.ErrPythonPackageNotFound)

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s", api.FullRootPath(), repoUUID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestGetPythonPackageVersionsTangError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"
	basePath := "python/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/python/python/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	packageName := "django"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypePython,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("PythonPackageVersionsGet", test.MockCtx(), repositoryHref, packageName).Return([]tangy.PythonPackageDetail{}, fmt.Errorf("failed to fetch package versions"))

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s", api.FullRootPath(), repoUUID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *PackagesSuite) TestGetPythonPackageDetailSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"
	basePath := "python/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/python/python/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	packageName := "django"
	packageVersion := "5.0"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypePython,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	tangDetail := tangy.PythonPackageDetail{
		Name:           "Django",
		NameNormalized: packageName,
		Version:        packageVersion,
		Summary:        "A high-level Python web framework",
		Description:    "Django is a high-level Python web framework.",
		Author:         "Django Software Foundation",
		AuthorEmail:    "foundation@djangoproject.com",
		License:        "BSD-3-Clause",
		ProjectURL:     "https://www.djangoproject.com/",
		LastUpdated:    "2024-01-01T12:00:00Z",
		Versions:       []string{"4.2", "5.0"},
		Distributions: []tangy.PythonDistributionListItem{
			{
				Name:           "Django",
				NameNormalized: packageName,
				Version:        packageVersion,
				Filename:       "django-5.0-py3-none-any.whl",
				PackageType:    "bdist_wheel",
				PythonVersion:  "py3",
				Sha256:         "abc123",
				Size:           1024,
				CreatedAt:      "2024-01-01T12:00:00Z",
			},
		},
	}

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("PythonPackageGet", test.MockCtx(), repositoryHref, packageName, packageVersion).Return(tangDetail, nil)

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s/%s", api.FullRootPath(), repoUUID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PythonPackageDetailResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, packageName, response.Name)
	assert.Equal(t, packageVersion, response.Version)
	assert.Equal(t, "A high-level Python web framework", response.Summary)
	assert.Equal(t, "Django is a high-level Python web framework.", response.Description)
	assert.Equal(t, "2024-01-01T12:00:00Z", response.LastUpdated)
	assert.Equal(t, "BSD-3-Clause", response.License)
	assert.Equal(t, "Django Software Foundation", response.Author.Name)
	assert.Equal(t, "foundation@djangoproject.com", response.Author.Email)
	assert.Equal(t, []string{"4.2", "5.0"}, response.UpstreamVersions)
	assert.Equal(t, "https://www.djangoproject.com/", response.ProjectURL)
	require.Len(t, response.Distributions, 1)
	assert.Equal(t, "django-5.0-py3-none-any.whl", response.Distributions[0].Filename)
	assert.Equal(t, "bdist_wheel", response.Distributions[0].PackageType)
}

func (suite *PackagesSuite) TestGetPythonPackageDetailNonPythonRepo() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440001"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := config.LightwellOrg
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s/%s", api.FullRootPath(), repoUUID, "django", "5.0")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *PackagesSuite) TestGetPythonPackageDetailNotFound() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"
	basePath := "python/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/python/python/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	packageName := "django"
	packageVersion := "9.9.9"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypePython,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("PythonPackageGet", test.MockCtx(), repositoryHref, packageName, packageVersion).Return(tangy.PythonPackageDetail{}, tangy.ErrPythonPackageNotFound)

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s/%s", api.FullRootPath(), repoUUID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestGetPythonPackageDetailTangError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"
	basePath := "python/remediated"
	repositoryHref := "/api/pulp/default/api/v3/repositories/python/python/018c1c95-4281-76eb-b277-842cbad524f4/"
	domainName := "test-domain"
	packageName := "django"
	packageVersion := "5.0"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypePython,
		PublishedDistBasePath: basePath,
	}

	dist := zest.DistributionResponse{}
	dist.SetRepository(repositoryHref)

	orgID := config.LightwellOrg

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(repo, nil)
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), orgID).Return(domainName, nil)
	suite.pulpClient.On("WithDomain", domainName).Return(suite.pulpClient)
	suite.pulpClient.On("ResolveRepositoryFromBasePath", test.MockCtx(), basePath).Return(&repositoryHref, nil)
	suite.tangClient.On("PythonPackageGet", test.MockCtx(), repositoryHref, packageName, packageVersion).Return(tangy.PythonPackageDetail{}, fmt.Errorf("failed to fetch package detail"))

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s/%s", api.FullRootPath(), repoUUID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}
