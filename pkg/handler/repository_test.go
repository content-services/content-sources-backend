package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type MockRepositoryDao struct {
	mock.Mock
}

func (r *MockRepositoryDao) Create(newRepo api.RepositoryRequest) error {
	args := r.Called(newRepo)
	return args.Error(0)
}
func (r *MockRepositoryDao) Fetch(orgId string, uuid string) api.RepositoryResponse {
	args := r.Called(orgId, uuid)
	if args.Get(0) == nil {
		return api.RepositoryResponse{}
	}
	rr, ok := args.Get(0).(api.RepositoryResponse)
	if ok {
		return rr
	} else {
		return api.RepositoryResponse{}
	}
}

func (r *MockRepositoryDao) Update(orgId string, uuid string, repoParams api.RepositoryRequest) error {
	args := r.Called(orgId, uuid, repoParams)
	return args.Error(0)
}

const mockAccountNumber = "0000"
const mockOrgId = "1111"

func encodedIdentity(t *testing.T) string {
	mockIdentity := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: mockAccountNumber,
			Internal: identity.Internal{
				OrgID: mockOrgId,
			},
		},
	}
	jsonIdentity, err := json.Marshal(mockIdentity)
	if err != nil {
		t.Error("Could not marshal JSON")
	}
	return base64.StdEncoding.EncodeToString(jsonIdentity)
}

