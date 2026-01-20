package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func createRepoRequest(name string, url string) api.RepositoryRequest {
	blank := ""
	account := test_handler.MockAccountNumber
	org := test_handler.MockOrgId
	return api.RepositoryRequest{
		UUID:      &blank,
		Name:      &name,
		URL:       &url,
		AccountID: &account,
		OrgID:     &org,
	}
}

func createRepoUpdateRequest(name string, url string, snapshot bool) api.RepositoryUpdateRequest {
	return api.RepositoryUpdateRequest{
		Name:     &name,
		URL:      &url,
		Snapshot: &snapshot,
	}
}

func createRepoCollection(size, limit, offset int) api.RepositoryCollectionResponse {
	repos := make([]api.RepositoryResponse, size)
	for i := 0; i < size; i++ {
		repo := api.RepositoryResponse{
			UUID:                         fmt.Sprintf("%d", i),
			Name:                         fmt.Sprintf("repo_%d", i),
			URL:                          fmt.Sprintf("http://repo-%d.com", i),
			DistributionVersions:         []string{config.El7},
			DistributionArch:             config.X8664,
			AccountID:                    test_handler.MockAccountNumber,
			OrgID:                        test_handler.MockOrgId,
			LastIntrospectionTime:        "2022-08-31 14:17:50.257623 -0400 EDT",
			LastIntrospectionSuccessTime: "2022-08-31 14:17:50.257623 -0400 EDT",
			LastIntrospectionUpdateTime:  "2022-08-31 14:17:50.257623 -0400 EDT",
			LastIntrospectionError:       "",
			Status:                       "Valid",
			GpgKey:                       "foo",
			MetadataVerification:         true,
			LastSnapshot: &api.SnapshotResponse{
				RepositoryPath: "distribution/path/",
				UUID:           uuid.NewString(),
				URL:            "http://pulp-content/pulp/content",
			},
		}
		repos[i] = repo
	}
	collection := api.RepositoryCollectionResponse{
		Data: repos,
	}
	params := fmt.Sprintf("?offset=%d&limit=%d", offset, limit)
	setCollectionResponseMetadata(&collection, getTestContext(params), int64(size))
	return collection
}

func (suite *ReposSuite) serveRepositoriesRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	rh := RepositoryHandler{
		DaoRegistry:          *suite.reg.ToDaoRegistry(),
		TaskClient:           suite.tcMock,
		FeatureServiceClient: suite.fsMock,
	}

	RegisterRepositoryRoutes(pathPrefix, suite.reg.ToDaoRegistry(), &rh.TaskClient, &rh.FeatureServiceClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func mockTaskClientEnqueueIntrospect(tcMock *client.MockTaskClient, expectedUrl string, repositoryUuid string) {
	tcMock.On("Enqueue", queue.Task{
		Typename:     payloads.Introspect,
		Payload:      payloads.IntrospectPayload{Url: expectedUrl, Force: true, Origin: utils.Ptr(config.OriginExternal)},
		Dependencies: nil,
		OrgId:        test_handler.MockOrgId,
		ObjectUUID:   &repositoryUuid,
		ObjectType:   utils.Ptr(config.ObjectTypeRepository),
	}).Return(nil, nil)
}

func mockTaskClientEnqueueSnapshot(repoSuite *ReposSuite, response *api.RepositoryResponse) {
	repoSuite.tcMock.On("Enqueue", queue.Task{
		Typename:   config.RepositorySnapshotTask,
		Payload:    payloads.SnapshotPayload{},
		OrgId:      response.OrgID,
		ObjectUUID: &response.RepositoryUUID,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
	}).Return(nil, nil)
	repoSuite.reg.RepositoryConfig.On(
		"UpdateLastSnapshotTask",
		test.MockCtx(),
		"00000000-0000-0000-0000-000000000000",
		response.OrgID,
		response.RepositoryUUID,
	).Return(nil)
	response.LastSnapshotTaskUUID = "00000000-0000-0000-0000-000000000000"
	repoSuite.tcMock.On("Enqueue", queue.Task{
		Typename:     config.UpdateLatestSnapshotTask,
		Payload:      tasks.UpdateLatestSnapshotPayload{RepositoryConfigUUID: response.UUID},
		Dependencies: []uuid.UUID{uuid.Nil},
		ObjectUUID:   &response.RepositoryUUID,
		ObjectType:   utils.Ptr(config.ObjectTypeRepository),
		OrgId:        response.OrgID,
	}).Return(nil, nil)
}

func mockTaskClientEnqueueUpdate(repoSuite *ReposSuite, response api.RepositoryResponse) {
	repoSuite.tcMock.On("Enqueue", queue.Task{
		Typename:   config.UpdateRepositoryTask,
		Payload:    tasks.UpdateRepositoryPayload{RepositoryConfigUUID: response.UUID},
		OrgId:      response.OrgID,
		ObjectUUID: &response.RepositoryUUID,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		Priority:   1,
	}).Return(nil, nil)
}

func mockTaskClientEnqueueAddUploads(repoSuite *ReposSuite, repo api.RepositoryResponse, request api.AddUploadsRequest) {
	repoSuite.tcMock.On("Enqueue", queue.Task{
		Typename: config.AddUploadsTask,
		Payload: tasks.AddUploadsPayload{
			RepositoryConfigUUID: repo.UUID,
			Artifacts:            request.Artifacts,
			Uploads:              request.Uploads,
		},
		OrgId:      repo.OrgID,
		ObjectUUID: &repo.RepositoryUUID,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		Priority:   0,
	}).Return(nil, nil)
	repoSuite.reg.RepositoryConfig.On(
		"UpdateLastSnapshotTask",
		test.MockCtx(),
		"00000000-0000-0000-0000-000000000000",
		repo.OrgID,
		repo.RepositoryUUID,
	).Return(nil)
	repo.LastSnapshotTaskUUID = "00000000-0000-0000-0000-000000000000"
	repoSuite.tcMock.On("Enqueue", queue.Task{
		Typename:     config.UpdateLatestSnapshotTask,
		Payload:      tasks.UpdateLatestSnapshotPayload{RepositoryConfigUUID: repo.UUID},
		Dependencies: []uuid.UUID{dao.UuidifyString(repo.LastSnapshotTaskUUID)},
		ObjectUUID:   &repo.RepositoryUUID,
		ObjectType:   utils.Ptr(config.ObjectTypeRepository),
		OrgId:        repo.OrgID,
	}).Return(nil, nil)
}

func mockSnapshotDeleteEvent(tcMock *client.MockTaskClient, repoConfigUUID string) {
	tcMock.On("Enqueue", queue.Task{
		Typename:     config.DeleteRepositorySnapshotsTask,
		Payload:      tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repoConfigUUID},
		Dependencies: nil,
		OrgId:        test_handler.MockOrgId,
		ObjectUUID:   &repoConfigUUID,
		ObjectType:   utils.Ptr(config.ObjectTypeRepository),
	}).Return(nil, nil)
}

