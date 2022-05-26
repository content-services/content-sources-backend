package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

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
		log.Fatalln(err)
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/"+rootRoute()+"/repositories/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	response := RepositoryCollectionResponse{}
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
		assert.Equal(t, repoConfig.URL, response.Data[0].Url)
		assert.Equal(t, repoConfig.AccountID, response.Data[0].AccountId)
	}
}

func (suite *ReposSuite) TestListPagedExtraRemaining() {
	t := suite.T()

	err := seeds.SeedRepositoryConfigurations(db.DB, 102)
	if err != nil {
		log.Fatalln(err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/"+rootRoute()+"/repositories/?limit=10", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	response := RepositoryCollectionResponse{}

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
			response = RepositoryCollectionResponse{}
			err = json.Unmarshal(rec.Body.Bytes(), &response)
			assert.Nil(t, err)
			assert.Equal(t, 2, len(response.Data))
		}

	}

}
func (suite *ReposSuite) TestListPagedNoRemaining() {
	err := seeds.SeedRepositoryConfigurations(db.DB, 100)
	if err != nil {
		log.Fatalln(err)
	}

	t := suite.T()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/"+rootRoute()+"/repositories/?limit=10", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	response := RepositoryCollectionResponse{}

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
			response = RepositoryCollectionResponse{}
			err = json.Unmarshal(rec.Body.Bytes(), &response)
			assert.Nil(t, err)
			assert.Equal(t, 10, len(response.Data))
		}
	}

}

func (suite *ReposSuite) TestDelete() {
	t := suite.T()
	err := seeds.SeedRepositoryConfigurations(db.DB, 1)
	if err != nil {
		log.Fatalln(err)
	}
	repoConfig := models.RepositoryConfiguration{}
	db.DB.First(&repoConfig)

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/"+rootRoute()+"/repositories/"+repoConfig.UUID, nil)

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
	req := httptest.NewRequest(http.MethodDelete, "/"+rootRoute()+"/repositories/"+repoConfig.UUID, nil)

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

func TestReposSuite(t *testing.T) {
	suite.Run(t, new(ReposSuite))
}
