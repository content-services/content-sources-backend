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
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/openlyinc/pointy"
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
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	rh := RepositoryHandler{
		DaoRegistry: *suite.reg.ToDaoRegistry(),
		TaskClient:  suite.tcMock,
	}

	RegisterRepositoryRoutes(pathPrefix, suite.reg.ToDaoRegistry(), &rh.TaskClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func mockTaskClientEnqueueIntrospect(tcMock *client.MockTaskClient, expectedUrl string, repositoryUuid string) {
	tcMock.On("Enqueue", queue.Task{
		Typename:       payloads.Introspect,
		Payload:        payloads.IntrospectPayload{Url: expectedUrl, Force: true},
		Dependencies:   nil,
		OrgId:          test_handler.MockOrgId,
		RepositoryUUID: &repositoryUuid,
	}).Return(nil, nil)
}

func mockTaskClientEnqueueSnapshot(repoSuite *ReposSuite, response *api.RepositoryResponse) {
	repoSuite.tcMock.On("Enqueue", queue.Task{
		Typename:       config.RepositorySnapshotTask,
		Payload:        payloads.SnapshotPayload{},
		OrgId:          response.OrgID,
		RepositoryUUID: &response.RepositoryUUID,
	}).Return(nil, nil)
	repoSuite.reg.RepositoryConfig.On(
		"UpdateLastSnapshotTask",
		test.MockCtx(),
		"00000000-0000-0000-0000-000000000000",
		response.OrgID,
		response.RepositoryUUID,
	).Return(nil)
	response.LastSnapshotTaskUUID = "00000000-0000-0000-0000-000000000000"
}

func mockSnapshotDeleteEvent(tcMock *client.MockTaskClient, repoConfigUUID string) {
	tcMock.On("Enqueue", queue.Task{
		Typename:       config.DeleteRepositorySnapshotsTask,
		Payload:        tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repoConfigUUID},
		Dependencies:   nil,
		OrgId:          test_handler.MockOrgId,
		RepositoryUUID: &repoConfigUUID,
	}).Return(nil, nil)
}

type ReposSuite struct {
	suite.Suite
	reg    *dao.MockDaoRegistry
	tcMock *client.MockTaskClient
	pcMock *pulp_client.MockPulpGlobalClient
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
	}

	repo := createRepoRequest("my repo", "https://example.com")
	repo.Snapshot = pointy.Pointer(true)
	repo.ModuleHotfixes = pointy.Pointer(true)
	repo.FillDefaults()

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
	repo.FillDefaults()
	repo.Snapshot = pointy.Bool(true)

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
	repo.FillDefaults()
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
	repo1.FillDefaults()
	repo1.Snapshot = pointy.Bool(true)
	repoUuid1 := "repoUuid1"

	repo2 := createRepoRequest("repo_2", "https://example2.com")
	repo2.ModuleHotfixes = pointy.Bool(true)
	repo2.FillDefaults()
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
		},
		{
			Name:           "repo_2",
			URL:            "https://example2.com",
			RepositoryUUID: repoUuid2,
			ModuleHotfixes: true,
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
	repo1.FillDefaults()

	repo2 := createRepoRequest("repo_2", "")
	repo2.FillDefaults()

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

	var repos = make([]api.RepositoryRequest, BulkCreateLimit+1)
	for i := 0; i < BulkCreateLimit+1; i++ {
		repos[i] = createRepoRequest("repo"+strconv.Itoa(i), "example"+strconv.Itoa(i)+".com")
		repos[i].FillDefaults()
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

func (suite *ReposSuite) TestDelete() {
	t := suite.T()
	uuid := "valid-uuid"

	suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: uuid,
	}, nil)
	suite.reg.TaskInfo.On("IsSnapshotInProgress", test.MockCtx(), test_handler.MockOrgId, uuid).Return(false, nil)
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
	suite.reg.TaskInfo.On("IsSnapshotInProgress", test.MockCtx(), test_handler.MockOrgId, uuid).Return(false, nil)
	suite.reg.RepositoryConfig.On("SoftDelete", test.MockCtx(), test_handler.MockOrgId, uuid).Return(&daoError)

	req := httptest.NewRequest(http.MethodDelete, api.FullRootPath()+"/repositories/"+uuid, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *ReposSuite) TestSnapshotInProgress() {
	t := suite.T()
	uuid := "inprogress-uuid"

	suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: uuid,
	}, nil)
	suite.reg.TaskInfo.On("IsSnapshotInProgress", test.MockCtx(), test_handler.MockOrgId, uuid).Return(true, nil)

	req := httptest.NewRequest(http.MethodDelete, api.FullRootPath()+"/repositories/"+uuid, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
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
		suite.reg.TaskInfo.On("IsSnapshotInProgress", test.MockCtx(), test_handler.MockOrgId, uuids[i]).Return(false, nil)
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
	suite.reg.TaskInfo.On("IsSnapshotInProgress", test.MockCtx(), test_handler.MockOrgId, uuids[0]).Return(false, nil)
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
	suite.reg.TaskInfo.On("IsSnapshotInProgress", test.MockCtx(), test_handler.MockOrgId, uuids[0]).Return(true, nil)
	suite.reg.TaskInfo.On("IsSnapshotInProgress", test.MockCtx(), test_handler.MockOrgId, uuids[1]).Return(false, nil)

	suite.reg.RepositoryConfig.On("BulkDelete", test.MockCtx(), test_handler.MockOrgId, uuids).Return([]error{})

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

	assert.Equal(t, http.StatusBadRequest, response.Errors[0].Status)
	assert.Equal(t, "Cannot delete repository while snapshot is in progress", response.Errors[0].Detail)
	assert.Equal(t, "", response.Errors[1].Detail)
}