type ReposSuite struct {
	suite.Suite
	reg    *dao.MockDaoRegistry
	tcMock *client.MockTaskClient
	pcMock *pulp_client.MockPulpGlobalClient
	fsMock *feature_service_client.MockFeatureServiceClient
}

func (suite *ReposSuite) TestSimple() {
	t := suite.T()

	collection := createRepoCollection(1, 10, 0)
	paginationData := api.PaginationData{Limit: 10, Offset: DefaultOffset}
	suite.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.FilterData{}).Return(collection, int64(1), nil)

	path := fmt.Sprintf("%s/repositories/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	response := api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(1), response.Meta.Count)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, collection.Data[0].Name, response.Data[0].Name)
	assert.Equal(t, collection.Data[0].URL, response.Data[0].URL)
	assert.Equal(t, collection.Data[0].AccountID, response.Data[0].AccountID)
	assert.Equal(t, collection.Data[0].DistributionVersions, response.Data[0].DistributionVersions)
	assert.Equal(t, collection.Data[0].DistributionArch, response.Data[0].DistributionArch)
	assert.Equal(t, collection.Data[0].LastIntrospectionUpdateTime, response.Data[0].LastIntrospectionUpdateTime)
	assert.Equal(t, collection.Data[0].LastIntrospectionTime, response.Data[0].LastIntrospectionTime)
	assert.Equal(t, collection.Data[0].LastIntrospectionSuccessTime, response.Data[0].LastIntrospectionSuccessTime)
	assert.Equal(t, collection.Data[0].LastIntrospectionError, response.Data[0].LastIntrospectionError)
	assert.Equal(t, collection.Data[0].GpgKey, response.Data[0].GpgKey)
	assert.Equal(t, collection.Data[0].MetadataVerification, response.Data[0].MetadataVerification)
	assert.Equal(t, collection.Data[0].LastSnapshot.URL, response.Data[0].LastSnapshot.URL)
	assert.Equal(t, collection.Data[0].LastSnapshot.UUID, response.Data[0].LastSnapshot.UUID)
}

func (suite *ReposSuite) TestListNoRepositories() {
	t := suite.T()

	collection := api.RepositoryCollectionResponse{}
	paginationData := api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset}
	suite.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.FilterData{}).Return(collection, int64(0), nil)

	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/repositories/", nil)
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
	assert.Equal(t, api.FullRootPath()+"/repositories/?limit=100&offset=0", response.Links.Last)
	assert.Equal(t, api.FullRootPath()+"/repositories/?limit=100&offset=0", response.Links.First)
}

