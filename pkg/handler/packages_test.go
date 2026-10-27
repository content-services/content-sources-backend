package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	fsc "github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PackagesSuite struct {
	suite.Suite
	reg        *dao.MockDaoRegistry
	tangClient *tangy.MockTangy
	pulpClient *pulp_client.MockPulpClient
	fsClient   *fsc.MockFeatureServiceClient
}

func TestPackagesSuite(t *testing.T) {
	suite.Run(t, new(PackagesSuite))
}

func (suite *PackagesSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
	suite.tangClient = tangy.NewMockTangy(suite.T())
	suite.pulpClient = pulp_client.NewMockPulpClient(suite.T())
	suite.fsClient = fsc.NewMockFeatureServiceClient(suite.T())
}

func (suite *PackagesSuite) servePackagesRouter(req *http.Request) (int, []byte, error) {
	rr := suite.recordPackages(req)
	return rr.Code, rr.Body.Bytes(), nil
}

func (suite *PackagesSuite) recordPackages(req *http.Request) *httptest.ResponseRecorder {
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
	return rr
}

func (suite *PackagesSuite) TestListPackagesMavenSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, "", "last_updated", "desc", 100, 0).
		Return(api.PackageResponse{
			Results: []api.PackageItem{{
				Group:          "io.smallrye.reactive",
				Name:           "smallrye-mutiny-vertx-core",
				Versions:       []string{"3.15.0", "3.16.0"},
				LatestReleases: []api.ReleaseInfo{{Version: "3.16.0", Release: "rhlw-4000", CreatedAt: "2024-02-01T14:20:00Z"}},
			}},
			Total:  1,
			Limit:  100,
			Offset: 0,
		}, nil)

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
	assert.Equal(t, 1, len(response.Results[0].LatestReleases))
	assert.Equal(t, "rhlw-4000", response.Results[0].LatestReleases[0].Release)
	assert.Equal(t, 1, response.Total)
	assert.Equal(t, 100, response.Limit)
	assert.Equal(t, 0, response.Offset)
}

func (suite *PackagesSuite) TestListPackagesMavenWithFilter() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440007"
	search := "smallrye"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, search, "group/name", "desc", 100, 0).
		Return(api.PackageResponse{
			Results: []api.PackageItem{{
				Group:    "io.smallrye.reactive",
				Name:     "smallrye-mutiny-vertx-core",
				Versions: []string{"3.16.0"},
			}},
			Total:  1,
			Limit:  100,
			Offset: 0,
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0&search=%s&sort_by=name:desc", api.FullRootPath(), repoUUID, search)
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
	groupID := "io.smallrye.reactive"
	packageName := "smallrye-mutiny-vertx-core"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListMavenPackageVersions", test.MockCtx(), repoUUID, groupID, packageName).Return(api.MavenPackageVersionsResponse{
		Group: groupID,
		Name:  packageName,
		Versions: []api.MavenPackageDetailResponse{
			{
				Group:      groupID,
				Name:       packageName,
				Version:    "3.16.0",
				Summary:    utils.Ptr("A reactive library."),
				License:    utils.Ptr("Apache-2.0"),
				ProjectURL: utils.Ptr("https://smallrye.io/smallrye-mutiny/"),
				Author:     utils.Ptr("SmallRye"),
				Builds: []api.ReleaseInfo{
					{Version: "3.16.0", Release: "rhlw-4000", CreatedAt: "2024-02-01T14:20:00Z"},
				},
			},
			{
				Group:   groupID,
				Name:    packageName,
				Version: "3.15.0",
				Summary: utils.Ptr("A reactive library."),
				License: utils.Ptr("Apache-2.0"),
				Builds: []api.ReleaseInfo{
					{Version: "3.15.0", Release: "rhlw-3001", CreatedAt: "2024-01-15T10:30:00Z"},
				},
			},
		},
	}, nil)

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s", api.FullRootPath(), repoUUID, groupID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	rr := suite.recordPackages(req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var response api.MavenPackageVersionsResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.Nil(t, err)
	assert.Equal(t, groupID, response.Group)
	assert.Equal(t, packageName, response.Name)
	assert.Equal(t, 2, len(response.Versions))
	assert.Equal(t, "3.16.0", response.Versions[0].Version)
	assert.Equal(t, "rhlw-4000", response.Versions[0].Builds[0].Release)
	require.NotNil(t, response.Versions[0].Summary)
	assert.Equal(t, "A reactive library.", *response.Versions[0].Summary)
	assert.Equal(t, "Apache-2.0", *response.Versions[0].License)
	assert.Equal(t, "3.15.0", response.Versions[1].Version)
	assert.Equal(t, "rhlw-3001", response.Versions[1].Builds[0].Release)
	require.NotNil(t, response.Versions[1].Summary)
	assert.Equal(t, "A reactive library.", *response.Versions[1].Summary)
}