func (suite *ReposSuite) TestBulkDeleteTooMany() {
	t := suite.T()

	var uuids = make([]string, BulkDeleteLimit+1)
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
	request := createRepoRequest("Some Name", "https://example.com")
	expected := createRepoRequest(*request.Name, *request.URL)
	expected.FillDefaults()

	suite.reg.RepositoryConfig.WithContextMock().On("Update", test.MockCtx(), test_handler.MockOrgId, uuid, expected).Return(false, nil)
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: repoUuid,
	}, nil)
	suite.reg.RepositoryConfig.On("Update", test.MockCtx(), test_handler.MockOrgId, uuid, expected).Return(false, nil)

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
	repoConfigUuid := "RepoConfigUuid"
	repoUuid := "RepoUuid"
	request := createRepoRequest("Some Name", "http://someurl.com")
	expected := createRepoRequest(*request.Name, *request.URL)
	repoConfig := api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           repoConfigUuid,
		RepositoryUUID: repoUuid,
		Snapshot:       true,
	}

	suite.reg.RepositoryConfig.WithContextMock().On("Update", test.MockCtx(), test_handler.MockOrgId, repoConfigUuid, expected).Return(true, nil)
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, repoConfigUuid).Return(repoConfig, nil)
	suite.reg.TaskInfo.On("IsSnapshotInProgress", test.MockCtx(), *expected.OrgID, repoUuid).Return(false, nil)

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
	request := createRepoRequest("Some Name", "https://example.com")
	expected := createRepoRequest(*request.Name, *request.URL)

	suite.reg.RepositoryConfig.WithContextMock().On("Update", test.MockCtx(), test_handler.MockOrgId, uuid, expected).Return(false, nil)
	suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: repoUuid,
		Snapshot:       false,
	}, nil)

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

func (suite *ReposSuite) TestIntrospectRepository() {
	t := suite.T()

	t.Setenv("OPTIONS_INTROSPECT_API_TIME_LIMIT_SEC", "0")
	config.Load()

	uuid := "abcadaba"
	repoUuid := "repoUuid"
	intReq := api.RepositoryIntrospectRequest{ResetCount: true}
	repoResp := api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: repoUuid,
	}
	repoUpdate := dao.RepositoryUpdate{UUID: "12345", FailedIntrospectionsCount: pointy.Int(0), LastIntrospectionStatus: pointy.String("Pending")}
	now := time.Now()
	repo := dao.Repository{UUID: "12345", LastIntrospectionTime: &now}

	mockTaskClientEnqueueIntrospect(suite.tcMock, "https://example.com", repoUuid)

	// Fetch will filter the request by Org ID before updating
	suite.reg.Repository.On("Update", test.MockCtx(), repoUpdate).Return(nil).NotBefore(
		suite.reg.Repository.On("FetchForUrl", test.MockCtx(), repoResp.URL).Return(repo, nil).NotBefore(
			suite.reg.RepositoryConfig.WithContextMock().On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(repoResp, nil),
		),
	)
	body, err := json.Marshal(intReq)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/"+uuid+"/introspect/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNoContent, code)
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
	suite.reg.Repository.On("FetchForUrl", test.MockCtx(), repoResp.URL).Return(repo, nil).NotBefore(
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

	uuid := "abcadaba"
	repoUuid := "repoUuid"
	repoResp := api.RepositoryResponse{
		Name:           "my repo",
		URL:            "https://example.com",
		UUID:           uuid,
		RepositoryUUID: repoUuid,
		Snapshot:       true,
	}

	repoUpdate := dao.RepositoryUpdate{UUID: repoUuid, LastIntrospectionStatus: pointy.String(config.StatusPending)}
	repo := dao.Repository{UUID: repoUuid}

	mockTaskClientEnqueueSnapshot(suite, &repoResp)

	// Fetch will filter the request by Org ID before updating
	suite.reg.Repository.On("Update", test.MockCtx(), repoUpdate).Return(nil).NotBefore(
		suite.reg.TaskInfo.On("IsSnapshotInProgress", test.MockCtx(), test_handler.MockOrgId, repo.UUID).Return(false, nil).
			NotBefore(suite.reg.RepositoryConfig.On("Fetch", test.MockCtx(), test_handler.MockOrgId, uuid).Return(repoResp, nil)))

	body, err := json.Marshal("")
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/repositories/"+uuid+"/snapshot/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveRepositoriesRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNoContent, code)
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

	suite.reg.TaskInfo.On("IsSnapshotInProgress", test.MockCtx(), test_handler.MockOrgId, repo.UUID).Return(true, nil).NotBefore(
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

	suite.reg.TaskInfo.On("IsSnapshotInProgress", test.MockCtx(), test_handler.MockOrgId, repo.UUID).Return(false, nil).NotBefore(
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
	suite.reg.Repository.On("FetchForUrl", test.MockCtx(), repoResp.URL).Return(repo, nil).NotBefore(
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
	suite.reg.RepositoryConfig.On("FetchWithoutOrgID", req.Context(), uuid).Return(repo, nil).Once()

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
}