func (suite *ReposSuite) TestListPagedExtraRemaining() {
	t := suite.T()

	collection := api.RepositoryCollectionResponse{}
	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 100}

	suite.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData1, api.FilterData{}).Return(collection, int64(102), nil).Once()
	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData2, api.FilterData{}).Return(collection, int64(102), nil).Once()

	path := fmt.Sprintf("%s/repositories/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.RepositoryCollectionResponse{}
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

	response = api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func (suite *ReposSuite) TestListWithFilters() {
	t := suite.T()
	collection := api.RepositoryCollectionResponse{}

	suite.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 100}, api.FilterData{ContentType: "rpm", Origin: "external"}).Return(collection, int64(100), nil)

	path := fmt.Sprintf("%s/repositories/?origin=%v&content_type=%v", api.FullRootPath(), "external", "rpm")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *ReposSuite) TestListWithExtendedReleaseFilters() {
	t := suite.T()
	collection := api.RepositoryCollectionResponse{}

	suite.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId,
		api.PaginationData{Limit: 100},
		api.FilterData{ExtendedRelease: "eus", ExtendedReleaseVersion: "9.4"}).
		Return(collection, int64(10), nil)

	path := fmt.Sprintf("%s/repositories/?extended_release=%v&extended_release_version=%v", api.FullRootPath(), "eus", "9.4")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *ReposSuite) TestListPagedNoRemaining() {
	t := suite.T()

	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 90}

	collection := api.RepositoryCollectionResponse{}
	suite.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData1, api.FilterData{}).Return(collection, int64(100), nil)
	suite.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData2, api.FilterData{}).Return(collection, int64(100), nil)

	path := fmt.Sprintf("%s/repositories/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.RepositoryCollectionResponse{}
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

	response = api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func (suite *ReposSuite) TestListDaoError() {
	t := suite.T()

	daoError := ce.DaoError{
		Message: "Column doesn't exist",
	}
	paginationData := api.PaginationData{Limit: DefaultLimit}

	suite.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.FilterData{}).
		Return(api.RepositoryCollectionResponse{}, int64(0), &daoError)

	path := fmt.Sprintf("%s/repositories/", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (suite *ReposSuite) TestFetch() {
	t := suite.T()

	uuid := "abcadaba"
	repo := api.RepositoryResponse{
		Name: "my repo",
		URL:  "https://example.com",
		UUID: uuid,
	}

	suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(repo, nil)
	suite.fsMock.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).Return([]string{}, nil)

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/repositories/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	var response api.RepositoryResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.NotEmpty(t, response.UUID)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *ReposSuite) TestFetchNotFound() {
	t := suite.T()

	uuid := "abcadaba"
	repo := api.RepositoryResponse{
		Name: "my repo",
		URL:  "https://example.com",
		UUID: uuid,
	}

	daoError := ce.DaoError{
		NotFound: true,
		Message:  "Not found",
	}
	suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(api.RepositoryResponse{}, &daoError)
	suite.fsMock.On("GetEntitledFeatures", test.MockCtx(), test_handler.MockOrgId).Return([]string{}, nil)

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/repositories/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, _ := suite.serveRepositoriesRouter(req)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *ReposSuite) TestCreate() {
	t := suite.T()

	config.Load()
	config.Get().Features.Snapshots.Enabled = true
	config.Get().Features.Snapshots.Accounts = &[]string{test_handler.MockAccountNumber}
	defer resetFeatures()

	config.Get().Clients.Pulp.Server = "some-server-address" // This ensures that PulpConfigured returns true
	repoUuid := "repoUuid"
	expected := api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		RepositoryUUID: repoUuid,
		Snapshot:       true,
		Origin:         config.OriginExternal,
	}

	repo := createRepoRequest("my repo", "https://example.com")
	repo.Snapshot = utils.Ptr(true)
	repo.ModuleHotfixes = utils.Ptr(true)
	repo.FillDefaults(&test_handler.MockAccountNumber, &test_handler.MockOrgId)

	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), test_handler.MockOrgId).Return("MyDomain", nil)
	suite.reg.RepositoryConfig.On("Create", test.MockCtx(), repo).Return(expected, nil)

	mockTaskClientEnqueueSnapshot(suite, &expected)
	mockTaskClientEnqueueIntrospect(suite.tcMock, expected.URL, repoUuid)

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	var response api.RepositoryResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.NotEmpty(t, response.Name)
	assert.Equal(t, http.StatusCreated, code)
}

func resetFeatures() {
	config.Get().Features.Snapshots.Enabled = true
	config.Get().Features.AdminTasks.Enabled = true
	config.Get().Features.Snapshots.Accounts = nil
	config.Get().Features.Snapshots.Users = nil
}

func (suite *ReposSuite) TestCreateSnapshotNotAllowed() {
	config.Get().Features.Snapshots.Enabled = false
	defer resetFeatures()

	t := suite.T()
	expected := api.RepositoryResponse{
		Name: "my repo",
		URL:  "https://example.com",
	}

	repo := createRepoRequest("my repo", "https://example.com")
	repo.FillDefaults(&test_handler.MockAccountNumber, &test_handler.MockOrgId)
	repo.Snapshot = utils.Ptr(true)

	suite.reg.RepositoryConfig.On("Create", test.MockCtx(), repo).Return(expected, nil)

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, 400, code)

	var response ce.ErrorResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)

	assert.Equal(t, "Snapshotting Feature is disabled.", response.Errors[0].Title)
}