func repoRequest(name string, url string) api.RepositoryRequest {
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

type ReposSuite struct {
	suite.Suite
	savedDB *gorm.DB
}

func (suite *ReposSuite) SetupTest() {
	suite.savedDB = db.DB
	db.DB = db.DB.Begin()
	db.DB.Where("1=1").Delete(models.RepositoryConfiguration{})
}

func (suite *ReposSuite) TearDownTest() {
	//Rollback and reset db.DB
	db.DB.Rollback()
	db.DB = suite.savedDB
}

func (suite *ReposSuite) TestSimple() {
	t := suite.T()
	err := seeds.SeedRepositoryConfigurations(db.DB, 1)
	if err != nil {
		log.Fatal().Err(err)
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/"+fullRootPath()+"/repositories/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	response := api.RepositoryCollectionResponse{}
	repoConfig := models.RepositoryConfiguration{}
	db.DB.First(&repoConfig)

	// Assertions
	if assert.NoError(t, listRepositories(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.Nil(t, err)
		assert.Equal(t, 0, response.Meta.Offset)
		assert.Equal(t, int64(1), response.Meta.Count)
		assert.Equal(t, 100, response.Meta.Limit)
		assert.Equal(t, 1, len(response.Data))
		assert.Equal(t, repoConfig.Name, response.Data[0].Name)
		assert.Equal(t, repoConfig.URL, response.Data[0].URL)
		assert.Equal(t, repoConfig.AccountID, response.Data[0].AccountID)
	}
}

func (suite *ReposSuite) TestListNoRepositories() {
	t := suite.T()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/"+fullRootPath()+"/repositories/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	response := api.RepositoryCollectionResponse{}

	// Assertions
	if assert.NoError(t, listRepositories(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.Nil(t, err)
		assert.Equal(t, 0, response.Meta.Offset)
		assert.Equal(t, int64(0), response.Meta.Count)
		assert.Equal(t, 100, response.Meta.Limit)
		assert.Equal(t, 0, len(response.Data))
		assert.Equal(t, "/"+fullRootPath()+"/repositories/?limit=100&offset=0", response.Links.Last)
		assert.Equal(t, "/"+fullRootPath()+"/repositories/?limit=100&offset=0", response.Links.First)
	}
}

func (suite *ReposSuite) TestListPagedExtraRemaining() {
	t := suite.T()

	err := seeds.SeedRepositoryConfigurations(db.DB, 102)
	if err != nil {
		log.Fatal().Err(err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/"+fullRootPath()+"/repositories/?limit=10", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	response := api.RepositoryCollectionResponse{}

	// Assertions
	if assert.NoError(t, listRepositories(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.Nil(t, err)
		assert.Equal(t, 0, response.Meta.Offset)
		assert.Equal(t, 10, response.Meta.Limit)
		assert.Equal(t, 10, len(response.Data))
		assert.Equal(t, int64(102), response.Meta.Count)
		assert.NotEmpty(t, response.Links.Last)
		//fetch last page

		req = httptest.NewRequest(http.MethodGet, response.Links.Last, nil)
		rec := httptest.NewRecorder()

		c = e.NewContext(req, rec)
		if assert.NoError(t, listRepositories(c)) {
			response = api.RepositoryCollectionResponse{}
			err = json.Unmarshal(rec.Body.Bytes(), &response)
			assert.Nil(t, err)
			assert.Equal(t, 2, len(response.Data))
		}

	}

}
func (suite *ReposSuite) TestListPagedNoRemaining() {
	err := seeds.SeedRepositoryConfigurations(db.DB, 100)
	if err != nil {
		log.Fatal().Err(err)
	}

	t := suite.T()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/"+fullRootPath()+"/repositories/?limit=10", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	response := api.RepositoryCollectionResponse{}

	if assert.NoError(t, listRepositories(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.Nil(t, err)
		assert.Equal(t, 0, response.Meta.Offset)
		assert.Equal(t, 10, response.Meta.Limit)
		assert.Equal(t, 10, len(response.Data))
		assert.Equal(t, int64(100), response.Meta.Count)
		assert.NotEmpty(t, response.Links.Last)
		//fetch last page

		req = httptest.NewRequest(http.MethodGet, response.Links.Last, nil)
		rec := httptest.NewRecorder()

		c = e.NewContext(req, rec)
		if assert.NoError(t, listRepositories(c)) {
			response = api.RepositoryCollectionResponse{}
			err = json.Unmarshal(rec.Body.Bytes(), &response)
			assert.Nil(t, err)
			assert.Equal(t, 10, len(response.Data))
		}
	}
}

func (suite *ReposSuite) TestFetch() {
	uuid := "abcadaba"
	repo := api.RepositoryResponse{
		Name: "my repo",
		URL:  "https://example.com",
		UUID: uuid,
	}

	mockDao := MockRepositoryDao{}
	mockDao.On("Fetch", mockOrgId, uuid).Return(repo)
	handler := RepositoryHandler{RepositoryDao: &mockDao}

	t := suite.T()
	e := echo.New()

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodGet, "/"+fullRootPath()+"/repositories/"+uuid+"/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-rh-identity", encodedIdentity(t))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("uuid")
	c.SetParamValues(uuid)

	if assert.NoError(t, handler.fetch(c)) {
		mockDao.AssertExpectations(t)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
}

func (suite *ReposSuite) TestCreate() {
	repo := api.RepositoryResponse{
		Name: "my repo",
		URL:  "https://example.com",
	}

	expected := repoRequest(repo.Name, repo.URL)
	expected.FillDefaults()
	mockDao := MockRepositoryDao{}
	mockDao.On("Create", expected).Return(nil)
	handler := RepositoryHandler{RepositoryDao: &mockDao}

	t := suite.T()
	e := echo.New()

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, "/"+fullRootPath()+"/repositories/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-rh-identity", encodedIdentity(t))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if assert.NoError(t, handler.createRepository(c)) {
		mockDao.AssertExpectations(t)
		assert.Equal(t, http.StatusCreated, rec.Code)
	}
}

func (suite *ReposSuite) TestCreateAlreadyExists() {
	t := suite.T()
	e := echo.New()
	repo := api.RepositoryResponse{
		Name: "my repo",
		URL:  "https://example.com",
	}

	expected := repoRequest(repo.Name, repo.URL)
	expected.FillDefaults()

	mockDao := MockRepositoryDao{}
	error := dao.Error{
		BadValidation: true,
		Message:       "Already exists",
	}
	mockDao.On("Create", expected).Return(&error)
	handler := RepositoryHandler{RepositoryDao: &mockDao}

	body, err := json.Marshal(repo)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, "/"+fullRootPath()+"/repositories/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-rh-identity", encodedIdentity(t))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = handler.createRepository(c)
	assert.Error(t, err)

	httpErr, ok := err.(*echo.HTTPError)
	if ok {
		assert.Equal(t, 400, httpErr.Code)
	} else {
		assert.Fail(t, "expected a 400 http error")
	}
}

func (suite *ReposSuite) TestDelete() {
	t := suite.T()
	err := seeds.SeedRepositoryConfigurations(db.DB, 1)
	if err != nil {
		log.Fatal().Err(err)
	}
	repoConfig := models.RepositoryConfiguration{}
	db.DB.First(&repoConfig)

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, fullRootPath()+"/repositories/"+repoConfig.UUID, nil)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("uuid")
	c.SetParamValues(repoConfig.UUID)

	if assert.NoError(t, deleteRepository(c)) {
		repoConfig = models.RepositoryConfiguration{}
		db.DB.First(&repoConfig)
		assert.Empty(t, repoConfig.UUID)
		assert.Equal(t, 204, rec.Code)
	}
}

func (suite *ReposSuite) TestDeleteNotFound() {
	t := suite.T()

	repoConfig := models.RepositoryConfiguration{}
	db.DB.First(&repoConfig)
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, fullRootPath()+"/repositories/"+repoConfig.UUID, nil)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("uuid")
	c.SetParamValues("SomeFalseUUID")
	err := deleteRepository(c)
	assert.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	if ok {
		assert.Equal(t, 404, httpErr.Code)
	} else {
		assert.Fail(t, "expected an http error")
	}

}

func (suite *ReposSuite) TestFullUpdate() {
	uuid := "someuuid"
	request := repoRequest("Some Name", "http://someurl.com")
	expected := repoRequest(*request.Name, *request.URL)
	expected.FillDefaults()

	mockDao := MockRepositoryDao{}
	mockDao.On("Update", mockOrgId, uuid, expected).Return(nil)
	handler := RepositoryHandler{RepositoryDao: &mockDao}

	t := suite.T()
	e := echo.New()

	body, err := json.Marshal(request)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPut, "/"+fullRootPath()+"/repositories/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-rh-identity", encodedIdentity(t))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("uuid")
	c.SetParamValues(uuid)

	if assert.NoError(t, handler.fullUpdate(c)) {
		mockDao.AssertExpectations(t)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
}

func (suite *ReposSuite) TestPartialUpdate() {
	uuid := "someuuid"
	request := repoRequest("Some Name", "http://someurl.com")
	expected := repoRequest(*request.Name, *request.URL)

	mockDao := MockRepositoryDao{}
	mockDao.On("Update", mockOrgId, uuid, expected).Return(nil)
	handler := RepositoryHandler{RepositoryDao: &mockDao}

	t := suite.T()
	e := echo.New()

	body, err := json.Marshal(request)
	if err != nil {
		t.Error("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPatch, "/"+fullRootPath()+"/repositories/",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-rh-identity", encodedIdentity(t))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("uuid")
	c.SetParamValues(uuid)

	if assert.NoError(t, handler.partialUpdate(c)) {
		mockDao.AssertExpectations(t)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
}

func TestReposSuite(t *testing.T) {
	suite.Run(t, new(ReposSuite))
}
