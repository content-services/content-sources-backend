package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type AdminLensSuite struct {
	suite.Suite
	reg *dao.MockDaoRegistry
}

func TestAdminLensSuite(t *testing.T) {
	suite.Run(t, new(AdminLensSuite))
}

func (suite *AdminLensSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
}

func (suite *AdminLensSuite) serveAdminLensRouter(req *http.Request, enabled bool, authorized bool) (int, []byte, error) {
	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	config.Get().Features.AdminLensDemandSignals.Enabled = enabled
	if authorized {
		config.Get().Features.AdminLensDemandSignals.Accounts = &[]string{test_handler.MockAccountNumber}
	} else {
		config.Get().Features.AdminLensDemandSignals.Accounts = &[]string{seeds.RandomAccountId()}
	}

	RegisterAdminLensRoutes(pathPrefix, suite.reg.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	response := rr.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *AdminLensSuite) newRequest(query string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/admin/lens/demand_signals/"+query, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))
	return req
}

func (suite *AdminLensSuite) TestListDemandSignals() {
	t := suite.T()
	before := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	after := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	collection := api.AdminDemandSignalCollectionResponse{Data: []api.AdminDemandSignalResponse{
		{Count: 4, Name: "lib", Version: "1.0", Ecosystem: "Java", Namespace: utils.Ptr("org.a")},
	}}
	suite.reg.CoverageReport.On("ListDemandSignals", test.MockCtx(),
		api.PaginationData{Limit: 10, Offset: DefaultOffset},
		api.AdminDemandSignalFilterData{Ecosystem: "Java", BeforeCreatedDate: &before, AfterCreatedDate: &after},
	).Return(collection, int64(1), nil)

	code, body, err := suite.serveAdminLensRouter(
		suite.newRequest("?ecosystem=Java&after_created_date=2026-10-01&before_created_date=2026-11-01&limit=10"), true, true)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.AdminDemandSignalCollectionResponse{}
	require.NoError(t, json.Unmarshal(body, &response))
	require.Len(t, response.Data, 1)
	assert.Equal(t, int64(4), response.Data[0].Count)
	assert.Equal(t, "org.a", *response.Data[0].Namespace)
	assert.Equal(t, int64(1), response.Meta.Count)
	assert.Equal(t, 10, response.Meta.Limit)
}

func (suite *AdminLensSuite) TestListDemandSignalsBadDate() {
	t := suite.T()

	code, body, err := suite.serveAdminLensRouter(suite.newRequest("?before_created_date=2026-13-01"), true, true)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "before_created_date")

	code, body, err = suite.serveAdminLensRouter(suite.newRequest("?after_created_date=11/01/2026"), true, true)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "after_created_date")
}

func (suite *AdminLensSuite) TestListDemandSignalsDisabled() {
	t := suite.T()

	code, body, err := suite.serveAdminLensRouter(suite.newRequest(""), false, true)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Admin lens demand signals feature is disabled")
}

func (suite *AdminLensSuite) TestListDemandSignalsNotAccessible() {
	t := suite.T()

	code, body, err := suite.serveAdminLensRouter(suite.newRequest(""), true, false)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Neither the user nor account is allowed")
}