func (suite *ReposSuite) TestCreateAlreadyExists() {
	t := suite.T()

	repo := createRepoRequest("my repo", "https://example.com")
	repo.FillDefaults(&test_handler.MockAccountNumber, &test_handler.MockOrgId)
	daoError := ce.DaoError{
		BadValidation: true,
		Message:       "Already exists",
	}
	suite.reg.RepositoryConfig.On("Create", test.MockCtx(), repo).Return(api.RepositoryResponse{}, &daoError)

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}
	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	var response api.RepositoryResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Empty(t, response.UUID)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *ReposSuite) TestBulkCreate() {
	resetFeatures()
	t := suite.T()
	config.Get().Features.Snapshots.Enabled = true
	config.Get().Features.Snapshots.Accounts = &[]string{test_handler.MockAccountNumber}
	config.Get().Clients.Pulp.Server = "some-server-address" // This ensures that PulpConfigured returns true
	repo1 := createRepoRequest("repo_1", "https://example1.com")
	repo1.FillDefaults(&test_handler.MockAccountNumber, &test_handler.MockOrgId)
	repo1.Snapshot = utils.Ptr(true)
	repoUuid1 := "repoUuid1"

	repo2 := createRepoRequest("repo_2", "https://example2.com")
	repo2.ModuleHotfixes = utils.Ptr(true)
	repo2.FillDefaults(&test_handler.MockAccountNumber, &test_handler.MockOrgId)
	repoUuid2 := "repoUuid2"

	repos := []api.RepositoryRequest{
		repo1,
		repo2,
	}

	expected := []api.RepositoryResponse{
		{
			Name:           "repo_1",
			URL:            "https://example1.com",
			RepositoryUUID: repoUuid1,
			Snapshot:       true,
			Origin:         config.OriginExternal,
		},
		{
			Name:           "repo_2",
			URL:            "https://example2.com",
			RepositoryUUID: repoUuid2,
			ModuleHotfixes: true,
			Origin:         config.OriginExternal,
		},
	}

	suite.reg.RepositoryConfig.On("BulkCreate", test.MockCtx(), repos).Return(expected, []error{})
	suite.reg.Domain.On("FetchOrCreateDomain", test.MockCtx(), test_handler.MockOrgId).Return("MyDomain", nil)

	mockTaskClientEnqueueSnapshot(suite, &expected[0])
	mockTaskClientEnqueueIntrospect(suite.tcMock, expected[0].URL, repoUuid1)
	mockTaskClientEnqueueIntrospect(suite.tcMock, expected[1].URL, repoUuid2)

	body, err := json.Marshal(repos)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/bulk_create/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	var response []api.RepositoryResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.NotEmpty(t, response[0].Name)
	assert.Equal(t, http.StatusCreated, code)
}

func (suite *ReposSuite) TestBulkCreateOneFails() {
	t := suite.T()

	repo1 := createRepoRequest("repo_1", "https://example1.com")
	repo1.FillDefaults(&test_handler.MockAccountNumber, &test_handler.MockOrgId)

	repo2 := createRepoRequest("repo_2", "")
	repo2.FillDefaults(&test_handler.MockAccountNumber, &test_handler.MockOrgId)

	repos := []api.RepositoryRequest{
		repo1,
		repo2,
	}

	expected := []error{
		nil,
		&ce.DaoError{
			BadValidation: true,
			Message:       "Bad validation",
		},
	}

	suite.reg.RepositoryConfig.On("BulkCreate", test.MockCtx(), repos).Return([]api.RepositoryResponse{}, expected)

	body, err := json.Marshal(repos)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/bulk_create/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	var response ce.ErrorResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, "", response.Errors[0].Detail)
	assert.NotEmpty(t, response.Errors[1].Detail)
	assert.NotEqual(t, http.StatusOK, response.Errors[1].Status)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *ReposSuite) TestBulkCreateTooMany() {
	t := suite.T()

	repos := make([]api.RepositoryRequest, BulkCreateLimit+1)
	for i := 0; i < BulkCreateLimit+1; i++ {
		repos[i] = createRepoRequest("repo"+strconv.Itoa(i), "example"+strconv.Itoa(i)+".com")
		repos[i].FillDefaults(&test_handler.MockAccountNumber, &test_handler.MockOrgId)
	}

	body, err := json.Marshal(repos)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/bulk_create/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusRequestEntityTooLarge, code)
}

func (suite *ReposSuite) TestBulkExport() {
	t := suite.T()

	exportRequest := api.RepositoryExportRequest{
		RepositoryUuids: []string{"uuid1", "uuid2"},
	}
	repos := []api.RepositoryExportResponse{
		{
			Name:                 "repo1",
			URL:                  "http://example1.com",
			Origin:               "external",
			DistributionVersions: []string{"8"},
			DistributionArch:     "x86_64",
			GpgKey:               "",
			MetadataVerification: false,
			ModuleHotfixes:       false,
			Snapshot:             false,
		},
		{
			Name:                 "repo2",
			URL:                  "http://example2.com",
			Origin:               "external",
			DistributionVersions: []string{"8"},
			DistributionArch:     "x86_64",
			GpgKey:               "",
			MetadataVerification: false,
			ModuleHotfixes:       false,
			Snapshot:             false,
		},
	}

	suite.reg.RepositoryConfig.WithContextMock().On("BulkExport", test.MockCtx(), test_handler.MockOrgId, exportRequest).Return(repos, nil)

	body, err := json.Marshal(exportRequest)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/bulk_export/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	var response []api.RepositoryExportResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, response[0].URL, repos[0].URL)
	assert.Equal(t, response[1].URL, repos[1].URL)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *ReposSuite) TestBulkImport() {
	resetFeatures()
	t := suite.T()
	config.Get().Features.Snapshots.Enabled = true
	config.Get().Features.Snapshots.Accounts = &[]string{test_handler.MockAccountNumber}
	config.Get().Clients.Pulp.Server = "some-server-address" // This ensures that PulpConfigured returns true
	repo1 := createRepoRequest("repo_1", "https://example1.com")
	repo1.FillDefaults(&test_handler.MockAccountNumber, &test_handler.MockOrgId)
	repo1.Snapshot = utils.Ptr(true)
	repoUuid1 := "repoUuid1"

	repo2 := createRepoRequest("repo_2", "https://example2.com")
	repo2.ModuleHotfixes = utils.Ptr(true)
	repo2.FillDefaults(&test_handler.MockAccountNumber, &test_handler.MockOrgId)
	repoUuid2 := "repoUuid2"

	repos := []api.RepositoryRequest{
		repo1,
		repo2,
	}

	expected := []api.RepositoryImportResponse{
		{
			RepositoryResponse: api.RepositoryResponse{
				Name:           "repo_1",
				URL:            "https://example1.com",
				RepositoryUUID: repoUuid1,
				Snapshot:       true,
				Origin:         config.OriginExternal,
			},
			Warnings: nil,
		},
		{
			RepositoryResponse: api.RepositoryResponse{
				Name:           "repo_2",
				URL:            "https://example2.com",
				RepositoryUUID: repoUuid2,
				Snapshot:       false,
				Origin:         config.OriginExternal,
			},
			Warnings: nil,
		},
	}

	suite.reg.RepositoryConfig.On("BulkImport", test.MockCtx(), repos).Return(expected, []error{})

	mockTaskClientEnqueueSnapshot(suite, &expected[0].RepositoryResponse)
	mockTaskClientEnqueueIntrospect(suite.tcMock, expected[0].URL, repoUuid1)
	mockTaskClientEnqueueIntrospect(suite.tcMock, expected[1].URL, repoUuid2)

	body, err := json.Marshal(repos)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/bulk_import/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	var response []api.RepositoryImportResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, response[0].URL, expected[0].URL)
	assert.Equal(t, response[1].URL, expected[1].URL)
	assert.Equal(t, http.StatusCreated, code)
}

