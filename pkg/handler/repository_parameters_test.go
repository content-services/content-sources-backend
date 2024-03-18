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
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/openlyinc/pointy"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type RepositoryParameterSuite struct {
	suite.Suite
	mockDao *dao.MockDaoRegistry
}

func TestRepositoryParameterSuite(t *testing.T) {
	suite.Run(t, new(RepositoryParameterSuite))
}

func (s *RepositoryParameterSuite) SetupTest() {
	s.mockDao = dao.GetMockDaoRegistry(s.T())
}

func (s *RepositoryParameterSuite) serveRepositoryParametersRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))
	pathPrefix := router.Group(api.FullRootPath())

	RegisterRepositoryParameterRoutes(pathPrefix, s.mockDao.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (s *RepositoryParameterSuite) TestListParams() {
	t := s.T()
	path := fmt.Sprintf("%s/repository_parameters/", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	setHeaders(t, req)
	code, body, err := s.serveRepositoryParametersRouter(req)

	assert.Nil(t, err)

	response := api.RepositoryParameterResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, response.DistributionArches)
	assert.NotEmpty(t, response.DistributionVersions)
}

func (s *RepositoryParameterSuite) TestValidate() {
	t := s.T()

	path := fmt.Sprintf("%s/repository_parameters/validate/", api.FullRootPath())

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

	s.mockDao.RepositoryConfig.Mock.On("ValidateParameters", test.MockCtx(), test_handler.MockOrgId, requestBody[0], []string{"steve-the-id", "paul-the-id"}).Return(expectedResponse[0], nil)
	s.mockDao.RepositoryConfig.Mock.On("ValidateParameters", test.MockCtx(), test_handler.MockOrgId, requestBody[1], []string{"steve-the-id", "paul-the-id"}).Return(expectedResponse[1], nil)
	s.mockDao.RepositoryConfig.Mock.On("ValidateParameters", test.MockCtx(), test_handler.MockOrgId, requestBody[2], []string{"steve-the-id", "paul-the-id"}).Return(expectedResponse[2], nil)

	code, body, err := s.serveRepositoryParametersRouter(req)

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
