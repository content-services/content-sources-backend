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
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type UserPreferencesSuite struct {
	suite.Suite
	reg *dao.MockDaoRegistry
}

func TestUserPreferencesSuite(t *testing.T) {
	suite.Run(t, new(UserPreferencesSuite))
}

func (suite *UserPreferencesSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
}

func (suite *UserPreferencesSuite) serveRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())
	RegisterUserPreferencesRoutes(pathPrefix, suite.reg.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *UserPreferencesSuite) TestList() {
	orgID := test_handler.MockOrgId
	userID := "user"
	expected := api.UserPreferencesResponse{
		{Label: models.UserPreferenceLightwellNotificationEnabled, Value: "true"},
		{Label: models.UserPreferenceLightwellNotificationMinimum, Value: "critical"},
	}

	suite.reg.UserPreference.On("List", test.MockCtx(), orgID, userID).Return(expected, nil)

	path := fmt.Sprintf("%s/user_preferences/", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	code, body, err := suite.serveRouter(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)

	var resp api.UserPreferencesResponse
	assert.NoError(suite.T(), json.Unmarshal(body, &resp))
	assert.Equal(suite.T(), expected, resp)
}

func (suite *UserPreferencesSuite) TestSet() {
	orgID := test_handler.MockOrgId
	userID := "user"
	label := models.UserPreferenceLightwellNotificationEnabled
	expected := api.UserPreferenceResponse{Label: label, Value: "true"}

	suite.reg.UserPreference.On("Set", test.MockCtx(), orgID, userID, label, "true").Return(expected, nil)

	path := fmt.Sprintf("%s/user_preferences/%s", api.FullRootPath(), label)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader([]byte(`"true"`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	code, body, err := suite.serveRouter(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)

	var resp api.UserPreferenceResponse
	assert.NoError(suite.T(), json.Unmarshal(body, &resp))
	assert.Equal(suite.T(), expected, resp)
}

func (suite *UserPreferencesSuite) TestSetInvalidLabel() {
	orgID := test_handler.MockOrgId
	userID := "user"
	label := "not-valid"

	suite.reg.UserPreference.On("Set", test.MockCtx(), orgID, userID, label, "true").
		Return(api.UserPreferenceResponse{}, &ce.DaoError{BadValidation: true, Message: "invalid preference label: not-valid"})

	path := fmt.Sprintf("%s/user_preferences/%s", api.FullRootPath(), label)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader([]byte(`"true"`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	code, _, err := suite.serveRouter(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, code)
}

func (suite *UserPreferencesSuite) TestSetInvalidBody() {
	label := models.UserPreferenceLightwellNotificationEnabled
	path := fmt.Sprintf("%s/user_preferences/%s", api.FullRootPath(), label)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader([]byte(`{"value":"true"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	code, _, err := suite.serveRouter(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, code)
}
