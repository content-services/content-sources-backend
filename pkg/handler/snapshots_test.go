package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type SnapshotSuite struct {
	suite.Suite
	reg        *dao.MockDaoRegistry
	pulpClient *pulp_client.MockPulpClient
	tcMock     *client.MockTaskClient
}

func TestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(SnapshotSuite))
}
func (suite *SnapshotSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
	suite.pulpClient = pulp_client.NewMockPulpClient(suite.T())
	suite.tcMock = client.NewMockTaskClient(suite.T())
}

func (suite *SnapshotSuite) serveSnapshotsRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	sh := SnapshotHandler{
		DaoRegistry: *suite.reg.ToDaoRegistry(),
		TaskClient:  suite.tcMock,
	}

	RegisterSnapshotRoutes(pathPrefix, suite.reg.ToDaoRegistry(), &sh.TaskClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *SnapshotSuite) TestListSnapshotsByDate() {
	t := suite.T()
	repoUUID := "abcadaba"
	request := api.ListSnapshotByDateRequest{Date: time.Time{}, RepositoryUUIDS: []string{repoUUID}}
	response := api.ListSnapshotByDateResponse{Data: []api.SnapshotForDate{{RepositoryUUID: repoUUID}}}

	suite.reg.Snapshot.On("FetchSnapshotsByDateAndRepository", test.MockCtx(), test_handler.MockOrgId, request).Return(response, nil)

	body, err := json.Marshal(request)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/snapshots/for_date/", bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, _, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *SnapshotSuite) TestListSnapshotsByDateBadRequestError() {
	t := suite.T()
	RepositoryUUIDS := []string{}

	request := api.ListSnapshotByDateRequest{Date: time.Time{}, RepositoryUUIDS: RepositoryUUIDS}

	body, err := json.Marshal(request)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/snapshots/for_date/", bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, _, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *SnapshotSuite) TestListSnapshotsByDateExceedLimitError() {
	t := suite.T()
	RepositoryUUIDS := []string{}
	for i := 0; i < SnapshotByDateQueryLimit+1; i++ {
		RepositoryUUIDS = append(RepositoryUUIDS, seeds.RandomOrgId())
	}

	request := api.ListSnapshotByDateRequest{Date: time.Time{}, RepositoryUUIDS: RepositoryUUIDS}

	body, err := json.Marshal(request)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/snapshots/for_date/", bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, _, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusRequestEntityTooLarge, code)
}

func (suite *SnapshotSuite) TestListSnapshotsForTemplate() {
	t := suite.T()

	paginationData := api.PaginationData{Limit: 10, Offset: DefaultOffset}
	collection := createSnapshotCollection(1, 10, 0)
	templateUUID := "abcadaba"
	template := api.TemplateResponse{
		Name: "my template",
		UUID: templateUUID,
	}

	suite.reg.Template.On("Fetch", test.MockCtx(), test_handler.MockOrgId, templateUUID, false).Return(template, nil)
	suite.reg.Snapshot.On("ListByTemplate", test.MockCtx(), test_handler.MockOrgId, template, "", paginationData).Return(collection, int64(1), nil)

	path := fmt.Sprintf("%s/templates/%s/snapshots/?limit=%d", api.FullRootPath(), templateUUID, 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)

	response := api.SnapshotCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(1), response.Meta.Count)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, collection.Data[0].RepositoryPath, response.Data[0].RepositoryPath)
	assert.Equal(t, collection.Data[0].UUID, response.Data[0].UUID)
	assert.Equal(t, collection.Data[0].URL, response.Data[0].URL)
}

func (suite *SnapshotSuite) TestSnapshotList() {
	t := suite.T()

	paginationData := api.PaginationData{Limit: 10, Offset: DefaultOffset}
	collection := createSnapshotCollection(1, 10, 0)
	repoUUID := "abcadaba"
	suite.reg.Snapshot.WithContextMock().On("List", test.MockCtx(), repoUUID, paginationData, api.FilterData{}).Return(collection, int64(1), nil)

	suite.reg.Snapshot.On("List", test.MockCtx(), test_handler.MockOrgId, repoUUID, paginationData, api.FilterData{}).Return(collection, int64(1), nil)

	path := fmt.Sprintf("%s/repositories/%s/snapshots/?limit=%d", api.FullRootPath(), repoUUID, 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)

	response := api.SnapshotCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(1), response.Meta.Count)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, collection.Data[0].RepositoryPath, response.Data[0].RepositoryPath)
	assert.Equal(t, collection.Data[0].UUID, response.Data[0].UUID)
	assert.Equal(t, collection.Data[0].URL, response.Data[0].URL)
}