func (suite *ReposSuite) TestDelete() {
	t := suite.T()
	uuid := "valid-uuid"

	suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: uuid,
	}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, uuid, config.RepositorySnapshotTask, config.IntrospectTask).Return([]string{}, nil)
	suite.reg.RepositoryConfig.On("SoftDelete", test.MockCtx(), test_handler.MockOrgId, uuid).Return(nil)
	mockSnapshotDeleteEvent(suite.tcMock, uuid)

	req := httptest.NewRequest(http.MethodDelete, api.FullRootPath()+"/repositories/"+uuid, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNoContent, code)
}

func (suite *ReposSuite) TestDeleteNotFound() {
	t := suite.T()

	uuid := "invalid-uuid"
	daoError := ce.DaoError{
		NotFound: true,
	}

	suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: uuid,
	}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, uuid, config.RepositorySnapshotTask, config.IntrospectTask).Return([]string{}, nil)
	suite.reg.RepositoryConfig.On("SoftDelete", test.MockCtx(), test_handler.MockOrgId, uuid).Return(&daoError)

	req := httptest.NewRequest(http.MethodDelete, api.FullRootPath()+"/repositories/"+uuid, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *ReposSuite) TestDeleteSnapshotInProgress() {
	t := suite.T()
	uuid := "inprogress-uuid"

	suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: uuid,
	}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, uuid, config.RepositorySnapshotTask, config.IntrospectTask).Return([]string{"task-uuid"}, nil)
	suite.tcMock.On("Cancel", test.MockCtx(), "task-uuid").Return(nil)
	suite.reg.RepositoryConfig.On("SoftDelete", test.MockCtx(), test_handler.MockOrgId, uuid).Return(nil)
	mockSnapshotDeleteEvent(suite.tcMock, uuid)

	req := httptest.NewRequest(http.MethodDelete, api.FullRootPath()+"/repositories/"+uuid, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNoContent, code)
}

func (suite *ReposSuite) TestBulkDelete() {
	t := suite.T()
	uuids := []string{"uuid-1", "uuid-2"}

	for i := range uuids {
		suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuids[i]).Return(api.RepositoryResponse{
			Name:           fmt.Sprintf("my repo %d", i),
			URL:            fmt.Sprintf("https://example.com/%d", i),
			UUID:           uuids[i],
			RepositoryUUID: uuids[i],
		}, nil)
		suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, uuids[i], config.RepositorySnapshotTask, config.IntrospectTask).Return([]string{}, nil)
		mockSnapshotDeleteEvent(suite.tcMock, uuids[i])
	}

	suite.reg.RepositoryConfig.On("BulkDelete", test.MockCtx(), test_handler.MockOrgId, uuids).Return([]error{})

	body, err := json.Marshal(api.UUIDListRequest{UUIDs: uuids})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/bulk_delete/", bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNoContent, code)
}

func (suite *ReposSuite) TestBulkDeleteNoUUIDs() {
	t := suite.T()

	body, err := json.Marshal(api.UUIDListRequest{})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/bulk_delete/", bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Request body must contain at least 1 repository UUID to delete.")

	req = httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/bulk_delete/", nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, _, err = suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Request body must contain at least 1 repository UUID to delete.")
}

func (suite *ReposSuite) TestBulkDeleteNotFound() {
	t := suite.T()
	uuids := []string{"uuid-1", "uuid-2"}
	daoError := ce.DaoError{
		NotFound: true,
	}

	suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuids[0]).Return(api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com/%d",
		UUID:           uuids[0],
		RepositoryUUID: uuids[0],
	}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, uuids[0], config.RepositorySnapshotTask, config.IntrospectTask).Return([]string{}, nil)
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuids[0]).Return(api.RepositoryResponse{}, nil)
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuids[1]).Return(api.RepositoryResponse{}, &daoError)

	body, err := json.Marshal(api.UUIDListRequest{UUIDs: uuids})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/bulk_delete/", bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)

	var response ce.ErrorResponse
	err = json.Unmarshal(body, &response)
	assert.NoError(t, err)

	assert.Equal(t, "", response.Errors[0].Detail)
	assert.Equal(t, http.StatusNotFound, response.Errors[1].Status)
}

