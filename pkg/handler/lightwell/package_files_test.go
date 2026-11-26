package lightwell

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const pythonFilesRoutePattern = "/repositories/:uuid/python_packages/:name/:version/files"

func pythonFilesRoutePrefixes() []string {
	return []string{LightwellAPIPath, api.FullRootPath(), api.MajorRootPath()}
}

func registerPythonFilesTestRoutes(router *echo.Echo, reg *dao.MockDaoRegistry, pulpClient pulp_client.PulpClient) {
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	var fsClient feature_service_client.FeatureServiceClient
	RegisterLightwellPackageRoutes(router.Group(LightwellAPIPath), reg.ToDaoRegistry(), nil, pulpClient, &fsClient)
	for _, prefix := range []string{api.FullRootPath(), api.MajorRootPath()} {
		handler.RegisterPackageRoutes(router.Group(prefix), reg.ToDaoRegistry(), nil, pulpClient)
	}
}

func TestPythonPackageFilesRouteParity(t *testing.T) {
	const repoUUID = "550e8400-e29b-41d4-a716-446655440005"
	var expectedData json.RawMessage
	var expectedMeta api.ResponseMetadata
	for _, prefix := range pythonFilesRoutePrefixes() {
		for _, tc := range []struct {
			name         string
			packageName  string
			versionPath  string
			version      string
			usingRawPath bool
		}{
			{"literal plus", "Lightwell__Fixture", "3.0.1+rhlw.2", "3.0.1+rhlw.2", false},
			{"escaped plus", "Lightwell__Fixture", "3.0.1%2Brhlw.2", "3.0.1+rhlw.2", true},
			{"escaped percent", "Lightwell__Fixture", "3.0.1%252Brhlw.2", "3.0.1%2Brhlw.2", false},
			{"escaped percent and name", "%4Cightwell__Fixture", "3.0.1%252Brhlw.2", "3.0.1%2Brhlw.2", true},
		} {
			t.Run(prefix+"/"+tc.name, func(t *testing.T) {
				suffix := "/repositories/" + repoUUID + "/python_packages/" + tc.packageName + "/" + tc.versionPath + "/files"
				reg := dao.GetMockDaoRegistry(t)
				pulpClient := pulp_client.NewMockPulpClient(t)
				repo := api.RepositoryResponse{
					UUID: repoUUID, OrgID: config.LightwellOrg, Origin: config.OriginLightwell,
					ContentType: config.ContentTypePython, PublishedDistBasePath: "python/remediated",
				}
				reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, api.PaginationData{Limit: 1}, api.FilterData{UUID: repoUUID}).
					Return(api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{repo}}, int64(1), nil).Once()
				reg.Domain.On("Fetch", test.MockCtx(), config.LightwellOrg).Return("lightwell", nil).Once()
				pulpClient.On("WithDomain", "lightwell").Return(pulpClient).Once()
				pulpClient.On("ListPythonPackageFiles", test.MockCtx(), "python/remediated", "lightwell-fixture", tc.version).
					Return([]zest.PythonPythonPackageContentResponse{
						{Filename: zest.PtrString("a.whl"), Packagetype: zest.PtrString("bdist_wheel")},
						{Filename: zest.PtrString("b.whl"), Packagetype: zest.PtrString("bdist_wheel")},
						{Filename: zest.PtrString("c.tar.gz"), Packagetype: zest.PtrString("sdist")},
					}, nil).Once()
				router := echo.New()
				registerPythonFilesTestRoutes(router, reg, pulpClient)
				resource, verb, err := rbac.ServicePermissions.Permission(http.MethodGet, pythonFilesRoutePattern)
				require.NoError(t, err)
				assert.Equal(t, rbac.ResourceRepositories, resource)
				assert.Equal(t, rbac.RbacVerbRead, verb)

				req := httptest.NewRequest(http.MethodGet, prefix+suffix+"?limit=1&offset=1&sort_by=filename%3Adesc", nil)
				assert.Equal(t, tc.usingRawPath, req.URL.RawPath != "", "exercise both Echo route path representations")
				req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
				rr := httptest.NewRecorder()
				router.ServeHTTP(rr, req)
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
				var response struct {
					Data  json.RawMessage      `json:"data"`
					Meta  api.ResponseMetadata `json:"meta"`
					Links api.Links            `json:"links"`
				}
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
				assert.JSONEq(t, `[{"filename":"b.whl","category":"package","packagetype":"bdist_wheel","size":null,"sha256":null}]`, string(response.Data))
				assert.Equal(t, api.ResponseMetadata{Count: 3, Limit: 1, Offset: 1}, response.Meta)
				if expectedData == nil {
					expectedData, expectedMeta = response.Data, response.Meta
				} else {
					assert.JSONEq(t, string(expectedData), string(response.Data))
					assert.Equal(t, expectedMeta, response.Meta)
				}
				for _, link := range []struct {
					value  string
					offset string
				}{
					{response.Links.First, "0"}, {response.Links.Prev, "0"},
					{response.Links.Next, "2"}, {response.Links.Last, "2"},
				} {
					parsed, err := url.Parse(link.value)
					require.NoError(t, err)
					assert.Equal(t, prefix+suffix, parsed.EscapedPath())
					assert.Equal(t, "filename:desc", parsed.Query().Get("sort_by"))
					assert.Equal(t, "1", parsed.Query().Get("limit"))
					assert.Equal(t, link.offset, parsed.Query().Get("offset"))
				}
			})
		}
	}
}

func TestPythonPackageFilesRouteAccessDenials(t *testing.T) {
	kessel := config.Get().Features.Kessel
	t.Cleanup(func() { config.Get().Features.Kessel = kessel })
	config.Get().Features.Kessel = config.Feature{}
	for _, prefix := range pythonFilesRoutePrefixes() {
		for _, hasIdentity := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/identity=%t", prefix, hasIdentity), func(t *testing.T) {
				reg := dao.GetMockDaoRegistry(t)
				pulpClient := pulp_client.NewMockPulpClient(t)
				router := echo.New()
				registerPythonFilesTestRoutes(router, reg, pulpClient)
				if hasIdentity {
					rbacClient := rbac.NewMockClientWrapper(t)
					rbacClient.On("Allowed", mock.Anything, rbac.ResourceRepositories, rbac.RbacVerbRead).
						Return(false, nil).Once()
					router.Use(middleware.NewRbac(middleware.Rbac{
						RbacClient: rbacClient, PermissionsMap: rbac.ServicePermissions,
					}))
				}
				req := httptest.NewRequest(http.MethodGet, prefix+"/repositories/550e8400-e29b-41d4-a716-446655440005/python_packages/fixture/3.0.1%2Brhlw.2/files", nil)
				if hasIdentity {
					req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
				}
				rr := httptest.NewRecorder()
				router.ServeHTTP(rr, req)
				expectedStatus := http.StatusBadRequest // identity middleware's missing-header response
				if hasIdentity {
					expectedStatus = http.StatusUnauthorized
				}
				assert.Equal(t, expectedStatus, rr.Code, rr.Body.String())
				reg.RepositoryConfig.AssertNotCalled(t, "List", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				pulpClient.AssertNotCalled(t, "ListPythonPackageFiles", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			})
		}
	}
}
