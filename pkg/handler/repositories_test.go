package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/event/producer"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/test/mocks"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

var mockAccountNumber = seeds.RandomAccountId()
var mockOrgId = seeds.RandomOrgId()

func encodedIdentity(t *testing.T) string {
	mockIdentity := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: mockAccountNumber,
			Internal: identity.Internal{
				OrgID: mockOrgId,
			},
			Type: "Associate",
		},
	}
	jsonIdentity, err := json.Marshal(mockIdentity)
	if err != nil {
		t.Error("Could not marshal JSON")
	}
	return base64.StdEncoding.EncodeToString(jsonIdentity)
}

func createRepoRequest(name string, url string) api.RepositoryRequest {
	blank := ""
	account := mockAccountNumber
	org := mockOrgId
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
			AccountID:                    mockAccountNumber,
			OrgID:                        mockOrgId,
			LastIntrospectionTime:        "2022-08-31 14:17:50.257623 -0400 EDT",
			LastIntrospectionSuccessTime: "2022-08-31 14:17:50.257623 -0400 EDT",
			LastIntrospectionUpdateTime:  "2022-08-31 14:17:50.257623 -0400 EDT",
			LastIntrospectionError:       "",
			Status:                       "Valid",
			GpgKey:                       "foo",
			MetadataVerification:         true,
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

func prepareProducer() *kafka.Producer {
	output, _ := producer.NewProducer(&config.Get().Kafka)
	return output
}

func serveRepositoriesRouter(req *http.Request, mockDao *mocks.RepositoryConfigDao) (int, []byte, error) {
	router := echo.New()
	router.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(config.WrapMiddlewareWithSkipper(identity.EnforceIdentity, config.SkipLiveness))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(fullRootPath())

	var prod producer.IntrospectRequest
	var err error
	if prod, err = producer.NewIntrospectRequest(prepareProducer()); err != nil {
		return 0, nil, fmt.Errorf("error creating IntrospectRequest producer")
	}

	rh := RepositoryHandler{
		RepositoryDao:             mockDao,
		IntrospectRequestProducer: prod,
	}
	RegisterRepositoryRoutes(pathPrefix, &rh.RepositoryDao, &rh.IntrospectRequestProducer)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	return response.StatusCode, body, err
}

type ReposSuite struct {
	suite.Suite
	savedDB *gorm.DB
}

func (suite *ReposSuite) SetupTest() {
	suite.savedDB = db.DB
	db.DB = db.DB.Begin()
}

func (suite *ReposSuite) TearDownTest() {
	//Rollback and reset db.DB
	db.DB.Rollback()
	db.DB = suite.savedDB
}

func (suite *ReposSuite) TestSimple() {
	t := suite.T()

	mockDao := mocks.RepositoryConfigDao{}

	collection := createRepoCollection(1, 10, 0)
	paginationData := api.PaginationData{Limit: 10, Offset: DefaultOffset}
	mockDao.On("List", mockOrgId, paginationData, api.FilterData{}).Return(collection, int64(1), nil)

	path := fmt.Sprintf("%s/repositories/?limit=%d", fullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, body, err := serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)

	response := api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)
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
}

func (suite *ReposSuite) TestListNoRepositories() {
	t := suite.T()

	mockDao := mocks.RepositoryConfigDao{}

	collection := api.RepositoryCollectionResponse{}
	paginationData := api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset}
	mockDao.On("List", mockOrgId, paginationData, api.FilterData{}).Return(collection, int64(0), nil)

	req := httptest.NewRequest(http.MethodGet, fullRootPath()+"/repositories/", nil)
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, body, err := serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(0), response.Meta.Count)
	assert.Equal(t, 100, response.Meta.Limit)
	assert.Equal(t, 0, len(response.Data))
	assert.Equal(t, fullRootPath()+"/repositories/?limit=100&offset=0", response.Links.Last)
	assert.Equal(t, fullRootPath()+"/repositories/?limit=100&offset=0", response.Links.First)
}

func (suite *ReposSuite) TestListPagedExtraRemaining() {
	t := suite.T()

	mockDao := mocks.RepositoryConfigDao{}

	collection := api.RepositoryCollectionResponse{}
	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 100}

	mockDao.On("List", mockOrgId, paginationData1, api.FilterData{}).Return(collection, int64(102), nil).Once()
	mockDao.On("List", mockOrgId, paginationData2, api.FilterData{}).Return(collection, int64(102), nil).Once()

	path := fmt.Sprintf("%s/repositories/?limit=%d", fullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, body, err := serveRepositoriesRouter(req, &mockDao)
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
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))
	code, body, err = serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)
}

func (suite *ReposSuite) TestListPagedNoRemaining() {
	t := suite.T()

	mockDao := mocks.RepositoryConfigDao{}
	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 90}

	collection := api.RepositoryCollectionResponse{}
	mockDao.On("List", mockOrgId, paginationData1, api.FilterData{}).Return(collection, int64(100), nil)
	mockDao.On("List", mockOrgId, paginationData2, api.FilterData{}).Return(collection, int64(100), nil)

	path := fmt.Sprintf("%s/repositories/?limit=%d", fullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, body, err := serveRepositoriesRouter(req, &mockDao)
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
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))
	code, body, err = serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)
}