func (suite *ReposSuite) TestBulkDeleteSnapshotInProgress() {
	t := suite.T()
	uuids := []string{"inprogress-uuid", "uuid-1"}

	for i := range uuids {
		suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuids[i]).Return(api.RepositoryResponse{
			Name:           fmt.Sprintf("my repo %d", i),
			URL:            fmt.Sprintf("https://example.com/%d", i),
			UUID:           uuids[i],
			RepositoryUUID: uuids[i],
		}, nil)
	}
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, uuids[0], config.RepositorySnapshotTask, config.IntrospectTask).Return([]string{"task-uuid"}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, uuids[0], config.RepositorySnapshotTask, config.IntrospectTask).Return([]string{}, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, uuids[1], config.RepositorySnapshotTask, config.IntrospectTask).Return([]string{}, nil)

	suite.tcMock.On("Cancel", test.MockCtx(), "task-uuid").Return(nil)
	suite.reg.RepositoryConfig.On("BulkDelete", test.MockCtx(), test_handler.MockOrgId, uuids).Return([]error{})
	mockSnapshotDeleteEvent(suite.tcMock, uuids[0])
	mockSnapshotDeleteEvent(suite.tcMock, uuids[1])

	body, err := json.Marshal(api.UUIDListRequest{UUIDs: uuids})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/bulk_delete/", bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNoContent, code)
}

func (suite *ReposSuite) TestBulkDeleteTooMany() {
	t := suite.T()

	uuids := make([]string, BulkDeleteLimit+1)
	for i := 0; i < len(uuids); i++ {
		uuids[i] = fmt.Sprintf("uuid-%d", i)
	}

	body, err := json.Marshal(api.UUIDListRequest{UUIDs: uuids})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/bulk_delete/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusRequestEntityTooLarge, code)
}

