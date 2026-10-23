package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTermsMiddlewareRoutes(t *testing.T) {
	features := config.Get().Features
	t.Cleanup(func() { config.Get().Features = features })
	config.Get().Features.LightwellLens = config.Feature{Enabled: true}
	config.Get().Features.LightwellBeacon = config.Feature{Enabled: true}
	config.Get().Features.LightwellTerms = config.Feature{}

	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	group := router.Group(api.FullRootPath())
	reg := dao.GetMockDaoRegistry(t)
	var fsClient feature_service_client.FeatureServiceClient
	var tsClient terms_service_client.TermsServiceClient
	var taskClient client.TaskClient
	guard := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return c.String(http.StatusPreconditionRequired, "terms guard invoked")
		}
	}

	RegisterPackageRoutes(group, reg.ToDaoRegistry(), nil, nil, guard)
	RegisterLightwellPackageRoutes(group, reg.ToDaoRegistry(), nil, nil, &fsClient, guard)
	RegisterCoverageReportRoutes(group, reg.ToDaoRegistry(), &taskClient, nil, &fsClient, guard)
	RegisterLightwellVulnerabilityRoutes(group, reg.ToDaoRegistry(), guard)
	RegisterLightwellAdvisoryRoutes(group, reg.ToDaoRegistry(), &fsClient, guard)
	RegisterRepositoryRoutes(group, reg.ToDaoRegistry(), &taskClient, &fsClient)
	RegisterLightwellTermsRoutes(group, &tsClient, &fsClient)

	routes := []struct {
		method string
		path   string
		access *config.Feature
	}{
		{http.MethodGet, "/repositories/repo/packages", nil},
		{http.MethodGet, "/repositories/repo/java_packages/group/name", nil},
		{http.MethodGet, "/repositories/repo/java_packages/group/name/version", nil},
		{http.MethodGet, "/repositories/repo/maven_packages/group/name", nil},
		{http.MethodGet, "/repositories/repo/maven_packages/group/name/version", nil},
		{http.MethodGet, "/repositories/repo/python_packages/name", nil},
		{http.MethodGet, "/repositories/repo/python_packages/name/version", nil},
		{http.MethodGet, "/repositories/repo/js_packages/scope/name", nil},
		{http.MethodGet, "/repositories/repo/js_packages/scope/name/version", nil},
		{http.MethodGet, "/lightwell/packages", nil},
		{http.MethodGet, "/lightwell/package_versions", nil},
		{http.MethodGet, "/lightwell/advisories", nil},
		{http.MethodPost, "/coverage_reports/", &config.Get().Features.LightwellLens},
		{http.MethodGet, "/coverage_reports/report", &config.Get().Features.LightwellLens},
		{http.MethodGet, "/coverage_reports/report/packages", &config.Get().Features.LightwellLens},
		{http.MethodGet, "/lightwell/beacon/status/", &config.Get().Features.LightwellBeacon},
		{http.MethodGet, "/lightwell/beacon/vulnerabilities/customers/", &config.Get().Features.LightwellBeacon},
		{http.MethodGet, "/lightwell/beacon/vulnerabilities/ltwlsupt-ticket-ids/", &config.Get().Features.LightwellBeacon},
		{http.MethodGet, "/lightwell/beacon/vulnerabilities/", &config.Get().Features.LightwellBeacon},
	}
	for _, route := range routes {
		t.Run(route.method+route.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(route.method, api.FullRootPath()+route.path, nil))
			require.Equal(t, http.StatusPreconditionRequired, recorder.Code)
			assert.Equal(t, "terms guard invoked", recorder.Body.String())
			if strings.Contains(route.path, "/maven_packages/") {
				successor := strings.Replace(api.FullRootPath()+route.path, "/maven_packages/", "/java_packages/", 1)
				assert.Equal(t, "true", recorder.Header().Get("Deprecation"))
				assert.Equal(t, "<"+successor+">; rel=\"successor-version\"", recorder.Header().Get("Link"))
			}
			if strings.Contains(route.path, "/java_packages/") {
				assert.Empty(t, recorder.Header().Get("Deprecation"))
			}

			if route.access != nil {
				route.access.Enabled = false
				t.Cleanup(func() { route.access.Enabled = true })
				recorder = httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(route.method, api.FullRootPath()+route.path, nil))
				assert.Equal(t, http.StatusBadRequest, recorder.Code)
				assert.NotContains(t, recorder.Body.String(), "terms guard invoked")
			}
		})
	}

	t.Run("repository listing remains unguarded", func(t *testing.T) {
		reg.RepositoryConfig.WithContextMock().On("List", mock.Anything, "", mock.Anything, mock.Anything).
			Return(api.RepositoryCollectionResponse{}, int64(0), nil).Once()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/repositories/", nil))
		assert.Equal(t, http.StatusOK, recorder.Code)
	})
	t.Run("terms check remains unguarded", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, api.FullRootPath()+"/lightwell/terms/required", nil))
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.JSONEq(t, `{"required":false}`, recorder.Body.String())
	})
}