func (suite *PackagesSuite) TestListMavenPackageVersionsNonMavenRepo() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440001"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: "rpm",
	}

	orgID := test_handler.MockOrgId
	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s", api.FullRootPath(), repoUUID, "some.group", "some-package")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *PackagesSuite) TestListMavenPackageVersionsRepoNotFound() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440003"

	orgID := test_handler.MockOrgId
	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{}}, int64(1), nil)

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s", api.FullRootPath(), repoUUID, "some.group", "some-package")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestListMavenPackageVersionsDaoError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	groupID := "io.smallrye.reactive"
	packageName := "smallrye-mutiny-vertx-core"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListMavenPackageVersions", test.MockCtx(), repoUUID, groupID, packageName).
		Return(api.MavenPackageVersionsResponse{}, fmt.Errorf("failed to list lightwell maven package versions"))

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s", api.FullRootPath(), repoUUID, groupID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *PackagesSuite) TestListMavenPackageVersionsEmpty() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	groupID := "io.smallrye.reactive"
	packageName := "smallrye-mutiny-vertx-core"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListMavenPackageVersions", test.MockCtx(), repoUUID, groupID, packageName).
		Return(api.MavenPackageVersionsResponse{}, &ce.DaoError{NotFound: true, Message: "Package not found"})

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s", api.FullRootPath(), repoUUID, groupID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestListPackagesPythonSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440005"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypePython,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, "", "last_updated", "desc", 100, 0).
		Return(api.PackageResponse{
			Results: []api.PackageItem{{
				Name:     "shelf-reader",
				Versions: []string{"0.1", "0.2"},
				LatestReleases: []api.ReleaseInfo{
					{Version: "0.1", CreatedAt: "2024-01-15T10:30:00Z"},
					{Version: "0.2", CreatedAt: "2024-02-01T14:20:00Z"},
				},
			}},
			Total:  1,
			Limit:  100,
			Offset: 0,
		}, nil)

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
	search := "shelf"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypePython,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, search, "name", "desc", 100, 0).
		Return(api.PackageResponse{
			Results: []api.PackageItem{{
				Name:     "shelf-reader",
				Versions: []string{"0.1"},
			}},
			Total:  1,
			Limit:  100,
			Offset: 0,
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0&search=%s&sort_by=name:desc", api.FullRootPath(), repoUUID, search)
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

func (suite *PackagesSuite) TestListPackagesPythonDaoError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440006"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypePython,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, "", "last_updated", "desc", 100, 0).
		Return(api.PackageResponse{}, fmt.Errorf("failed to list lightwell repository packages"))

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

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)

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
	suite.reg.LightwellPackage.AssertNotCalled(t, "ListRepositoryPackages", test.MockCtx(), repoUUID, "", "last_updated", "desc", 100, 0)
}

func (suite *PackagesSuite) TestListPackagesMissingDistBasePath() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"

	repo := api.RepositoryResponse{
		UUID:                  repoUUID,
		ContentType:           config.ContentTypeMaven,
		PublishedDistBasePath: "",
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, "", "last_updated", "desc", 100, 0).
		Return(api.PackageResponse{Results: []api.PackageItem{}, Total: 0, Limit: 100, Offset: 0}, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *PackagesSuite) TestListPackagesRepositoryNotFound() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440003"
	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{}}, int64(1), nil)

	path := fmt.Sprintf("%s/repositories/%s/packages", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestListPackagesMavenDaoError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440004"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, "", "last_updated", "desc", 100, 0).
		Return(api.PackageResponse{}, fmt.Errorf("failed to list lightwell repository packages"))

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *PackagesSuite) TestListPackagesInvalidSortBy() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440004"

	for _, sortBy := range []string{"version:desc", "group/name", "scope/name"} {
		path := fmt.Sprintf("%s/repositories/%s/packages?sort_by=%s", api.FullRootPath(), repoUUID, sortBy)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

		code, body, err := suite.servePackagesRouter(req)
		assert.Nil(t, err)
		assert.Equal(t, http.StatusBadRequest, code)

		var response ce.ErrorResponse
		assert.Nil(t, json.Unmarshal(body, &response))
		require.NotEmpty(t, response.Errors)
		assert.Contains(t, response.Errors[0].Detail, "valid fields are name and last_updated")
	}
	suite.reg.LightwellPackage.AssertNotCalled(t, "ListRepositoryPackages")
}