func (suite *SnapshotSuite) TestGetRepositoryConfigurationFile() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	snapUUID := uuid.NewString()
	repoConfigFile := "file"
	refererHeader := "anotherhost.example.com"

	suite.reg.Snapshot.WithContextMock().On("GetRepositoryConfigurationFile", test.MockCtx(), orgID, snapUUID, false).Return(repoConfigFile, nil).Once()

	path := fmt.Sprintf("%s/snapshots/%s/config.repo", api.FullRootPath(), snapUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("x-forwarded-host", refererHeader)

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)

	response := string(body)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, response, repoConfigFile)
}

func (suite *SnapshotSuite) TestDelete() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	collection := createSnapshotModels(4, repoUUID)
	snapUUID := collection[0].UUID
	var snap api.SnapshotResponse
	dao.SnapshotModelToApi(collection[0], &snap)

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask).Return([]string{}, nil)
	suite.reg.Snapshot.On("FetchForRepoConfigUUID", test.MockCtx(), repoUUID, false).Return(collection, nil)
	suite.reg.Snapshot.On("Fetch", test.MockCtx(), snapUUID).Return(snap, nil)
	suite.reg.Snapshot.On("SoftDelete", test.MockCtx(), snapUUID).Return(nil)
	mockDeleteSnapshotEnqueue(suite.tcMock, repoUUID, requestID, snapUUID).Return(uuid.New(), nil)

	path := fmt.Sprintf("%s/repositories/%s/snapshots/%s", api.FullRootPath(), repoUUID, snapUUID)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, _, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNoContent, code)
}

func (suite *SnapshotSuite) TestDeleteRepoNotFound() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	wrongRepoUUID := uuid.NewString()

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, wrongRepoUUID).Return(api.RepositoryResponse{}, errors.New("not found"))

	path := fmt.Sprintf("%s/repositories/%s/snapshots/%s", api.FullRootPath(), wrongRepoUUID, uuid.NewString())
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, _, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *SnapshotSuite) TestDeleteTaskFetchFailed() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	collection := createSnapshotModels(4, repoUUID)
	snapUUID := collection[0].UUID

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask).Return([]string{}, errors.New("test error"))

	path := fmt.Sprintf("%s/repositories/%s/snapshots/%s", api.FullRootPath(), repoUUID, snapUUID)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, _, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *SnapshotSuite) TestDeleteInProgress() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	collection := createSnapshotModels(4, repoUUID)
	snapUUID := collection[0].UUID

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask).Return([]string{uuid.NewString()}, nil)

	path := fmt.Sprintf("%s/repositories/%s/snapshots/%s", api.FullRootPath(), repoUUID, snapUUID)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.True(t, strings.Contains(string(body), "Delete is already in progress"))
}

func (suite *SnapshotSuite) TestDeleteRepoFetchFailed() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	collection := createSnapshotModels(4, repoUUID)
	snapUUID := collection[0].UUID

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask).Return([]string{}, nil)
	suite.reg.Snapshot.On("FetchForRepoConfigUUID", test.MockCtx(), repoUUID, false).Return(collection, errors.New("test error"))

	path := fmt.Sprintf("%s/repositories/%s/snapshots/%s", api.FullRootPath(), repoUUID, snapUUID)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, _, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *SnapshotSuite) TestDeleteAllSnapshotsError() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	collection := createSnapshotModels(1, repoUUID)
	snapUUID := collection[0].UUID

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask).Return([]string{}, nil)
	suite.reg.Snapshot.On("FetchForRepoConfigUUID", test.MockCtx(), repoUUID, false).Return(collection, nil)

	path := fmt.Sprintf("%s/repositories/%s/snapshots/%s", api.FullRootPath(), repoUUID, snapUUID)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.True(t, strings.Contains(string(body), "Can't delete all the snapshots in the repository"))
}

func (suite *SnapshotSuite) TestDeleteSnapNotInRepo() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	collection := createSnapshotModels(4, repoUUID)
	snapUUID := uuid.NewString()

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask).Return([]string{}, nil)
	suite.reg.Snapshot.On("FetchForRepoConfigUUID", test.MockCtx(), repoUUID, false).Return(collection, nil)

	path := fmt.Sprintf("%s/repositories/%s/snapshots/%s", api.FullRootPath(), repoUUID, snapUUID)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
	assert.True(t, strings.Contains(string(body), "snapshot with this UUID does not exist for the specified repository"))
}

