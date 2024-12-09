package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CandlepinSuite struct {
	suite.Suite
	cpMock    *candlepin_client.MockCandlepinClient
	cacheMock *cache.MockCache
}

func TestCandlepinSuite(t *testing.T) {
	suite.Run(t, new(CandlepinSuite))
}
func (suite *CandlepinSuite) SetupTest() {
	suite.cpMock = candlepin_client.NewMockCandlepinClient(suite.T())
	suite.cacheMock = cache.NewMockCache(suite.T())
}

func (suite *CandlepinSuite) serverCandlepinRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	h := CandlepinHandler{
		cpClient: suite.cpMock,
		cache:    suite.cacheMock,
	}

	RegisterCandlepinRoutes(pathPrefix, &h.cpClient, &h.cache)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *CandlepinSuite) TestSubscriptionCheck() {
	t := suite.T()
	suite.cacheMock.On("GetSubscriptionCheck", test.MockCtx()).Return(nil, nil)
	suite.cacheMock.On("SetSubscriptionCheck", test.MockCtx(), api.SubscriptionCheckResponse{RedHatEnterpriseLinux: true}).Return(nil)

	// Test subscription exists
	expectedProducts := make([]caliri.ProductDTO, 1)
	suite.cpMock.On("ListProducts", test.MockCtx(), test_handler.MockOrgId, RHProductIDs).Return(expectedProducts, nil).Once()

	path := fmt.Sprintf("%s/subscription_check/", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serverCandlepinRouter(req)
	assert.Nil(t, err)

	response := api.SubscriptionCheckResponse{}
	err = json.Unmarshal(body, &response)
	assert.NoError(t, err)
	assert.Equal(t, true, response.RedHatEnterpriseLinux)
	assert.Equal(t, http.StatusOK, code)

	// Test subscription does not exist
	suite.cpMock.On("ListProducts", test.MockCtx(), test_handler.MockOrgId, RHProductIDs).Return(nil, nil).Once()
	code, body, err = suite.serverCandlepinRouter(req)
	assert.Nil(t, err)

	response = api.SubscriptionCheckResponse{}
	err = json.Unmarshal(body, &response)
	assert.NoError(t, err)
	assert.Equal(t, false, response.RedHatEnterpriseLinux)
	assert.Equal(t, http.StatusOK, code)
}