func (suite *PackagesSuite) TestGetPackageDetailSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	groupID := "io.smallrye.reactive"
	packageName := "smallrye-mutiny-vertx-core"
	packageVersion := "3.15.0"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("GetMavenPackageDetail", test.MockCtx(), repoUUID, groupID, packageName, packageVersion).Return(api.MavenPackageDetailResponse{
		Group:   groupID,
		Name:    packageName,
		Version: packageVersion,
		Builds: []api.ReleaseInfo{
			{Version: "3.15.0", Release: "rhlw-4000", CreatedAt: "2024-02-01T14:20:00Z"},
			{Version: "3.15.0", Release: "rhlw-3001", CreatedAt: "2024-01-15T10:30:00Z"},
		},
	}, nil)

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s/%s?limit=100&offset=0", api.FullRootPath(), repoUUID, groupID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	rr := suite.recordPackages(req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var response api.MavenPackageDetailResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.Nil(t, err)
	assert.Equal(t, groupID, response.Group)
	assert.Equal(t, packageName, response.Name)
	assert.Equal(t, packageVersion, response.Version)
	assert.Equal(t, 2, len(response.Builds))
	assert.Equal(t, "3.15.0", response.Builds[0].Version)
	assert.Equal(t, "rhlw-4000", response.Builds[0].Release)
	assert.Equal(t, "2024-02-01T14:20:00Z", response.Builds[0].CreatedAt)
	assert.Equal(t, "3.15.0", response.Builds[1].Version)
	assert.Equal(t, "rhlw-3001", response.Builds[1].Release)
	assert.Equal(t, "2024-01-15T10:30:00Z", response.Builds[1].CreatedAt)
}

func (suite *PackagesSuite) TestGetPackageDetailReturnsCachedMetadata() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	groupID := "commons-io"
	packageName := "commons-io"
	packageVersion := "2.11.0"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("GetMavenPackageDetail", test.MockCtx(), repoUUID, groupID, packageName, packageVersion).Return(api.MavenPackageDetailResponse{
		Group:      groupID,
		Name:       packageName,
		Version:    packageVersion,
		Summary:    utils.Ptr("The Apache Commons IO library contains utility classes."),
		License:    utils.Ptr("Apache-2.0"),
		ProjectURL: utils.Ptr("https://commons.apache.org/proper/commons-io/"),
		Author:     utils.Ptr("The Apache Software Foundation"),
		Builds: []api.ReleaseInfo{
			{Version: packageVersion, CreatedAt: "2024-01-15T10:30:00Z"},
		},
	}, nil)

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s/%s?limit=100&offset=0", api.FullRootPath(), repoUUID, groupID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	rr := suite.recordPackages(req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var response api.MavenPackageDetailResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
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

