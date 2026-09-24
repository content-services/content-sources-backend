package lightwell

import (
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/suite"
)

// LightwellSuite is a shared base suite for all lightwell handler tests
type LightwellSuite struct {
	suite.Suite
	reg        *dao.MockDaoRegistry
	tangClient *tangy.MockTangy
	pulpClient *pulp_client.MockPulpClient
	fsClient   *feature_service_client.MockFeatureServiceClient
}

func (s *LightwellSuite) SetupTest() {
	s.reg = dao.GetMockDaoRegistry(s.T())
	s.tangClient = tangy.NewMockTangy(s.T())
	s.pulpClient = pulp_client.NewMockPulpClient(s.T())
	s.fsClient = feature_service_client.NewMockFeatureServiceClient(s.T())
}

// serveRouter sets up a router with all lightwell routes registered
func (s *LightwellSuite) serveRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(LightwellAPIPath)

	RegisterLightwellRepositoryRoutes(pathPrefix, s.reg.ToDaoRegistry())
	RegisterLightwellPackageRoutes(pathPrefix, s.reg.ToDaoRegistry(), s.tangClient, s.pulpClient)

	var fsClient feature_service_client.FeatureServiceClient = s.fsClient
	RegisterLightwellAdvisoryRoutes(pathPrefix, s.reg.ToDaoRegistry(), &fsClient)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}
