package handler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func serveRepositoryParametersRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	pathPrefix := router.Group(fullRootPath())

	RegisterRepositoryParameterRoutes(pathPrefix)

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

	path := fmt.Sprintf("%s/repository_parameters/", fullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)

	code, body, err := serveRepositoryParametersRouter(req)
	assert.Nil(t, err)

	response := api.RepositoryParameterResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)

	assert.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, response.DistributionArches)
	assert.NotEmpty(t, response.DistributionVersions)

}

func TestRepositoryParameterSuite(t *testing.T) {
	suite.Run(t, new(RepositoryParameterSuite))
}