func (suite *PackagesSuite) TestGetPackageDetailDaoError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	groupID := "commons-io"
	packageName := "commons-io"
	packageVersion := "2.11.0"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("GetMavenPackageDetail", test.MockCtx(), repoUUID, groupID, packageName, packageVersion).
		Return(api.MavenPackageDetailResponse{}, fmt.Errorf("failed to get lightwell maven package detail"))

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s/%s?limit=100&offset=0", api.FullRootPath(), repoUUID, groupID, packageName, packageVersion)
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

	orgID := test_handler.MockOrgId
	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s/%s", api.FullRootPath(), repoUUID, "some.group", "some-package", "1.0.0")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *PackagesSuite) TestGetPackageDetailRepoNotFound() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440003"

	orgID := test_handler.MockOrgId
	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{}}, int64(0), nil)

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s/%s", api.FullRootPath(), repoUUID, "some.group", "some-package", "1.0.0")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestGetPackageDetailEmptyBuilds() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440000"
	groupID := "io.smallrye.reactive"
	packageName := "smallrye-mutiny-vertx-core"
	packageVersion := "3.15.0"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("GetMavenPackageDetail", test.MockCtx(), repoUUID, groupID, packageName, packageVersion).
		Return(api.MavenPackageDetailResponse{}, &ce.DaoError{NotFound: true, Message: "Package not found"})

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s/%s?limit=100&offset=0", api.FullRootPath(), repoUUID, groupID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestListPackagesMavenSearchGroupArtifact() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440010"
	search := "io.smallrye.reactive:smallrye-mutiny-vertx-core"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, search, "group/name", "asc", 100, 0).
		Return(api.PackageResponse{
			Results: []api.PackageItem{{
				Group:    "io.smallrye.reactive",
				Name:     "smallrye-mutiny-vertx-core",
				Versions: []string{"3.16.0"},
			}},
			Total:  1,
			Limit:  100,
			Offset: 0,
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0&search=%s&sort_by=name", api.FullRootPath(), repoUUID, search)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PackageResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 1, response.Total)
	assert.Equal(t, "io.smallrye.reactive", response.Results[0].Group)
}

func (suite *PackagesSuite) TestListPackagesMavenPagination() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440011"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, "", "last_updated", "desc", 2, 1).
		Return(api.PackageResponse{
			Results: []api.PackageItem{
				{Group: "org.example", Name: "lib-a", Versions: []string{"1.0.0"}},
				{Group: "org.example", Name: "lib-b", Versions: []string{"2.0.0"}},
			},
			Total:  5,
			Limit:  2,
			Offset: 1,
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=2&offset=1&sort_by=last_updated", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PackageResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 5, response.Total)
	assert.Equal(t, 2, response.Limit)
	assert.Equal(t, 1, response.Offset)
	assert.Equal(t, 2, len(response.Results))
}

func (suite *PackagesSuite) TestGetPackageDetailBaseVersionExact() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440012"
	groupID := "org.example"
	packageName := "lib"
	packageVersion := "5.3.18"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("GetMavenPackageDetail", test.MockCtx(), repoUUID, groupID, packageName, packageVersion).Return(api.MavenPackageDetailResponse{
		Group:   groupID,
		Name:    packageName,
		Version: packageVersion,
		Builds: []api.ReleaseInfo{
			{Version: "5.3.18", Release: "rhlw-00003", CreatedAt: "2024-03-01T00:00:00Z"},
		},
	}, nil)

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s/%s", api.FullRootPath(), repoUUID, groupID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.MavenPackageDetailResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	require.Len(t, response.Builds, 1)
	assert.Equal(t, "5.3.18", response.Builds[0].Version)
	assert.Equal(t, "rhlw-00003", response.Builds[0].Release)
	assert.NotContains(t, response.Builds[0].Version, "5.3.180")
}

func (suite *PackagesSuite) TestListMavenPackageVersionsMultipleRebuilds() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440013"
	groupID := "org.example"
	packageName := "lib"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListMavenPackageVersions", test.MockCtx(), repoUUID, groupID, packageName).Return(api.MavenPackageVersionsResponse{
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

	path := fmt.Sprintf("%s/repositories/%s/java_packages/%s/%s", api.FullRootPath(), repoUUID, groupID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.MavenPackageVersionsResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	require.Len(t, response.Versions, 1)
	assert.Equal(t, "5.3.18", response.Versions[0].Version)
	require.Len(t, response.Versions[0].Builds, 2)
	assert.Equal(t, "rhlw-00003", response.Versions[0].Builds[0].Release)
	assert.Equal(t, "rhlw-00001", response.Versions[0].Builds[1].Release)
	assert.Equal(t, "5.3.18", response.Versions[0].Builds[0].Version)
	assert.Equal(t, "5.3.18", response.Versions[0].Builds[1].Version)
}

func (suite *PackagesSuite) TestGetPythonPackageVersionsSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"
	packageName := "django"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypePython,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListPythonPackageVersions", test.MockCtx(), repoUUID, packageName).Return(api.PythonPackageVersionsResponse{
		Name: packageName,
		Versions: []api.PythonPackageDetailResponse{
			{
				Name:            packageName,
				Version:         "4.25.7+rhlw.3",
				Summary:         "A high-level Python web framework",
				LastUpdated:     "2024-01-01T12:00:00Z",
				UpstreamVersion: "4.25.7",
				Distributions:   []api.PythonDistribution{},
			},
			{
				Name:            packageName,
				Version:         "5.0",
				Summary:         "A high-level Python web framework",
				LastUpdated:     "2024-02-01T12:00:00Z",
				UpstreamVersion: "5.0",
				Distributions:   []api.PythonDistribution{},
			},
		},
	}, nil)

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
	assert.Equal(t, "4.25.7+rhlw.3", response.Versions[0].Version)
	assert.Equal(t, "4.25.7", response.Versions[0].UpstreamVersion)
	assert.Empty(t, response.Versions[0].Distributions)
	assert.Equal(t, "5.0", response.Versions[1].Version)
	assert.Empty(t, response.Versions[1].Distributions)
}

