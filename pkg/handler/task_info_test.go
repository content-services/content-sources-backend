package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func createTaskCollection(size, limit, offset int) api.TaskInfoCollectionResponse {
	tasks := make([]api.TaskInfoResponse, size)
	for i := 0; i < size; i++ {
		tasks[i] = api.TaskInfoResponse{
			UUID:      fmt.Sprintf("%d", i),
			Status:    fmt.Sprintf("status of task %d", i),
			CreatedAt: "2022-08-31 14:17:50.257623 -0400 EDT",
			EndedAt:   "2022-08-31 14:17:50.257623 -0400 EDT",
			Error:     fmt.Sprintf("error of task %d", i),
			OrgId:     test_handler.MockOrgId,
		}
	}
	collection := api.TaskInfoCollectionResponse{
		Data: tasks,
	}
	params := fmt.Sprintf("?offset=%d&limit=%d", offset, limit)
	setCollectionResponseMetadata(&collection, getTestContext(params), int64(size))
	return collection
}

func (suite *TaskInfoSuite) serveTasksRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	th := TaskInfoHandler{
		TaskClient: suite.tcMock,
	}

	RegisterTaskInfoRoutes(pathPrefix, suite.reg.ToDaoRegistry(), &th.TaskClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

type TaskInfoSuite struct {
	suite.Suite
	reg    *dao.MockDaoRegistry
	tcMock *client.MockTaskClient
}

func TestTaskInfoSuite(t *testing.T) {
	suite.Run(t, new(TaskInfoSuite))
}
func (suite *TaskInfoSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
	suite.tcMock = client.NewMockTaskClient(suite.T())
}

func (suite *TaskInfoSuite) TestSimple() {
	t := suite.T()

	collection := createTaskCollection(1, 10, 0)
	paginationData := api.PaginationData{Limit: 10, Offset: DefaultOffset}
	suite.reg.TaskInfo.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.TaskInfoFilterData{}).Return(collection, int64(1), nil)

	path := fmt.Sprintf("%s/tasks/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveTasksRouter(req)
	assert.Nil(t, err)

	response := api.TaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(1), response.Meta.Count)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, collection.Data[0].UUID, response.Data[0].UUID)
	assert.Equal(t, collection.Data[0].Status, response.Data[0].Status)
	assert.Equal(t, collection.Data[0].CreatedAt, response.Data[0].CreatedAt)
	assert.Equal(t, collection.Data[0].EndedAt, response.Data[0].EndedAt)
	assert.Equal(t, collection.Data[0].Error, response.Data[0].Error)
	assert.Equal(t, collection.Data[0].OrgId, response.Data[0].OrgId)
}

func (suite *TaskInfoSuite) TestListNoTasks() {
	t := suite.T()

	collection := api.TaskInfoCollectionResponse{}
	paginationData := api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset}
	suite.reg.TaskInfo.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.TaskInfoFilterData{}).Return(collection, int64(0), nil)

	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/tasks/", nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveTasksRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.TaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(0), response.Meta.Count)
	assert.Equal(t, 100, response.Meta.Limit)
	assert.Equal(t, 0, len(response.Data))
	assert.Equal(t, api.FullRootPath()+"/tasks/?limit=100&offset=0", response.Links.Last)
	assert.Equal(t, api.FullRootPath()+"/tasks/?limit=100&offset=0", response.Links.First)
}

func (suite *TaskInfoSuite) TestListPagedExtraRemaining() {
	t := suite.T()

	collection := api.TaskInfoCollectionResponse{}
	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 100}

	suite.reg.TaskInfo.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData1, api.TaskInfoFilterData{}).Return(collection, int64(102), nil).Once()
	suite.reg.TaskInfo.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData2, api.TaskInfoFilterData{}).Return(collection, int64(102), nil).Once()

	path := fmt.Sprintf("%s/tasks/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveTasksRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.TaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(102), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Fetch last page
	req = httptest.NewRequest(http.MethodGet, response.Links.Last, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, body, err = suite.serveTasksRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.TaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func (suite *TaskInfoSuite) TestListPagedNoRemaining() {
	t := suite.T()

	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 90}

	collection := api.TaskInfoCollectionResponse{}
	suite.reg.TaskInfo.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData1, api.TaskInfoFilterData{}).Return(collection, int64(100), nil)
	suite.reg.TaskInfo.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData2, api.TaskInfoFilterData{}).Return(collection, int64(100), nil)

	path := fmt.Sprintf("%s/tasks/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveTasksRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.TaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(100), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Fetch last page
	req = httptest.NewRequest(http.MethodGet, response.Links.Last, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, body, err = suite.serveTasksRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.TaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func (suite *TaskInfoSuite) TestListStatusFilter() {
	t := suite.T()

	paginationData := api.PaginationData{Limit: 10, Offset: 0}
	filterData := api.TaskInfoFilterData{Status: "completed"}

	collection := api.TaskInfoCollectionResponse{}
	suite.reg.TaskInfo.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, filterData).Return(collection, int64(100), nil)
	suite.reg.TaskInfo.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.TaskInfoFilterData{}).Return(collection, int64(110), nil)

	// Listing with filter
	path := fmt.Sprintf("%s/tasks/?limit=%d&status=%s", api.FullRootPath(), 10, filterData.Status)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveTasksRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.TaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(100), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Listing without filter
	path = fmt.Sprintf("%s/tasks/?limit=%d", api.FullRootPath(), 10)
	req = httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err = suite.serveTasksRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.TaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(110), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)
}