func (suite *ReposSuite) TestFullUpdate() {
	t := suite.T()

	uuid := "someuuid"
	repoUuid := "repoUuid"
	request := createRepoUpdateRequest("Some Name", "https://example.com", false)
	expected := createRepoUpdateRequest(*request.Name, *request.URL, *request.Snapshot)
	expected.FillDefaults()

	resp := api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: repoUuid,
		OrgID:          test_handler.MockOrgId,
	}

	suite.reg.RepositoryConfig.WithContextMock().On("Update", test.MockCtx(), test_handler.MockOrgId, uuid, expected).Return(false, nil)
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(resp, nil)
	suite.reg.RepositoryConfig.On("Update", test.MockCtx(), test_handler.MockOrgId, uuid, expected).Return(false, nil)
	mockTaskClientEnqueueUpdate(suite, resp)

	body, err := json.Marshal(request)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPut, api.FullRootPath()+"/repositories/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *ReposSuite) TestPartialUpdateUrlChange() {
	t := suite.T()
	config.Get().Clients.Pulp.Server = "some-server-address" // This ensures that PulpConfigured returns true
	config.Get().Features.Snapshots.Enabled = true
	config.Get().Features.Snapshots.Accounts = &[]string{test_handler.MockAccountNumber}
	defer resetFeatures()

	repoConfigUuid := "RepoConfigUuid"
	repoUuid := "RepoUuid"
	request := createRepoUpdateRequest("Some Name", "http://someurl.com", true)
	expected := createRepoUpdateRequest(*request.Name, *request.URL, *request.Snapshot)
	repoConfig := api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           repoConfigUuid,
		RepositoryUUID: repoUuid,
		Snapshot:       true,
		OrgID:          test_handler.MockOrgId,
		Origin:         config.OriginExternal,
	}

	suite.reg.RepositoryConfig.WithContextMock().On("Update", test.MockCtx(), test_handler.MockOrgId, repoConfigUuid, expected).Return(true, nil)
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, repoConfigUuid).Return(repoConfig, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, repoUuid, config.RepositorySnapshotTask, config.IntrospectTask).Return([]string{}, nil)

	mockTaskClientEnqueueUpdate(suite, repoConfig)
	mockTaskClientEnqueueSnapshot(suite, &repoConfig)
	mockTaskClientEnqueueIntrospect(suite.tcMock, "https://example.com", repoUuid)
	body, err := json.Marshal(request)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPatch, api.FullRootPath()+"/repositories/"+repoConfigUuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *ReposSuite) TestPartialUpdate() {
	t := suite.T()

	uuid := "someuuid"
	repoUuid := "repoUuid"
	request := createRepoUpdateRequest("Some Name", "https://example.com", false)
	expected := createRepoUpdateRequest(*request.Name, *request.URL, *request.Snapshot)
	resp := api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: repoUuid,
		Snapshot:       false,
		OrgID:          test_handler.MockOrgId,
	}

	suite.reg.RepositoryConfig.WithContextMock().On("Update", test.MockCtx(), test_handler.MockOrgId, uuid, expected).Return(false, nil)
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(resp, nil)
	mockTaskClientEnqueueUpdate(suite, resp)

	body, err := json.Marshal(request)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPatch, api.FullRootPath()+"/repositories/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *ReposSuite) TestPartialUpdateSnapshottingChangedToEnabled() {
	t := suite.T()
	config.Get().Clients.Pulp.Server = "some-server-address" // This ensures that PulpConfigured returns true
	config.Get().Features.Snapshots.Enabled = true
	config.Get().Features.Snapshots.Accounts = &[]string{test_handler.MockAccountNumber}
	defer resetFeatures()

	repoConfigUuid := "RepoConfigUuid"
	repoUuid := "RepoUuid"
	request := createRepoUpdateRequest("my repo", "https://example.com", true)
	expected := createRepoUpdateRequest(*request.Name, *request.URL, *request.Snapshot)
	repoConfig := api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           repoConfigUuid,
		RepositoryUUID: repoUuid,
		Snapshot:       false,
		OrgID:          test_handler.MockOrgId,
		Origin:         config.OriginExternal,
	}
	updatedRepoConfig := repoConfig
	updatedRepoConfig.Snapshot = true

	suite.reg.RepositoryConfig.WithContextMock().On("Update", test.MockCtx(), test_handler.MockOrgId, repoConfigUuid, expected).Return(true, nil)
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, repoConfigUuid).Once().Return(repoConfig, nil)
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, repoConfigUuid).Once().Return(updatedRepoConfig, nil)
	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, repoUuid, config.RepositorySnapshotTask, config.IntrospectTask).Return([]string{}, nil)

	mockTaskClientEnqueueUpdate(suite, updatedRepoConfig)
	mockTaskClientEnqueueSnapshot(suite, &updatedRepoConfig)
	mockTaskClientEnqueueIntrospect(suite.tcMock, "https://example.com", repoUuid)
	body, err := json.Marshal(request)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPatch, api.FullRootPath()+"/repositories/"+repoConfigUuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *ReposSuite) TestIntrospectRepository() {
	t := suite.T()

	t.Setenv("OPTIONS_INTROSPECT_API_TIME_LIMIT_SEC", "0")
	config.Load()

	repoConfigUUID := "abcadaba"
	repoUuid := "repoUuid"
	intReq := api.RepositoryIntrospectRequest{ResetCount: true}
	repoResp := api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           repoConfigUUID,
		RepositoryUUID: repoUuid,
		Origin:         config.OriginExternal,
	}
	repoUpdate := dao.RepositoryUpdate{UUID: "12345", FailedIntrospectionsCount: utils.Ptr(0), LastIntrospectionStatus: utils.Ptr("Pending")}
	now := time.Now()
	repo := dao.Repository{UUID: "12345", LastIntrospectionTime: &now}
	expectedTaskInfo := api.TaskInfoResponse{OrgId: test_handler.MockOrgId}

	mockTaskClientEnqueueIntrospect(suite.tcMock, "https://example.com", repoUuid)

	// Fetch will filter the request by Org ID before updating
	suite.reg.Repository.On("Update", test.MockCtx(), repoUpdate).Return(nil).NotBefore(
		suite.reg.Repository.On("FetchForUrl", test.MockCtx(), repoResp.URL, &repoResp.Origin).Return(repo, nil).NotBefore(
			suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, repoConfigUUID).Return(repoResp, nil),
		),
	)
	suite.reg.TaskInfo.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid.Nil.String()).Return(expectedTaskInfo, nil)

	body, err := json.Marshal(intReq)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/"+repoConfigUUID+"/introspect/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	var actualTaskInfo api.TaskInfoResponse
	err = json.Unmarshal(body, &actualTaskInfo)
	assert.Nil(t, err)

	assert.Equal(t, actualTaskInfo, expectedTaskInfo)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *ReposSuite) TestIntrospectRepositoryFailedLimit() {
	t := suite.T()
	intReq := api.RepositoryIntrospectRequest{}
	repo := dao.Repository{UUID: "12345", FailedIntrospectionsCount: 21}
	repoResp := api.RepositoryResponse{
		Name: "my repo",
		URL:  "https://example.com",
		UUID: "someuuid",
	}

	// Fetch will filter the request by Org ID before updating
	suite.reg.Repository.On("FetchForUrl", test.MockCtx(), repoResp.URL, &repoResp.Origin).Return(repo, nil).NotBefore(
		suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, repoResp.UUID).Return(repoResp, nil),
	)

	body, err := json.Marshal(intReq)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/"+repoResp.UUID+"/introspect/?reset_count=true",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *ReposSuite) TestCreateSnapshot() {
	t := suite.T()
	config.Load()
	config.Get().Features.Snapshots.Enabled = true
	config.Get().Features.Snapshots.Accounts = &[]string{test_handler.MockAccountNumber}
	defer resetFeatures()

	repoConfigUUID := "abcadaba"
	repoUuid := "repoUuid"
	repoResp := api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           repoConfigUUID,
		RepositoryUUID: repoUuid,
		Snapshot:       true,
	}

	repoUpdate := dao.RepositoryUpdate{UUID: repoUuid, LastIntrospectionStatus: utils.Ptr(config.StatusPending)}
	repo := dao.Repository{UUID: repoUuid}
	expectedTaskInfo := api.TaskInfoResponse{OrgId: test_handler.MockOrgId}

	mockTaskClientEnqueueSnapshot(suite, &repoResp)

	// Fetch will filter the request by Org ID before updating
	suite.reg.Repository.On("Update", test.MockCtx(), repoUpdate).Return(nil).NotBefore(
		suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, repo.UUID, config.RepositorySnapshotTask).Return([]string{}, nil).
			NotBefore(suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, repoConfigUUID).Return(repoResp, nil)),
	)

	suite.reg.TaskInfo.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid.Nil.String()).Return(expectedTaskInfo, nil)

	body, err := json.Marshal("")
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/"+repoConfigUUID+"/snapshot/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	var actualTaskInfo api.TaskInfoResponse
	err = json.Unmarshal(body, &actualTaskInfo)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusOK, code)
}

