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
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
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
	suite.pulpClient.On("FindGenericDistributionByBasePath", test.MockCtx(), basePath).Return(&dist, nil)
	suite.tangClient.On("MavenPackageList", test.MockCtx(), repositoryHref, tangy.PageOptions{Offset: 0, Limit: 100}).Return(tangResp, nil)

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
	suite.pulpClient.On("FindGenericDistributionByBasePath", test.MockCtx(), basePath).Return(&dist, nil)
	suite.tangClient.On("PythonPackageList", test.MockCtx(), repositoryHref, tangy.PageOptions{Offset: 0, Limit: 100}).Return(tangResp, nil)

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
	suite.pulpClient.On("FindGenericDistributionByBasePath", test.MockCtx(), basePath).Return(&dist, nil)
	suite.tangClient.On("PythonPackageList", test.MockCtx(), repositoryHref, tangy.PageOptions{Offset: 0, Limit: 100}).Return(tangy.PythonPackageListResponse{}, fmt.Errorf("failed to fetch packages"))

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
	suite.pulpClient.On("FindGenericDistributionByBasePath", test.MockCtx(), basePath).Return(&dist, nil)
	suite.tangClient.On("MavenPackageList", test.MockCtx(), repositoryHref, tangy.PageOptions{Offset: 0, Limit: 100}).Return(tangy.MavenPackageListResponse{}, fmt.Errorf("failed to fetch packages"))

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}
