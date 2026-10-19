package lightwell

import (
	"net/http"
	"net/http/httptest"

	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/clients/s3_client"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
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
	tsClient   *terms_service_client.MockTermsServiceClient
	s3Client   *s3_client.MockS3Client
}

func (s *LightwellSuite) SetupTest() {
	s.reg = dao.GetMockDaoRegistry(s.T())
	s.tangClient = tangy.NewMockTangy(s.T())
	s.pulpClient = pulp_client.NewMockPulpClient(s.T())
	s.fsClient = feature_service_client.NewMockFeatureServiceClient(s.T())
	s.tsClient = terms_service_client.NewMockTermsServiceClient(s.T())
	s.s3Client = s3_client.NewMockS3Client(s.T())
}

// serveRouter sets up a router with all lightwell routes registered
func (s *LightwellSuite) serveRouter(req *http.Request) (int, []byte, error) {
	rr := s.record(req)
	return rr.Code, rr.Body.Bytes(), nil
}

func (s *LightwellSuite) record(req *http.Request) *httptest.ResponseRecorder {
	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(LightwellAPIPath)

	var fsClient feature_service_client.FeatureServiceClient = s.fsClient

	RegisterLightwellRepositoryRoutes(pathPrefix, s.reg.ToDaoRegistry())
	RegisterLightwellPackageRoutes(pathPrefix, s.reg.ToDaoRegistry(), s.tangClient, s.pulpClient, &fsClient)
	RegisterLightwellAdvisoryRoutes(pathPrefix, s.reg.ToDaoRegistry(), &fsClient)

	var tsClient terms_service_client.TermsServiceClient = s.tsClient
	var fsClientForTerms feature_service_client.FeatureServiceClient = s.fsClient
	RegisterLightwellTermsRoutes(pathPrefix, &tsClient, &fsClientForTerms)
	RegisterClearinghouseRoutes(pathPrefix, s.reg.ToDaoRegistry().ClearinghouseUpload, s.s3Client)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}
