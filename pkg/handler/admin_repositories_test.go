package handler

import (
	"bytes"
	"encoding/json"
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
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type AdminRepositoriesSuite struct {
	suite.Suite
	reg *dao.MockDaoRegistry
}

func TestAdminRepositoriesSuite(t *testing.T) {
	suite.Run(t, new(AdminRepositoriesSuite))
}

func (suite *AdminRepositoriesSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
}

func (suite *AdminRepositoriesSuite) serveAdminReposRouter(req *http.Request, enabled bool, authorized bool) (int, []byte, error) {
	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	if enabled {
		config.Get().Features.AdminPartnerRepositories.Enabled = true
	} else {
		config.Get().Features.AdminPartnerRepositories.Enabled = false
	}
	if authorized {
		config.Get().Features.AdminPartnerRepositories.Accounts = &[]string{test_handler.MockAccountNumber}
	} else {
		config.Get().Features.AdminPartnerRepositories.Accounts = &[]string{seeds.RandomAccountId()}
	}

	RegisterAdminRepositoriesRoutes(pathPrefix, suite.reg.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	response := rr.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *AdminRepositoriesSuite) TestTogglePartnerDisabled() {
	t := suite.T()

	repoUUID := uuid.NewString()
	body, err := json.Marshal(api.SetPartnerRepositoryRequest{Partner: utils.Ptr(true)})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, api.FullRootPath()+"/admin/repositories/"+repoUUID+"/partner", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, respBody, err := suite.serveAdminReposRouter(req, false, false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(respBody), "Partner repositories feature is disabled")
}

func (suite *AdminRepositoriesSuite) TestTogglePartnerNotAccessible() {
	t := suite.T()

	repoUUID := uuid.NewString()
	body, err := json.Marshal(api.SetPartnerRepositoryRequest{Partner: utils.Ptr(true)})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, api.FullRootPath()+"/admin/repositories/"+repoUUID+"/partner", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, respBody, err := suite.serveAdminReposRouter(req, true, false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(respBody), "Neither the user nor account is allowed")
}

func (suite *AdminRepositoriesSuite) TestTogglePartnerSuccess() {
	t := suite.T()

	repoUUID := uuid.NewString()
	expectedResponse := api.RepositoryResponse{UUID: repoUUID, Partner: true}

	suite.reg.RepositoryConfig.On("SetPartnerRepo", test.MockCtx(), repoUUID, true).Return(nil)
	suite.reg.RepositoryConfig.On("FetchWithoutOrgID", test.MockCtx(), repoUUID, false).Return(expectedResponse, nil)

	body, err := json.Marshal(api.SetPartnerRepositoryRequest{Partner: utils.Ptr(true)})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, api.FullRootPath()+"/admin/repositories/"+repoUUID+"/partner", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, respBody, err := suite.serveAdminReposRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.RepositoryResponse
	err = json.Unmarshal(respBody, &response)
	assert.NoError(t, err)
	assert.Equal(t, repoUUID, response.UUID)
	assert.True(t, response.Partner)
}

func (suite *AdminRepositoriesSuite) TestTogglePartnerDaoError() {
	t := suite.T()

	repoUUID := uuid.NewString()
	daoErr := &ce.DaoError{BadValidation: true, Message: "Only upload repositories can be marked as partner"}
	suite.reg.RepositoryConfig.On("SetPartnerRepo", test.MockCtx(), repoUUID, true).Return(daoErr)

	body, err := json.Marshal(api.SetPartnerRepositoryRequest{Partner: utils.Ptr(true)})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, api.FullRootPath()+"/admin/repositories/"+repoUUID+"/partner", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, respBody, err := suite.serveAdminReposRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(respBody), "Only upload repositories can be marked as partner")
}

func (suite *AdminRepositoriesSuite) TestTogglePartnerNotFound() {
	t := suite.T()

	repoUUID := uuid.NewString()
	daoErr := &ce.DaoError{NotFound: true, Message: "Not found"}
	suite.reg.RepositoryConfig.On("SetPartnerRepo", test.MockCtx(), repoUUID, true).Return(daoErr)

	body, err := json.Marshal(api.SetPartnerRepositoryRequest{Partner: utils.Ptr(true)})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, api.FullRootPath()+"/admin/repositories/"+repoUUID+"/partner", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveAdminReposRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *AdminRepositoriesSuite) TestAdminTasksDoesNotGrantPartnerAccess() {
	t := suite.T()

	config.Get().Features.AdminTasks.Enabled = true
	config.Get().Features.AdminTasks.Accounts = &[]string{test_handler.MockAccountNumber}

	repoUUID := uuid.NewString()
	body, err := json.Marshal(api.SetPartnerRepositoryRequest{Partner: utils.Ptr(true)})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, api.FullRootPath()+"/admin/repositories/"+repoUUID+"/partner", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, respBody, err := suite.serveAdminReposRouter(req, false, false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(respBody), "Partner repositories feature is disabled")
}