func (suite *PackagesSuite) TestGetPythonPackageVersionsNonPythonRepo() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440001"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := test_handler.MockOrgId
	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)

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
	packageName := "nonexistent"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypePython,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListPythonPackageVersions", test.MockCtx(), repoUUID, packageName).
		Return(api.PythonPackageVersionsResponse{}, &ce.DaoError{NotFound: true, Message: "Package not found"})

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s", api.FullRootPath(), repoUUID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestGetPythonPackageVersionsDaoError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"
	packageName := "django"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypePython,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListPythonPackageVersions", test.MockCtx(), repoUUID, packageName).
		Return(api.PythonPackageVersionsResponse{}, fmt.Errorf("failed to list lightwell python package versions"))

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
	packageName := "django"
	packageVersion := "5.0"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypePython,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("GetPythonPackageDetail", test.MockCtx(), repoUUID, packageName, packageVersion).Return(api.PythonPackageDetailResponse{
		Name:    packageName,
		Version: packageVersion,
		Builds: []api.ReleaseInfo{
			{Version: packageVersion, Release: "rhlw.3", CreatedAt: "2024-02-01T00:00:00Z"},
			{Version: packageVersion, Release: "rhlw.1", CreatedAt: "2024-01-01T00:00:00Z"},
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
	assert.Equal(t, "2024-02-01T00:00:00Z", response.LastUpdated)
	assert.Equal(t, "BSD-3-Clause", response.License)
	assert.Equal(t, "Django Software Foundation", response.Author.Name)
	assert.Equal(t, "foundation@djangoproject.com", response.Author.Email)
	assert.Equal(t, "5.0", response.UpstreamVersion)
	assert.Equal(t, "https://www.djangoproject.com/", response.ProjectURL)
	assert.Equal(t, []api.ReleaseInfo{
		{Version: packageVersion, Release: "rhlw.3", CreatedAt: "2024-02-01T00:00:00Z"},
		{Version: packageVersion, Release: "rhlw.1", CreatedAt: "2024-01-01T00:00:00Z"},
	}, response.Builds)
	assert.Empty(t, response.Distributions)
}

func (suite *PackagesSuite) TestGetPythonPackageDetailNonPythonRepo() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440001"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := test_handler.MockOrgId
	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)

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
	packageName := "django"
	packageVersion := "9.9.9"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypePython,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("GetPythonPackageDetail", test.MockCtx(), repoUUID, packageName, packageVersion).
		Return(api.PythonPackageDetailResponse{}, &ce.DaoError{NotFound: true, Message: "Package not found"})

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s/%s", api.FullRootPath(), repoUUID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestGetPythonPackageDetailDaoError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440002"
	packageName := "django"
	packageVersion := "5.0"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypePython,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("GetPythonPackageDetail", test.MockCtx(), repoUUID, packageName, packageVersion).
		Return(api.PythonPackageDetailResponse{}, fmt.Errorf("failed to get lightwell python package detail"))

	path := fmt.Sprintf("%s/repositories/%s/python_packages/%s/%s", api.FullRootPath(), repoUUID, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *PackagesSuite) TestListPackagesNpmSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440009"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeNpm,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, "", "last_updated", "desc", 100, 0).
		Return(api.PackageResponse{
			Results: []api.PackageItem{{
				Group:          "@types",
				Name:           "is-odd",
				Versions:       []string{"3.0.0"},
				LatestReleases: []api.ReleaseInfo{{Version: "3.0.0", CreatedAt: "2024-01-15T10:30:00Z"}},
			}},
			Total:  1,
			Limit:  100,
			Offset: 0,
		}, nil)

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
	assert.Equal(t, "@types", response.Results[0].Group)
	assert.Equal(t, "is-odd", response.Results[0].Name)
	assert.Equal(t, 1, len(response.Results[0].Versions))
	assert.Equal(t, 1, len(response.Results[0].LatestReleases))
	assert.Equal(t, "3.0.0", response.Results[0].LatestReleases[0].Version)
}