func (suite *ReposSuite) TestAddUploads() {
	t := suite.T()

	repoUuid := "repoUuid"
	configUUID := "configUUID"
	repo := api.RepositoryResponse{
		UUID:           configUUID,
		OrgID:          test_handler.MockOrgId,
		Name:           "my repo",
		URL:            "",
		RepositoryUUID: repoUuid,
		Snapshot:       true,
		Origin:         config.OriginUpload,
	}
	uploads := api.AddUploadsRequest{
		Uploads: []api.Upload{{
			Href:   "foo",
			Sha256: "foo1",
		}},
		Artifacts: []api.Artifact{{
			Href:   "foo2",
			Sha256: "foo3",
		}},
	}

	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), repo.OrgID, repo.UUID).Return(repo, nil)

	suite.reg.TaskInfo.On("Fetch", test.MockCtx(), repo.OrgID, uuid.Nil.String()).Return(api.TaskInfoResponse{}, nil)

	mockTaskClientEnqueueAddUploads(suite, repo, uploads)

	body, err := json.Marshal(uploads)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/"+repo.UUID+"/add_uploads/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	var response api.TaskInfoResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusCreated, code)
}

func (suite *ReposSuite) TestCreateSnapshotError() {
	t := suite.T()
	config.Load()
	config.Get().Features.Snapshots.Enabled = true
	config.Get().Features.Snapshots.Accounts = &[]string{test_handler.MockAccountNumber}
	defer resetFeatures()
	uuid := "abcadaba"
	repoUuid := "repoUuid"
	repoResp := api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: repoUuid,
	}

	repo := dao.Repository{UUID: repoUuid}

	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, repo.UUID, config.RepositorySnapshotTask).Return([]string{"task-uuid"}, nil).NotBefore(
		suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(repoResp, nil),
	)

	body, err := json.Marshal("")
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/"+uuid+"/snapshot/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusConflict, code)
}

func (suite *ReposSuite) TestCreateSnapshotErrorSnapshottingNotEnabled() {
	t := suite.T()
	config.Load()
	config.Get().Features.Snapshots.Enabled = true
	config.Get().Features.Snapshots.Accounts = &[]string{test_handler.MockAccountNumber}
	defer resetFeatures()
	uuid := "abcadaba"
	repoUuid := "repoUuid"
	repoResp := api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: repoUuid,
	}

	repo := dao.Repository{UUID: repoUuid}

	suite.reg.TaskInfo.On("FetchActiveTasks", test.MockCtx(), test_handler.MockOrgId, repo.UUID, config.RepositorySnapshotTask).Return([]string{}, nil).NotBefore(
		suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(repoResp, nil),
	)

	body, err := json.Marshal("")
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/"+uuid+"/snapshot/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *ReposSuite) TestIntrospectRepositoryBeforeTimeLimit() {
	t := suite.T()

	t.Setenv("OPTIONS_INTROSPECT_API_TIME_LIMIT_SEC", "300")
	config.Load()

	uuid := "abcadaba"
	intReq := api.RepositoryIntrospectRequest{ResetCount: true}
	repoResp := api.RepositoryResponse{
		Name: "my repo",
		URL:  "https://example.com",
		UUID: uuid,
	}

	now := time.Now()
	repo := dao.Repository{UUID: uuid, LastIntrospectionTime: &now}

	// Fetch will filter the request by Org ID before updating
	suite.reg.Repository.On("FetchForUrl", test.MockCtx(), repoResp.URL, &repoResp.Origin).Return(repo, nil).NotBefore(
		suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(repoResp, nil),
	)
	body, err := json.Marshal(intReq)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/"+uuid+"/introspect/?reset_count=true",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *ReposSuite) TestGetGpgKeyFile() {
	t := suite.T()

	// Test returns GPG Key file
	uuid := "abcadaba"
	repo := api.RepositoryResponse{
		Name:   "my repo",
		URL:    "https://example.com",
		UUID:   uuid,
		GpgKey: "gpg",
	}

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/repository_gpg_key/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	suite.reg.RepositoryConfig.On("FetchWithoutOrgID", req.Context(), uuid, false).Return(repo, nil).Once()

	code, body, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)

	gpgKeyFile := string(body)
	assert.Equal(t, repo.GpgKey, gpgKeyFile)
	assert.Equal(t, http.StatusOK, code)

	// Test GPG Key not found
	repoNoGPG := api.RepositoryResponse{
		Name: "my repo",
		URL:  "https://example.com",
		UUID: uuid,
	}

	req = httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/repositories/"+uuid+"/gpg_key/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	suite.reg.RepositoryConfig.On("FetchWithoutOrgID", req.Context(), uuid).Return(repoNoGPG, nil).Once()

	code, _, err = suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func TestReposSuite(t *testing.T) {
	suite.Run(t, new(ReposSuite))
}

func (suite *ReposSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
	suite.tcMock = client.NewMockTaskClient(suite.T())
	suite.pcMock = pulp_client.NewMockPulpGlobalClient(suite.T())
	suite.fsMock = feature_service_client.NewMockFeatureServiceClient(suite.T())
}