func (suite *SnapshotSuite) TestDeleteSoftDeleteFailed() {
	t := suite.T()
	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	collection := createSnapshotModels(4, repoUUID)
	snapUUID := collection[0].UUID

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask).Return([]string{}, nil)
	suite.reg.Snapshot.On("FetchForRepoConfigUUID", test.MockCtx(), repoUUID, false).Return(collection, nil)
	suite.reg.Snapshot.On("SoftDelete", test.MockCtx(), snapUUID).Return(errors.New("test error"))

	path := fmt.Sprintf("%s/repositories/%s/snapshots/%s", api.FullRootPath(), repoUUID, snapUUID)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, _, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *SnapshotSuite) TestDeleteEnqueueFailed() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	collection := createSnapshotModels(4, repoUUID)
	snapUUID := collection[0].UUID

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask).Return([]string{}, nil)
	suite.reg.Snapshot.On("FetchForRepoConfigUUID", test.MockCtx(), repoUUID, false).Return(collection, nil)
	suite.reg.Snapshot.On("SoftDelete", test.MockCtx(), snapUUID).Return(nil)
	mockDeleteSnapshotEnqueue(suite.tcMock, repoUUID, requestID, snapUUID).Return(uuid.Nil, errors.New("test error"))
	suite.reg.Snapshot.On("ClearDeletedAt", test.MockCtx(), snapUUID).Return(nil)

	path := fmt.Sprintf("%s/repositories/%s/snapshots/%s", api.FullRootPath(), repoUUID, snapUUID)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.True(t, strings.Contains(string(body), "Error enqueueing task"))
}

func (suite *SnapshotSuite) TestDeleteClearDeletedAtFailed() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	collection := createSnapshotModels(4, repoUUID)
	snapUUID := collection[0].UUID

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask).Return([]string{}, nil)
	suite.reg.Snapshot.On("FetchForRepoConfigUUID", test.MockCtx(), repoUUID, false).Return(collection, nil)
	suite.reg.Snapshot.On("SoftDelete", test.MockCtx(), snapUUID).Return(nil)
	mockDeleteSnapshotEnqueue(suite.tcMock, repoUUID, requestID, snapUUID).Return(uuid.Nil, errors.New("test error"))
	suite.reg.Snapshot.On("ClearDeletedAt", test.MockCtx(), snapUUID).Return(errors.New("test error"))

	path := fmt.Sprintf("%s/repositories/%s/snapshots/%s", api.FullRootPath(), repoUUID, snapUUID)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.True(t, strings.Contains(string(body), "Error clearing deleted_at field"))
}

func (suite *SnapshotSuite) TestBulkDelete() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	collection := createSnapshotModels(4, repoUUID)
	snapUUIDs := []string{collection[1].UUID, collection[2].UUID}

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask).Return([]string{}, nil)
	suite.reg.Snapshot.On("FetchForRepoConfigUUID", test.MockCtx(), repoUUID, false).Return(collection, nil)
	suite.reg.Snapshot.On("BulkDelete", test.MockCtx(), snapUUIDs).Return([]error{})
	mockDeleteSnapshotEnqueue(suite.tcMock, repoUUID, requestID, snapUUIDs...).Return(uuid.New(), nil)

	body, err := json.Marshal(api.UUIDListRequest{UUIDs: snapUUIDs})
	assert.NoError(t, err)

	path := fmt.Sprintf("%s/repositories/%s/snapshots/bulk_delete/", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, _, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNoContent, code)
}

func (suite *SnapshotSuite) TestBulkDeleteNoUUIDs() {
	t := suite.T()

	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	var snapUUIDs []string
	body, err := json.Marshal(api.UUIDListRequest{UUIDs: snapUUIDs})
	assert.NoError(t, err)

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, repoUUID).Return(api.RepositoryResponse{}, nil)

	path := fmt.Sprintf("%s/repositories/%s/snapshots/bulk_delete/", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.True(t, strings.Contains(string(body), "Request body must contain at least 1 snapshot UUID to delete."))
}

func (suite *SnapshotSuite) TestBulkDeleteTooMany() {
	t := suite.T()

	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	var snapUUIDs []string
	for i := 0; i < 110; i++ {
		snapUUIDs = append(snapUUIDs, uuid.NewString())
	}
	body, err := json.Marshal(api.UUIDListRequest{UUIDs: snapUUIDs})
	assert.NoError(t, err)

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, repoUUID).Return(api.RepositoryResponse{}, nil)

	path := fmt.Sprintf("%s/repositories/%s/snapshots/bulk_delete/", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusRequestEntityTooLarge, code)
	assert.True(t, strings.Contains(string(body), fmt.Sprintf("Cannot delete more than %d snapshots at once.", BulkDeleteLimit)))
}