func (suite *PackagesSuite) TestListPackagesNpmWithFilter() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440010"
	search := "@types/"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeNpm,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, search, "group/name", "asc", 100, 0).
		Return(api.PackageResponse{
			Results: []api.PackageItem{{
				Group: "@types",
				Name:  "is-odd",
			}},
			Total:  1,
			Limit:  100,
			Offset: 0,
		}, nil)

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0&search=%s&sort_by=name", api.FullRootPath(), repoUUID, search)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.PackageResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Results))
	assert.Equal(t, "@types", response.Results[0].Group)
	assert.Equal(t, "is-odd", response.Results[0].Name)
}

func (suite *PackagesSuite) TestListPackagesNpmDaoError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440011"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeNpm,
	}

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListRepositoryPackages", test.MockCtx(), repoUUID, "", "last_updated", "desc", 100, 0).
		Return(api.PackageResponse{}, fmt.Errorf("failed to list lightwell repository packages"))

	path := fmt.Sprintf("%s/repositories/%s/packages?limit=100&offset=0", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *PackagesSuite) TestGetNpmPackageVersionsSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440012"
	scope := "@types"
	packageName := "is-odd"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeNpm,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListNpmPackageVersions", test.MockCtx(), repoUUID, scope, packageName).Return(api.NpmPackageVersionsResponse{
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
					{Version: "3.0.0", Release: "rhlw.1", CreatedAt: "2024-01-01T00:00:00Z"},
				},
			},
		},
	}, nil)

	path := fmt.Sprintf("%s/repositories/%s/js_packages/%s/%s", api.FullRootPath(), repoUUID, scope, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.NpmPackageVersionsResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, scope, response.Scope)
	assert.Equal(t, packageName, response.Name)
	require.Len(t, response.Versions, 1)
	assert.Equal(t, "3.0.0", response.Versions[0].Version)
	assert.Equal(t, "2024-04-01T00:00:00Z", response.Versions[0].LastUpdated)
	assert.Equal(t, "3.0.0", response.Versions[0].UpstreamVersion)
	assert.Equal(t, []api.ReleaseInfo{
		{Version: "3.0.0", Release: "rhlw.3", CreatedAt: "2024-04-01T00:00:00Z"},
		{Version: "3.0.0", Release: "rhlw.1", CreatedAt: "2024-01-01T00:00:00Z"},
	}, response.Versions[0].Builds)
	assert.Empty(t, response.Versions[0].Tarball.Filename)
}

func (suite *PackagesSuite) TestGetNpmPackageVersionsUnscopedSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440013"
	packageName := "left-pad"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeNpm,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListNpmPackageVersions", test.MockCtx(), repoUUID, "-", packageName).Return(api.NpmPackageVersionsResponse{
		Scope: "-",
		Name:  packageName,
		Versions: []api.NpmPackageDetailResponse{
			{
				Scope:           "-",
				Name:            packageName,
				Version:         "1.0.0",
				UpstreamVersion: "1.0.0",
				Builds: []api.ReleaseInfo{
					{Version: "1.0.0", CreatedAt: "2024-05-01T00:00:00Z"},
				},
			},
		},
	}, nil)

	path := fmt.Sprintf("%s/repositories/%s/js_packages/-/%s", api.FullRootPath(), repoUUID, packageName)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.NpmPackageVersionsResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, "-", response.Scope)
	assert.Equal(t, packageName, response.Name)
	require.Len(t, response.Versions, 1)
	assert.Equal(t, "1.0.0", response.Versions[0].Version)
}

