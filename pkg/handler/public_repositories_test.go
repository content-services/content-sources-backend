package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PublicReposSuite struct {
	suite.Suite
	reg *dao.MockDaoRegistry
}

func TestPublicReposSuite(t *testing.T) {
	suite.Run(t, new(PublicReposSuite))
}
func (suite *PublicReposSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
}

func (suite *PublicReposSuite) serveRepositoriesRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	RegisterPublicRepositoriesRoutes(pathPrefix, suite.reg.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *PublicReposSuite) TestList() {
	t := suite.T()

	collection := createPublicRepoCollection(1, 10, 0)
	paginationData := api.PaginationData{Limit: 10, Offset: DefaultOffset}
	suite.reg.Repository.On("ListPublic", test.MockCtx(), paginationData, api.FilterData{}).Return(collection, int64(1), nil)

	path := fmt.Sprintf("%s/public_repositories/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	response := api.PublicRepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(1), response.Meta.Count)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, collection.Data[0].URL, response.Data[0].URL)
	assert.Equal(t, collection.Data[0].LastIntrospectionUpdateTime, response.Data[0].LastIntrospectionUpdateTime)
	assert.Equal(t, collection.Data[0].LastIntrospectionTime, response.Data[0].LastIntrospectionTime)
	assert.Equal(t, collection.Data[0].LastIntrospectionSuccessTime, response.Data[0].LastIntrospectionSuccessTime)
	assert.Equal(t, collection.Data[0].LastIntrospectionError, response.Data[0].LastIntrospectionError)
}

func (suite *PublicReposSuite) TestListNoRepositories() {
	t := suite.T()

	collection := api.PublicRepositoryCollectionResponse{}
	paginationData := api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset}
	suite.reg.Repository.On("ListPublic", test.MockCtx(), paginationData, api.FilterData{}).Return(collection, int64(0), nil)

	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/public_repositories/", nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(0), response.Meta.Count)
	assert.Equal(t, 100, response.Meta.Limit)
	assert.Equal(t, 0, len(response.Data))
	assert.Equal(t, api.FullRootPath()+"/public_repositories/?limit=100&offset=0", response.Links.Last)
	assert.Equal(t, api.FullRootPath()+"/public_repositories/?limit=100&offset=0", response.Links.First)
}

func (suite *PublicReposSuite) TestListPagedExtraRemaining() {
	t := suite.T()

	collection := api.PublicRepositoryCollectionResponse{}
	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 100}

	suite.reg.Repository.On("ListPublic", test.MockCtx(), paginationData1, api.FilterData{}).Return(collection, int64(102), nil).Once()
	suite.reg.Repository.On("ListPublic", test.MockCtx(), paginationData2, api.FilterData{}).Return(collection, int64(102), nil).Once()

	path := fmt.Sprintf("%s/public_repositories/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.PublicRepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(102), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Fetch last page
	req = httptest.NewRequest(http.MethodGet, response.Links.Last, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, body, err = suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.PublicRepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func (suite *PublicReposSuite) TestListPagedNoRemaining() {
	t := suite.T()

	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 90}

	collection := api.PublicRepositoryCollectionResponse{}
	suite.reg.Repository.On("ListPublic", test.MockCtx(), paginationData1, api.FilterData{}).Return(collection, int64(100), nil)
	suite.reg.Repository.On("ListPublic", test.MockCtx(), paginationData2, api.FilterData{}).Return(collection, int64(100), nil)

	path := fmt.Sprintf("%s/public_repositories/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.PublicRepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(100), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Fetch last page
	req = httptest.NewRequest(http.MethodGet, response.Links.Last, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, body, err = suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.PublicRepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func (suite *PublicReposSuite) TestListDaoError() {
	t := suite.T()

	daoError := ce.DaoError{
		Message: "Column doesn't exist",
	}
	paginationData := api.PaginationData{Limit: DefaultLimit}

	suite.reg.Repository.On("ListPublic", test.MockCtx(), paginationData, api.FilterData{}).
		Return(api.PublicRepositoryCollectionResponse{}, int64(0), &daoError)

	path := fmt.Sprintf("%s/public_repositories/", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func createPublicRepoCollection(size, limit, offset int) api.PublicRepositoryCollectionResponse {
	repos := make([]api.PublicRepositoryResponse, size)
	for i := 0; i < size; i++ {
		repo := api.PublicRepositoryResponse{
			URL:                          fmt.Sprintf("http://repo-%d.com", i),
			LastIntrospectionTime:        "2022-08-31 14:17:50.257623 -0400 EDT",
			LastIntrospectionSuccessTime: "2022-08-31 14:17:50.257623 -0400 EDT",
			LastIntrospectionUpdateTime:  "2022-08-31 14:17:50.257623 -0400 EDT",
			LastIntrospectionError:       "",
			Status:                       "Valid",
		}
		repos[i] = repo
	}
	collection := api.PublicRepositoryCollectionResponse{
		Data: repos,
	}
	params := fmt.Sprintf("?offset=%d&limit=%d", offset, limit)
	setCollectionResponseMetadata(&collection, getTestContext(params), int64(size))
	return collection
}
