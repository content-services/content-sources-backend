package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	fsc "github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type LightwellTokensSuite struct {
	suite.Suite
	reg      *dao.MockDaoRegistry
	fsClient *fsc.MockFeatureServiceClient
}

func TestLightwellTokensSuite(t *testing.T) {
	suite.Run(t, new(LightwellTokensSuite))
}

func (suite *LightwellTokensSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
	suite.fsClient = fsc.NewMockFeatureServiceClient(suite.T())
	config.Get().Features.Lightwell.Enabled = true
	config.Get().Options.LightwellValidateSecret = "test-validate-secret"
	config.Get().Options.FeatureFilter = []string{config.LightwellNetworkFeature}
	config.Get().Options.EntitleAll = true
}

func (suite *LightwellTokensSuite) serveRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.Use(middleware.LightwellBearerAuth(suite.reg.ToDaoRegistry(), suite.fsClient))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())
	RegisterLightwellTokenRoutes(pathPrefix, suite.reg.ToDaoRegistry(), suite.fsClient)
	RegisterLightwellInternalRoutes(router, suite.reg.ToDaoRegistry(), suite.fsClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	response := rr.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func orgAdminIdentity(t *testing.T) string {
	id := test_handler.MockIdentity
	id.Identity.User = &identity.User{
		Username: "admin",
		UserID:   test_handler.MockUserID,
		OrgAdmin: true,
	}
	return test_handler.EncodedCustomIdentity(t, id)
}

func nonAdminIdentity(t *testing.T) string {
	return test_handler.EncodedIdentity(t)
}

func (suite *LightwellTokensSuite) TestCreateRequiresOrgAdmin() {
	path := fmt.Sprintf("%s/tokens/", api.FullRootPath())
	body, _ := json.Marshal(api.LightwellTokenCreateRequest{Name: "t1", AccessLevel: config.LightwellAccessValidated})
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, nonAdminIdentity(suite.T()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	code, _, err := suite.serveRouter(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusForbidden, code)
}

func (suite *LightwellTokensSuite) TestCreateSuccess() {
	orgID := test_handler.MockOrgId
	userID := test_handler.MockUserID
	expires := time.Now().UTC().Add(24 * time.Hour)
	expected := api.LightwellTokenResponse{
		UUID:        "tok-uuid",
		OrgID:       orgID,
		UserID:      userID,
		Name:        "ci",
		AccessLevel: config.LightwellAccessValidated,
		TokenPrefix: "lw_abcdef12",
		Token:       "lw_abcdef12plaintext",
		ExpiresAt:   expires,
		CreatedAt:   time.Now().UTC(),
	}

	suite.fsClient.On("GetEntitledFeatures", mock.Anything, orgID).Return([]string{config.LightwellNetworkFeature}, nil)
	suite.reg.LightwellToken.On("Create", mock.Anything, orgID, userID, "ci", config.LightwellAccessValidated, (*time.Time)(nil)).Return(expected, nil)

	path := fmt.Sprintf("%s/tokens/", api.FullRootPath())
	body, _ := json.Marshal(api.LightwellTokenCreateRequest{Name: "ci", AccessLevel: config.LightwellAccessValidated})
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, orgAdminIdentity(suite.T()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	code, respBody, err := suite.serveRouter(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusCreated, code)

	var resp api.LightwellTokenResponse
	assert.NoError(suite.T(), json.Unmarshal(respBody, &resp))
	assert.Equal(suite.T(), expected.Token, resp.Token)
	assert.Equal(suite.T(), expected.UUID, resp.UUID)
	assert.Equal(suite.T(), config.LightwellAccessValidated, resp.AccessLevel)
}

func (suite *LightwellTokensSuite) TestCreateMissingEntitlement() {
	orgID := test_handler.MockOrgId
	suite.fsClient.On("GetEntitledFeatures", mock.Anything, orgID).Return([]string{"RHEL-OS-x86_64"}, nil)

	path := fmt.Sprintf("%s/tokens/", api.FullRootPath())
	body, _ := json.Marshal(api.LightwellTokenCreateRequest{Name: "ci", AccessLevel: config.LightwellAccessValidated})
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, orgAdminIdentity(suite.T()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	code, _, err := suite.serveRouter(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusForbidden, code)
}

func (suite *LightwellTokensSuite) TestValidateInternal() {
	orgID := test_handler.MockOrgId
	suite.reg.LightwellToken.On("Validate", mock.Anything, "lw_good").Return(api.LightwellTokenValidateResponse{
		OrgID: orgID, UserID: "u1", TokenUUID: "tok-1", AccessLevel: config.LightwellAccessValidated,
	}, nil)
	suite.fsClient.On("GetEntitledFeatures", mock.Anything, orgID).Return([]string{config.LightwellNetworkFeature}, nil)

	body, _ := json.Marshal(api.LightwellTokenValidateRequest{Token: "lw_good"})
	req := httptest.NewRequest(http.MethodPost, LightwellInternalValidatePath, bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(lightwellInternalValidateHeader, "test-validate-secret")

	code, respBody, err := suite.serveRouter(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)

	var resp api.LightwellTokenValidateResponse
	assert.NoError(suite.T(), json.Unmarshal(respBody, &resp))
	assert.Equal(suite.T(), orgID, resp.OrgID)
	assert.Equal(suite.T(), config.LightwellAccessValidated, resp.AccessLevel)
}

func (suite *LightwellTokensSuite) TestValidatePathAccessAllowed() {
	orgID := test_handler.MockOrgId
	suite.reg.LightwellToken.On("Validate", mock.Anything, "lw_rem").Return(api.LightwellTokenValidateResponse{
		OrgID: orgID, UserID: "u1", TokenUUID: "tok-1", AccessLevel: config.LightwellAccessRemediated,
	}, nil)
	suite.fsClient.On("GetEntitledFeatures", mock.Anything, orgID).Return([]string{config.LightwellNetworkFeature}, nil)

	body, _ := json.Marshal(api.LightwellTokenValidateRequest{
		Token: "lw_rem",
		Path:  "/api/pulp-content/lightwell/java/remediated/pkg.pom",
	})
	req := httptest.NewRequest(http.MethodPost, LightwellInternalValidatePath, bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(lightwellInternalValidateHeader, "test-validate-secret")

	code, _, err := suite.serveRouter(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)
}

func (suite *LightwellTokensSuite) TestValidatePathAccessDenied() {
	orgID := test_handler.MockOrgId
	suite.reg.LightwellToken.On("Validate", mock.Anything, "lw_rem").Return(api.LightwellTokenValidateResponse{
		OrgID: orgID, UserID: "u1", TokenUUID: "tok-1", AccessLevel: config.LightwellAccessRemediated,
	}, nil)
	suite.fsClient.On("GetEntitledFeatures", mock.Anything, orgID).Return([]string{config.LightwellNetworkFeature}, nil)

	body, _ := json.Marshal(api.LightwellTokenValidateRequest{
		Token: "lw_rem",
		Path:  "/api/pulp-content/lightwell/java/validated/pkg.pom",
	})
	req := httptest.NewRequest(http.MethodPost, LightwellInternalValidatePath, bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(lightwellInternalValidateHeader, "test-validate-secret")

	code, _, err := suite.serveRouter(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusForbidden, code)
}

func (suite *LightwellTokensSuite) TestValidateMissingSecret() {
	body, _ := json.Marshal(api.LightwellTokenValidateRequest{Token: "lw_good"})
	req := httptest.NewRequest(http.MethodPost, LightwellInternalValidatePath, bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	code, _, err := suite.serveRouter(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusUnauthorized, code)
}

func (suite *LightwellTokensSuite) TestBearerRejectedOnTokenCreate() {
	path := fmt.Sprintf("%s/tokens/", api.FullRootPath())
	body, _ := json.Marshal(api.LightwellTokenCreateRequest{Name: "ci", AccessLevel: config.LightwellAccessValidated})
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, "Bearer lw_some-token")

	code, _, err := suite.serveRouter(req)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusUnauthorized, code)
}