func (suite *PackagesSuite) TestGetNpmPackageVersionsNonNpmRepo() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440001"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := test_handler.MockOrgId
	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)

	path := fmt.Sprintf("%s/repositories/%s/js_packages/@types/is-odd", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *PackagesSuite) TestGetNpmPackageVersionsEmpty() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440012"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeNpm,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListNpmPackageVersions", test.MockCtx(), repoUUID, "@types", "is-odd").
		Return(api.NpmPackageVersionsResponse{}, &ce.DaoError{NotFound: true, Message: "Package not found"})

	path := fmt.Sprintf("%s/repositories/%s/js_packages/@types/is-odd", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestGetNpmPackageVersionsDaoError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440012"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeNpm,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("ListNpmPackageVersions", test.MockCtx(), repoUUID, "@types", "is-odd").
		Return(api.NpmPackageVersionsResponse{}, fmt.Errorf("failed to list lightwell npm package versions"))

	path := fmt.Sprintf("%s/repositories/%s/js_packages/@types/is-odd", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *PackagesSuite) TestGetNpmPackageDetailSuccess() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440012"
	scope := "@types"
	packageName := "is-odd"
	packageVersion := "3.0.0"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeNpm,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("GetNpmPackageDetail", test.MockCtx(), repoUUID, scope, packageName, packageVersion).Return(api.NpmPackageDetailResponse{
		Scope:           scope,
		Name:            packageName,
		Version:         packageVersion,
		LastUpdated:     "2024-04-01T00:00:00Z",
		UpstreamVersion: "3.0.0",
		Builds: []api.ReleaseInfo{
			{Version: "3.0.0", Release: "rhlw.3", CreatedAt: "2024-04-01T00:00:00Z"},
			{Version: "3.0.0", Release: "rhlw.1", CreatedAt: "2024-01-01T00:00:00Z"},
		},
	}, nil)

	path := fmt.Sprintf("%s/repositories/%s/js_packages/%s/%s/%s", api.FullRootPath(), repoUUID, scope, packageName, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.NpmPackageDetailResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, scope, response.Scope)
	assert.Equal(t, packageName, response.Name)
	assert.Equal(t, packageVersion, response.Version)
	assert.Equal(t, "2024-04-01T00:00:00Z", response.LastUpdated)
	assert.Empty(t, response.Tarball.Filename)
	assert.Equal(t, "3.0.0", response.UpstreamVersion)
	assert.Equal(t, []api.ReleaseInfo{
		{Version: "3.0.0", Release: "rhlw.3", CreatedAt: "2024-04-01T00:00:00Z"},
		{Version: "3.0.0", Release: "rhlw.1", CreatedAt: "2024-01-01T00:00:00Z"},
	}, response.Builds)
}

func (suite *PackagesSuite) TestGetNpmPackageDetailNonNpmRepo() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440001"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeMaven,
	}

	orgID := test_handler.MockOrgId
	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)

	path := fmt.Sprintf("%s/repositories/%s/js_packages/@types/is-odd/3.0.0", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *PackagesSuite) TestGetNpmPackageDetailEmpty() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440012"
	packageVersion := "9.9.9"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeNpm,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("GetNpmPackageDetail", test.MockCtx(), repoUUID, "@types", "is-odd", packageVersion).
		Return(api.NpmPackageDetailResponse{}, &ce.DaoError{NotFound: true, Message: "Package not found"})

	path := fmt.Sprintf("%s/repositories/%s/js_packages/@types/is-odd/%s", api.FullRootPath(), repoUUID, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *PackagesSuite) TestGetNpmPackageDetailDaoError() {
	t := suite.T()
	repoUUID := "550e8400-e29b-41d4-a716-446655440012"
	packageVersion := "3.0.0"

	repo := api.RepositoryResponse{
		UUID:        repoUUID,
		ContentType: config.ContentTypeNpm,
	}

	orgID := test_handler.MockOrgId

	suite.reg.RepositoryConfig.On("List", test.MockCtx(), orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil)
	suite.reg.LightwellPackage.On("GetNpmPackageDetail", test.MockCtx(), repoUUID, "@types", "is-odd", packageVersion).
		Return(api.NpmPackageDetailResponse{}, fmt.Errorf("failed to get lightwell npm package detail"))

	path := fmt.Sprintf("%s/repositories/%s/js_packages/@types/is-odd/%s", api.FullRootPath(), repoUUID, packageVersion)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.servePackagesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}
