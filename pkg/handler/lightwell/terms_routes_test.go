package lightwell

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTermsMiddlewareRoutes(t *testing.T) {
	termsFeature := config.Get().Features.LightwellTerms
	t.Cleanup(func() { config.Get().Features.LightwellTerms = termsFeature })
	config.Get().Features.LightwellTerms = config.Feature{}

	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	group := router.Group(LightwellAPIPath)
	reg := dao.GetMockDaoRegistry(t)
	var fsClient feature_service_client.FeatureServiceClient
	var tsClient terms_service_client.TermsServiceClient
	guard := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return c.String(http.StatusPreconditionRequired, "terms guard invoked")
		}
	}
	RegisterLightwellPackageRoutes(group, reg.ToDaoRegistry(), nil, nil, &fsClient, guard)
	RegisterLightwellAdvisoryRoutes(group, reg.ToDaoRegistry(), &fsClient, guard)
	RegisterLightwellRepositoryRoutes(group, reg.ToDaoRegistry())
	RegisterLightwellTermsRoutes(group, &tsClient, &fsClient)

	for _, path := range []string{
		"/packages",
		"/package_versions",
		"/repositories/repo/packages",
		"/repositories/repo/maven_packages/group/name",
		"/repositories/repo/maven_packages/group/name/version",
		"/repositories/repo/python_packages/name",
		"/repositories/repo/python_packages/name/version",
		"/advisories",
	} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, LightwellAPIPath+path, nil))
			require.Equal(t, http.StatusPreconditionRequired, recorder.Code)
			assert.Equal(t, "terms guard invoked", recorder.Body.String())
		})
	}

	t.Run("repository listing remains unguarded", func(t *testing.T) {
		reg.RepositoryConfig.WithContextMock().On("List", mock.Anything, "", mock.Anything, mock.Anything).
			Return(api.RepositoryCollectionResponse{}, int64(0), nil).Once()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, LightwellAPIPath+"/repositories", nil))
		assert.Equal(t, http.StatusOK, recorder.Code)
	})
	t.Run("terms check remains unguarded", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, LightwellAPIPath+"/terms/required", nil))
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.JSONEq(t, `{"required":false}`, recorder.Body.String())
	})
}