func (suite *TaskInfoSuite) TestListTypeFilter() {
	t := suite.T()

	paginationData := api.PaginationData{Limit: 10, Offset: 0}
	filterData := api.TaskInfoFilterData{Typename: "snapshot"}

	collection := api.TaskInfoCollectionResponse{}
	suite.reg.TaskInfo.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, filterData).Return(collection, int64(100), nil)
	suite.reg.TaskInfo.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.TaskInfoFilterData{}).Return(collection, int64(110), nil)

	// Listing with filter
	path := fmt.Sprintf("%s/tasks/?limit=%d&type=%s", api.FullRootPath(), 10, filterData.Typename)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveTasksRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.TaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(100), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Listing without filter
	path = fmt.Sprintf("%s/tasks/?limit=%d", api.FullRootPath(), 10)
	req = httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err = suite.serveTasksRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.TaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(110), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)
}

func (suite *TaskInfoSuite) TestListRepoUuidFilter() {
	t := suite.T()

	paginationData := api.PaginationData{Limit: 10, Offset: 0}
	filterData := api.TaskInfoFilterData{RepoConfigUUID: "abc"}

	collection := api.TaskInfoCollectionResponse{}
	suite.reg.TaskInfo.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, filterData).Return(collection, int64(100), nil)
	suite.reg.TaskInfo.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.TaskInfoFilterData{}).Return(collection, int64(110), nil)

	// Listing with filter
	path := fmt.Sprintf("%s/tasks/?limit=%d&repository_uuid=%s", api.FullRootPath(), 10, filterData.RepoConfigUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveTasksRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.TaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(100), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Listing without filter
	path = fmt.Sprintf("%s/tasks/?limit=%d", api.FullRootPath(), 10)
	req = httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err = suite.serveTasksRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.TaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(110), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)
}

func (suite *TaskInfoSuite) TestFetch() {
	t := suite.T()

	uuid := "abcadaba"
	task := api.TaskInfoResponse{
		UUID:           uuid,
		Status:         "status",
		CreatedAt:      time.Now().Format(time.RFC3339),
		EndedAt:        time.Now().Format(time.RFC3339),
		Error:          "error",
		OrgId:          "org id",
		RepoConfigUUID: "abc",
		RepoConfigName: "repo1",
	}

	suite.reg.TaskInfo.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(task, nil)

	body, err := json.Marshal(task)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/tasks/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveTasksRouter(req)
	assert.Nil(t, err)

	var response api.TaskInfoResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.NotEmpty(t, response.UUID)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, task, response)
}

func (suite *TaskInfoSuite) TestFetchNotFound() {
	t := suite.T()

	uuid := "abcadaba"
	task := api.TaskInfoResponse{
		UUID:           uuid,
		Status:         "status",
		CreatedAt:      time.Now().Format(time.RFC3339),
		EndedAt:        time.Now().Format(time.RFC3339),
		Error:          "error",
		OrgId:          "org id",
		RepoConfigUUID: "abc",
		RepoConfigName: "repo1",
	}

	daoError := ce.DaoError{
		NotFound: true,
		Message:  "Not found",
	}
	suite.reg.TaskInfo.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(api.TaskInfoResponse{}, &daoError)

	body, err := json.Marshal(task)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/tasks/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, _ := suite.serveTasksRouter(req)
	assert.Equal(t, http.StatusNotFound, code)
}