func (suite *SnapshotSuite) TestBulkDeleteHasErr() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	collection := createSnapshotModels(4, repoUUID)
	snapUUIDs := []string{collection[1].UUID, uuid.NewString(), collection[2].UUID}

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask).Return([]string{}, nil)
	suite.reg.Snapshot.On("FetchForRepoConfigUUID", test.MockCtx(), repoUUID, false).Return(collection, nil)

	body, err := json.Marshal(api.UUIDListRequest{UUIDs: snapUUIDs})
	assert.NoError(t, err)

	path := fmt.Sprintf("%s/repositories/%s/snapshots/bulk_delete/", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.True(t, strings.Contains(string(body), "\"detail\":\"snapshot with this UUID does not exist for the specified repository\""))
}

func (suite *SnapshotSuite) TestBulkDeleteFailedEnqueueAndClear() {
	t := suite.T()

	orgID := test_handler.MockOrgId
	requestID := uuid.NewString()
	repoUUID := uuid.NewString()
	collection := createSnapshotModels(4, repoUUID)
	snapUUIDs := []string{collection[1].UUID, collection[2].UUID}

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), orgID, repoUUID).Return(api.RepositoryResponse{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask).Return([]string{}, nil)
	suite.reg.Snapshot.On("FetchForRepoConfigUUID", test.MockCtx(), repoUUID, false).Return(collection, nil)
	suite.reg.Snapshot.On("BulkDelete", test.MockCtx(), snapUUIDs).Return([]error{})
	mockDeleteSnapshotEnqueue(suite.tcMock, repoUUID, requestID, snapUUIDs...).Return(uuid.Nil, errors.New("test error, failed enqueue"))
	suite.reg.Snapshot.On("ClearDeletedAt", test.MockCtx(), snapUUIDs[0]).Return(nil)
	suite.reg.Snapshot.On("ClearDeletedAt", test.MockCtx(), snapUUIDs[1]).Return(errors.New("test error, failed clear"))

	body, err := json.Marshal(api.UUIDListRequest{UUIDs: snapUUIDs})
	assert.NoError(t, err)

	path := fmt.Sprintf("%s/repositories/%s/snapshots/bulk_delete/", api.FullRootPath(), repoUUID)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set(config.HeaderRequestId, requestID)

	code, body, err := suite.serveSnapshotsRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.False(t, strings.Contains(string(body), "\"detail\":\"test error, failed enqueue\""))
	assert.True(t, strings.Contains(string(body), "\"detail\":\"test error, failed clear\""))
}

func mockDeleteSnapshotEnqueue(mock *client.MockTaskClient, repoUUID, requestID string, snapUUIDs ...string) *mock.Call {
	return mock.On("Enqueue", queue.Task{
		Typename:   config.DeleteSnapshotsTask,
		Payload:    payloads.DeleteSnapshotsPayload{RepoUUID: repoUUID, SnapshotsUUIDs: snapUUIDs},
		OrgId:      test_handler.MockOrgId,
		AccountId:  test_handler.MockAccountNumber,
		ObjectUUID: utils.Ptr(repoUUID),
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		RequestID:  requestID,
	})
}

func createSnapshotCollection(size, limit, offset int) api.SnapshotCollectionResponse {
	snaps := make([]api.SnapshotResponse, size)
	for i := 0; i < size; i++ {
		snap := api.SnapshotResponse{
			RepositoryPath: "distribution/path/",
			UUID:           uuid.NewString(),
			URL:            "http://pulp-content/pulp/content",
		}
		snaps[i] = snap
	}
	collection := api.SnapshotCollectionResponse{
		Data: snaps,
	}
	params := fmt.Sprintf("?offset=%d&limit=%d", offset, limit)
	setCollectionResponseMetadata(&collection, getTestContext(params), int64(size))
	return collection
}

func createSnapshotModels(size int, repoUUID string) []models.Snapshot {
	snaps := make([]models.Snapshot, size)
	for i := 0; i < size; i++ {
		snap := models.Snapshot{
			Base: models.Base{
				UUID: uuid.NewString(),
			},
			VersionHref:                 "/pulp/version",
			PublicationHref:             "/pulp/publication",
			DistributionPath:            fmt.Sprintf("/path/to/%v", uuid.NewString()),
			RepositoryConfigurationUUID: repoUUID,
			ContentCounts:               models.ContentCountsType{"rpm.package": int64(3), "rpm.advisory": int64(1)},
			AddedCounts:                 models.ContentCountsType{"rpm.package": int64(1), "rpm.advisory": int64(3)},
			RemovedCounts:               models.ContentCountsType{"rpm.package": int64(2), "rpm.advisory": int64(2)},
		}
		snaps[i] = snap
	}
	return snaps
}