func (suite *ReposSuite) TestListDaoError() {
	t := suite.T()

	mockDao := mocks.RepositoryConfigDao{}
	daoError := ce.DaoError{
		Message: "Column doesn't exist",
	}
	paginationData := api.PaginationData{Limit: DefaultLimit}

	mockDao.On("List", mockOrgId, paginationData, api.FilterData{}).
		Return(api.RepositoryCollectionResponse{}, int64(0), &daoError)

	path := fmt.Sprintf("%s/repositories/", fullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, _, err := serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)
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

	mockDao := mocks.RepositoryConfigDao{}
	mockDao.On("Fetch", mockOrgId, uuid).Return(repo, nil)

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodGet, fullRootPath()+"/repositories/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, body, err := serveRepositoriesRouter(req, &mockDao)
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

	mockDao := mocks.RepositoryConfigDao{}
	daoError := ce.DaoError{
		NotFound: true,
		Message:  "Not found",
	}
	mockDao.On("Fetch", mockOrgId, uuid).Return(api.RepositoryResponse{}, &daoError)

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodGet, fullRootPath()+"/repositories/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, _, _ := serveRepositoriesRouter(req, &mockDao)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *ReposSuite) TestCreate() {
	expected := api.RepositoryResponse{
		Name: "my repo",
		URL:  "https://example.com",
	}

	repo := createRepoRequest("my repo", "https://example.com")
	repo.FillDefaults()

	mockDao := mocks.RepositoryConfigDao{}
	mockDao.On("Create", repo).Return(expected, nil)

	t := suite.T()

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, fullRootPath()+"/repositories/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, body, err := serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)

	var response api.RepositoryResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.NotEmpty(t, response.Name)
	assert.Equal(t, http.StatusCreated, code)
}

func (suite *ReposSuite) TestCreateAlreadyExists() {
	t := suite.T()

	repo := createRepoRequest("my repo", "https://example.com")
	repo.FillDefaults()
	mockDao := mocks.RepositoryConfigDao{}
	daoError := ce.DaoError{
		BadValidation: true,
		Message:       "Already exists",
	}
	mockDao.On("Create", repo).Return(api.RepositoryResponse{}, &daoError)

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}
	req := httptest.NewRequest(http.MethodPost, fullRootPath()+"/repositories/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, body, err := serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)

	var response api.RepositoryResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Empty(t, response.UUID)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *ReposSuite) TestBulkCreate() {
	t := suite.T()

	repo1 := createRepoRequest("repo_1", "https://example1.com")
	repo1.FillDefaults()

	repo2 := createRepoRequest("repo_2", "https://example2.com")
	repo2.FillDefaults()

	repos := []api.RepositoryRequest{
		repo1,
		repo2,
	}

	expected := []api.RepositoryResponse{
		{
			Name: "repo_1",
			URL:  "https://example1.com",
		},
		{
			Name: "repo_2",
			URL:  "https://example2.com",
		},
	}

	mockDao := mocks.RepositoryConfigDao{}
	mockDao.On("BulkCreate", repos).Return(expected, []error{})

	body, err := json.Marshal(repos)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, fullRootPath()+"/repositories/bulk_create/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, body, err := serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)

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

	mockDao := mocks.RepositoryConfigDao{}
	mockDao.On("BulkCreate", repos).Return([]api.RepositoryResponse{}, expected)

	body, err := json.Marshal(repos)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, fullRootPath()+"/repositories/bulk_create/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, body, err := serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)

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

	req := httptest.NewRequest(http.MethodPost, fullRootPath()+"/repositories/bulk_create/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, _, err := serveRepositoriesRouter(req, nil)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusRequestEntityTooLarge, code)
}

func (suite *ReposSuite) TestDelete() {
	t := suite.T()

	uuid := "valid-uuid"
	mockDao := mocks.RepositoryConfigDao{}
	mockDao.On("Delete", mockOrgId, uuid).Return(nil)

	req := httptest.NewRequest(http.MethodDelete, fullRootPath()+"/repositories/"+uuid, nil)
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, _, err := serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)
	assert.Equal(t, http.StatusNoContent, code)
}

func (suite *ReposSuite) TestDeleteNotFound() {
	t := suite.T()

	uuid := "invalid-uuid"
	mockDao := mocks.RepositoryConfigDao{}
	daoError := ce.DaoError{
		NotFound: true,
	}
	mockDao.On("Delete", mockOrgId, uuid).Return(&daoError)

	req := httptest.NewRequest(http.MethodDelete, fullRootPath()+"/repositories/"+uuid, nil)
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, _, err := serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *ReposSuite) TestFullUpdate() {
	t := suite.T()

	uuid := "someuuid"
	request := createRepoRequest("Some Name", "http://someurl.com")
	expected := createRepoRequest(*request.Name, *request.URL)
	expected.FillDefaults()

	mockDao := mocks.RepositoryConfigDao{}
	mockDao.On("Update", mockOrgId, uuid, expected).Return(nil)

	body, err := json.Marshal(request)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPut, fullRootPath()+"/repositories/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, _, err := serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)
	assert.Equal(t, http.StatusOK, code)
}

func (suite *ReposSuite) TestPartialUpdate() {
	t := suite.T()

	uuid := "someuuid"
	request := createRepoRequest("Some Name", "http://someurl.com")
	expected := createRepoRequest(*request.Name, *request.URL)

	mockDao := mocks.RepositoryConfigDao{}
	mockDao.On("Update", mockOrgId, uuid, expected).Return(nil)

	body, err := json.Marshal(request)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPatch, fullRootPath()+"/repositories/"+uuid,
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, encodedIdentity(t))

	code, _, err := serveRepositoriesRouter(req, &mockDao)
	assert.Nil(t, err)
	mockDao.AssertExpectations(t)
	assert.Equal(t, http.StatusOK, code)
}

func TestReposSuite(t *testing.T) {
	suite.Run(t, new(ReposSuite))
}
