package handler

import (
	"bytes"
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
	"github.com/content-services/content-sources-backend/pkg/seeds"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func createAdminTaskCollection(size, limit, offset int) api.AdminTaskInfoCollectionResponse {
	tasks := make([]api.AdminTaskInfoResponse, size)
	payload, _ := json.Marshal(map[string]string{"url": "https://example.com"})
	for i := 0; i < size; i++ {
		tasks[i] = api.AdminTaskInfoResponse{
			UUID:       fmt.Sprintf("%d", i),
			Status:     fmt.Sprintf("status of task %d", i),
			Typename:   fmt.Sprintf("type of task %d", i),
			QueuedAt:   "2022-08-31 14:17:50.257623 -0400 EDT",
			StartedAt:  "2022-08-31 14:17:50.257623 -0400 EDT",
			FinishedAt: "2022-08-31 14:17:50.257623 -0400 EDT",
			Error:      fmt.Sprintf("error of task %d", i),
			AccountId:  test_handler.MockAccountNumber,
			OrgId:      test_handler.MockOrgId,
			Payload:    payload,
		}
	}
	collection := api.AdminTaskInfoCollectionResponse{
		Data: tasks,
	}
	params := fmt.Sprintf("?offset=%d&limit=%d", offset, limit)
	setCollectionResponseMetadata(&collection, getTestContext(params), int64(size))
	return collection
}

func createAdminTask() api.AdminTaskInfoResponse {
	payload, _ := json.Marshal(map[string]string{"url": "https://example.com"})
	return api.AdminTaskInfoResponse{
		UUID:       uuid.NewString(),
		Status:     "test status",
		Typename:   "test type",
		QueuedAt:   "2022-08-31 14:17:50.257623 -0400 EDT",
		StartedAt:  "2022-08-31 14:17:50.257623 -0400 EDT",
		FinishedAt: "2022-08-31 14:17:50.257623 -0400 EDT",
		Error:      "test error",
		AccountId:  test_handler.MockAccountNumber,
		OrgId:      test_handler.MockOrgId,
		Payload:    payload,
	}
}

func (suite *AdminTasksSuite) serveAdminTasksRouter(req *http.Request, enabled bool, authorized bool) (int, []byte, error) {
	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(fullRootPath())

	if enabled {
		config.Get().Features.AdminTasks.Enabled = true
	} else {
		config.Get().Features.AdminTasks.Enabled = false
	}

	if authorized {
		config.Get().Features.AdminTasks.Accounts = &[]string{test_handler.MockAccountNumber}
	} else {
		config.Get().Features.AdminTasks.Accounts = &[]string{seeds.RandomAccountId()}
	}

	RegisterAdminTaskRoutes(pathPrefix, suite.reg.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

type AdminTasksSuite struct {
	suite.Suite
	reg *dao.MockDaoRegistry
}

func TestAdminTasksSuite(t *testing.T) {
	suite.Run(t, new(AdminTasksSuite))
}
func (suite *AdminTasksSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
}

func (suite *AdminTasksSuite) TestSimple() {
	t := suite.T()

	collection := createAdminTaskCollection(1, 10, 0)
	paginationData := api.PaginationData{Limit: 10, Offset: DefaultOffset}
	filterData := api.AdminTaskFilterData{}
	suite.reg.AdminTask.On("List", paginationData, filterData).Return(collection, int64(1), nil)

	path := fmt.Sprintf("%s/admin/tasks/?limit=%d", fullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveAdminTasksRouter(req, true, true)
	assert.Nil(t, err)

	response := api.AdminTaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(1), response.Meta.Count)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, 1, len(response.Data))

	assert.Equal(t, collection.Data[0].UUID, response.Data[0].UUID)
	assert.Equal(t, collection.Data[0].Status, response.Data[0].Status)
	assert.Equal(t, collection.Data[0].Typename, response.Data[0].Typename)
	assert.Equal(t, collection.Data[0].QueuedAt, response.Data[0].QueuedAt)
	assert.Equal(t, collection.Data[0].StartedAt, response.Data[0].StartedAt)
	assert.Equal(t, collection.Data[0].FinishedAt, response.Data[0].FinishedAt)
	assert.Equal(t, collection.Data[0].Error, response.Data[0].Error)
	assert.Equal(t, collection.Data[0].OrgId, response.Data[0].OrgId)
	assert.Equal(t, collection.Data[0].AccountId, response.Data[0].AccountId)
	assert.Equal(t, collection.Data[0].Payload, response.Data[0].Payload)
}

func (suite *AdminTasksSuite) TestListNoTasks() {
	t := suite.T()

	collection := api.AdminTaskInfoCollectionResponse{}
	paginationData := api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset}
	filterData := api.AdminTaskFilterData{}
	suite.reg.AdminTask.On("List", paginationData, filterData).Return(collection, int64(0), nil)

	req := httptest.NewRequest(http.MethodGet, fullRootPath()+"/admin/tasks/", nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveAdminTasksRouter(req, true, true)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.AdminTaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(0), response.Meta.Count)
	assert.Equal(t, 100, response.Meta.Limit)
	assert.Equal(t, 0, len(response.Data))
	assert.Equal(t, fullRootPath()+"/admin/tasks/?limit=100&offset=0", response.Links.Last)
	assert.Equal(t, fullRootPath()+"/admin/tasks/?limit=100&offset=0", response.Links.First)
}

func (suite *AdminTasksSuite) TestListDisabled() {
	t := suite.T()

	req := httptest.NewRequest(http.MethodGet, fullRootPath()+"/admin/tasks/", nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveAdminTasksRouter(req, false, false)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Admin tasks feature is disabled.")

	paginationData := api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset}
	filterData := api.AdminTaskFilterData{}
	suite.reg.AdminTask.AssertNotCalled(t, "List", paginationData, filterData)
}

func (suite *AdminTasksSuite) TestListNotAccessible() {
	t := suite.T()

	req := httptest.NewRequest(http.MethodGet, fullRootPath()+"/admin/tasks/", nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveAdminTasksRouter(req, true, false)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Neither the user nor account is allowed.")

	paginationData := api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset}
	filterData := api.AdminTaskFilterData{}
	suite.reg.AdminTask.AssertNotCalled(t, "List", paginationData, filterData)
}

func (suite *AdminTasksSuite) TestListPagedExtraRemaining() {
	t := suite.T()

	collection := api.AdminTaskInfoCollectionResponse{}
	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 100}

	suite.reg.AdminTask.On("List", paginationData1, api.AdminTaskFilterData{}).Return(collection, int64(102), nil).Once()
	suite.reg.AdminTask.On("List", paginationData2, api.AdminTaskFilterData{}).Return(collection, int64(102), nil).Once()

	path := fmt.Sprintf("%s/admin/tasks/?limit=%d", fullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveAdminTasksRouter(req, true, true)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.AdminTaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(102), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Fetch last page
	req = httptest.NewRequest(http.MethodGet, response.Links.Last, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, body, err = suite.serveAdminTasksRouter(req, true, true)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.AdminTaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func (suite *AdminTasksSuite) TestListPagedNoRemaining() {
	t := suite.T()

	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 90}

	collection := api.AdminTaskInfoCollectionResponse{}
	suite.reg.AdminTask.On("List", paginationData1, api.AdminTaskFilterData{}).Return(collection, int64(100), nil)
	suite.reg.AdminTask.On("List", paginationData2, api.AdminTaskFilterData{}).Return(collection, int64(100), nil)

	path := fmt.Sprintf("%s/admin/tasks/?limit=%d", fullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveAdminTasksRouter(req, true, true)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.AdminTaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(100), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Fetch last page
	req = httptest.NewRequest(http.MethodGet, response.Links.Last, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, body, err = suite.serveAdminTasksRouter(req, true, true)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.AdminTaskInfoCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func (suite *AdminTasksSuite) TestFetch() {
	t := suite.T()

	task := createAdminTask()

	suite.reg.AdminTask.On("Fetch", task.UUID).Return(task, nil)

	var body []byte

	req := httptest.NewRequest(http.MethodGet, fullRootPath()+"/admin/tasks/"+task.UUID,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveAdminTasksRouter(req, true, true)
	assert.Nil(t, err)

	var response api.AdminTaskInfoResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.NotEmpty(t, response.UUID)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, task, response)
}

func (suite *AdminTasksSuite) TestFetchNotFound() {
	t := suite.T()

	task := createAdminTask()

	daoError := ce.DaoError{
		NotFound: true,
		Message:  "Not found",
	}
	suite.reg.AdminTask.On("Fetch", task.UUID).Return(api.AdminTaskInfoResponse{}, &daoError)

	var body []byte

	req := httptest.NewRequest(http.MethodGet, fullRootPath()+"/admin/tasks/"+task.UUID,
		bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, _ := suite.serveAdminTasksRouter(req, true, true)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *AdminTasksSuite) TestFetchDisabled() {
	t := suite.T()

	task := createAdminTask()

	req := httptest.NewRequest(http.MethodGet, fullRootPath()+"/admin/tasks/"+task.UUID, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveAdminTasksRouter(req, false, false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Admin tasks feature is disabled.")
	suite.reg.AdminTask.AssertNotCalled(t, "Fetch", task.UUID)
}

func (suite *AdminTasksSuite) TestFetchNotAccessible() {
	t := suite.T()

	task := createAdminTask()

	req := httptest.NewRequest(http.MethodGet, fullRootPath()+"/admin/tasks/"+task.UUID, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveAdminTasksRouter(req, true, false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Neither the user nor account is allowed.")
	suite.reg.AdminTask.AssertNotCalled(t, "Fetch", task.UUID)
}
