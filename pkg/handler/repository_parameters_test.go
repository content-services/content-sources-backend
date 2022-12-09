package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/test/mocks"
	"github.com/labstack/echo/v4"
	"github.com/openlyinc/pointy"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func serveRepositoryParametersRouter(req *http.Request, mockDao *mocks.RepositoryConfigDao) (int, []byte, error) {
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(config.WrapMiddlewareWithSkipper(identity.EnforceIdentity, config.SkipLiveness))
	pathPrefix := router.Group(fullRootPath())

	repoDao := dao.RepositoryConfigDao(mockDao)
	RegisterRepositoryParameterRoutes(pathPrefix, &repoDao)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	return response.StatusCode, body, err
}

type RepositoryParameterSuite struct {
	suite.Suite
}

func (suite *RepositoryParameterSuite) TestListParams() {
	t := suite.T()
	mockDao := mocks.RepositoryConfigDao{}
	path := fmt.Sprintf("%s/repository_parameters/", fullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	setHeaders(t, req)
	code, body, err := serveRepositoryParametersRouter(req, &mockDao)

	assert.Nil(t, err)

	response := api.RepositoryParameterResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, response.DistributionArches)
	assert.NotEmpty(t, response.DistributionVersions)
}

func (suite *RepositoryParameterSuite) TestValidate() {
	t := suite.T()

	mockDao := mocks.RepositoryConfigDao{}
	path := fmt.Sprintf("%s/repository_parameters/validate/", fullRootPath())

	requestBody := []api.RepositoryValidationRequest{
		{
			Name: pointy.String("myValidateRepo"),
			UUID: pointy.String("steve-the-id"),
		},
		{
			URL:  pointy.String("http://myrepo.com"),
			UUID: pointy.String("paul-the-id"),
		},
		{},
	}

	expectedResponse := []api.RepositoryValidationResponse{
		{
			Name: api.GenericAttributeValidationResponse{
				Valid: true,
			},
		},
		{
			URL: api.UrlValidationResponse{
				Valid:           true,
				MetadataPresent: true,
			},
		},
		{
			Name: api.GenericAttributeValidationResponse{
				Skipped: true,
			},
			URL: api.UrlValidationResponse{
				Skipped: true,
			},
		},
	}

	requestJson, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatal("Could not marshal JSON")
	}

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(requestJson))
	setHeaders(t, req)

	mockDao.Mock.On("ValidateParameters", test_handler.MockOrgId, requestBody[0]).Return(expectedResponse[0])
	mockDao.Mock.On("ValidateParameters", test_handler.MockOrgId, requestBody[1]).Return(expectedResponse[1])
	mockDao.Mock.On("ValidateParameters", test_handler.MockOrgId, requestBody[2]).Return(expectedResponse[2])

	code, body, err := serveRepositoryParametersRouter(req, &mockDao)

	assert.Nil(t, err)
	assert.Equal(t, 200, code)

	var response []api.RepositoryValidationResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func setHeaders(t *testing.T, req *http.Request) {
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	req.Header.Set("Content-Type", "application/json")
}

func TestRepositoryParameterSuite(t *testing.T) {
	suite.Run(t, new(RepositoryParameterSuite))
}
