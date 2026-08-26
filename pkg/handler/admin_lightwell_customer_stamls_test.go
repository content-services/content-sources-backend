package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type AdminLightwellCustomerStamlSuite struct {
	suite.Suite
	reg *dao.MockDaoRegistry
}

func TestAdminLightwellCustomerStamlSuite(t *testing.T) {
	suite.Run(t, new(AdminLightwellCustomerStamlSuite))
}

func (suite *AdminLightwellCustomerStamlSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
}

func (suite *AdminLightwellCustomerStamlSuite) serveRouter(req *http.Request, enabled bool, authorized bool) (int, []byte, error) {
	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	if enabled {
		config.Get().Features.AdminLightwell.Enabled = true
	} else {
		config.Get().Features.AdminLightwell.Enabled = false
	}
	if authorized {
		config.Get().Features.AdminLightwell.Accounts = &[]string{test_handler.MockAccountNumber}
	} else {
		config.Get().Features.AdminLightwell.Accounts = &[]string{seeds.RandomAccountId()}
	}

	RegisterAdminLightwellCustomerStamlRoutes(pathPrefix, suite.reg.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	response := rr.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *AdminLightwellCustomerStamlSuite) newJSONRequest(method string, payload api.LightwellCustomerStamlRequest) *http.Request {
	t := suite.T()
	body, err := json.Marshal(payload)
	assert.NoError(t, err)
	req := httptest.NewRequest(method, api.FullRootPath()+"/admin/lightwell/customer-stamls/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	return req
}

func (suite *AdminLightwellCustomerStamlSuite) TestCreateDisabled() {
	req := suite.newJSONRequest(http.MethodPost, api.LightwellCustomerStamlRequest{CustomerID: "cid-1", Staml: "staml-1"})
	code, respBody, err := suite.serveRouter(req, false, false)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, code)
	assert.Contains(suite.T(), string(respBody), "Admin Lightwell feature is disabled")
}

func (suite *AdminLightwellCustomerStamlSuite) TestCreateNotAccessible() {
	req := suite.newJSONRequest(http.MethodPost, api.LightwellCustomerStamlRequest{CustomerID: "cid-1", Staml: "staml-1"})
	code, respBody, err := suite.serveRouter(req, true, false)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, code)
	assert.Contains(suite.T(), string(respBody), "Neither the user nor account is allowed")
}

func (suite *AdminLightwellCustomerStamlSuite) TestCreateMissingFields() {
	req := suite.newJSONRequest(http.MethodPost, api.LightwellCustomerStamlRequest{CustomerID: "  ", Staml: ""})
	code, respBody, err := suite.serveRouter(req, true, true)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, code)
	assert.Contains(suite.T(), string(respBody), "customer_id and staml are required")
}

func (suite *AdminLightwellCustomerStamlSuite) TestCreateSuccess() {
	expected := api.LightwellCustomerStamlResponse{
		CustomerID: "cid-1",
		Staml:      "staml-1",
		CreatedAt:  time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
	}
	suite.reg.LightwellCustomerStaml.On("Create", test.MockCtx(), "cid-1", "staml-1").Return(expected, nil)

	req := suite.newJSONRequest(http.MethodPost, api.LightwellCustomerStamlRequest{CustomerID: " cid-1 ", Staml: " staml-1 "})
	code, respBody, err := suite.serveRouter(req, true, true)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusCreated, code)

	var resp api.LightwellCustomerStamlResponse
	assert.NoError(suite.T(), json.Unmarshal(respBody, &resp))
	assert.Equal(suite.T(), expected, resp)
}

func (suite *AdminLightwellCustomerStamlSuite) TestCreateAlreadyExists() {
	suite.reg.LightwellCustomerStaml.On("Create", test.MockCtx(), "cid-1", "staml-1").
		Return(api.LightwellCustomerStamlResponse{}, &ce.DaoError{AlreadyExists: true, Message: "STAML to CID mapping already exists"})

	req := suite.newJSONRequest(http.MethodPost, api.LightwellCustomerStamlRequest{CustomerID: "cid-1", Staml: "staml-1"})
	code, respBody, err := suite.serveRouter(req, true, true)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusConflict, code)
	assert.Contains(suite.T(), string(respBody), "STAML to CID mapping already exists")
}

func (suite *AdminLightwellCustomerStamlSuite) TestDeleteSuccess() {
	suite.reg.LightwellCustomerStaml.On("Delete", test.MockCtx(), "cid-1", "staml-1").Return(nil)

	req := suite.newJSONRequest(http.MethodDelete, api.LightwellCustomerStamlRequest{CustomerID: "cid-1", Staml: "staml-1"})
	code, _, err := suite.serveRouter(req, true, true)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusNoContent, code)
}

func (suite *AdminLightwellCustomerStamlSuite) TestDeleteNotFound() {
	suite.reg.LightwellCustomerStaml.On("Delete", test.MockCtx(), "cid-1", "staml-1").
		Return(&ce.DaoError{NotFound: true, Message: "STAML to CID mapping not found"})

	req := suite.newJSONRequest(http.MethodDelete, api.LightwellCustomerStamlRequest{CustomerID: "cid-1", Staml: "staml-1"})
	code, _, err := suite.serveRouter(req, true, true)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusNotFound, code)
}

func (suite *AdminLightwellCustomerStamlSuite) TestDeleteDisabled() {
	req := suite.newJSONRequest(http.MethodDelete, api.LightwellCustomerStamlRequest{CustomerID: "cid-1", Staml: "staml-1"})
	code, respBody, err := suite.serveRouter(req, false, false)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, code)
	assert.Contains(suite.T(), string(respBody), "Admin Lightwell feature is disabled")
}
