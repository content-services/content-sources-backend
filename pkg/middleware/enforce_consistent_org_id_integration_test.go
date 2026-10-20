package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type EnforceSuite struct {
	suite.Suite
	reg    *dao.MockDaoRegistry
	tcMock *client.MockTaskClient
	fsMock *feature_service_client.MockFeatureServiceClient
	cpMock *candlepin_client.MockCandlepinClient
}

func TestEnforceSuite(t *testing.T) {
	suite.Run(t, new(EnforceSuite))
}

func (suite *EnforceSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
	suite.tcMock = client.NewMockTaskClient(suite.T())
	suite.fsMock = feature_service_client.NewMockFeatureServiceClient(suite.T())
}

func (suite *EnforceSuite) setupTestHandler() *echo.Echo {
	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.Use(middleware.EnforceConsistentOrgId)
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	rh := handler.RepositoryHandler{
		DaoRegistry:          *suite.reg.ToDaoRegistry(),
		TaskClient:           suite.tcMock,
		FeatureServiceClient: suite.fsMock,
	}

	td := handler.AdminTaskHandler{
		FeatureServiceClient: suite.fsMock,
		CandlepinClient:      suite.cpMock,
	}

	config.Get().Features.AdminTasks.Enabled = true
	config.Get().Features.AdminTasks.Accounts = &[]string{test_handler.MockAccountNumber}

	handler.RegisterRepositoryRoutes(pathPrefix, suite.reg.ToDaoRegistry(), &rh.TaskClient, &rh.FeatureServiceClient)
	handler.RegisterAdminTaskRoutes(pathPrefix, suite.reg.ToDaoRegistry(), &td.FeatureServiceClient, &td.CandlepinClient)

	return router
}

func (suite *EnforceSuite) TestRepositoryListWithValidOrgId() {
	collection := api.RepositoryCollectionResponse{
		Data: []api.RepositoryResponse{
			{
				UUID:  "test-uuid-1",
				Name:  "Test Repo",
				URL:   "http://example.com/repo1",
				OrgID: test_handler.MockOrgId, // Valid org_id matching user
			},
		},
	}
	paginationData := api.PaginationData{Limit: 100, Offset: 0}

	// Mock the database returning repositories with valid org_id
	suite.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.FilterData{}).Return(collection, int64(1), nil)

	router := suite.setupTestHandler()
	path := fmt.Sprintf("%s/repositories/", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
}

func (suite *EnforceSuite) TestRepositoryListWithInvalidOrgId() {
	collection := api.RepositoryCollectionResponse{
		Data: []api.RepositoryResponse{
			{
				UUID:  "test-uuid-1",
				Name:  "Test Repo",
				URL:   "http://example.com/repo1",
				OrgID: test_handler.MockOrgId, // Valid org_id
			},
			// This one should cause an expected failure
			{
				UUID:  "test-uuid-2",
				Name:  "Test Repo 2",
				URL:   "http://example.com/repo2",
				OrgID: "invalid-org-id", // Invalid org_id
			},
		},
	}
	paginationData := api.PaginationData{Limit: 100, Offset: 0}

	// Mock the database returning repositories with mixed org_ids (one invalid)
	suite.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.FilterData{}).Return(collection, int64(2), nil)

	router := suite.setupTestHandler()
	path := fmt.Sprintf("%s/repositories/", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	assert.Equal(suite.T(), http.StatusInternalServerError, response.StatusCode)
}

func (suite *EnforceSuite) TestRepositoryListWithSharedOrgIds() {
	collection := api.RepositoryCollectionResponse{
		Data: []api.RepositoryResponse{
			{
				UUID:  "test-uuid-1",
				Name:  "User Repo",
				URL:   "http://example.com/user-repo",
				OrgID: test_handler.MockOrgId, // User's org_id
			},
			{
				UUID:  "rhel-repo-123",
				Name:  "RHEL Repository",
				URL:   "http://example.com/rhel-repo",
				OrgID: config.RedHatOrg, // RHEL org_id should be allowed
			},
			{
				UUID:  "community-repo-123",
				Name:  "Community Repository",
				URL:   "http://example.com/community-repo",
				OrgID: config.CommunityOrg, // Community org_id should be allowed
			},
		},
	}
	paginationData := api.PaginationData{Limit: 100, Offset: 0}

	// Mock the database returning repositories with mixed valid org_ids
	suite.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.FilterData{}).Return(collection, int64(3), nil)

	router := suite.setupTestHandler()
	path := fmt.Sprintf("%s/repositories/", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
}

func (suite *EnforceSuite) TestRepositoryListWithMissingOrgId() {
	// Create identity with missing org ID
	xrhid := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: test_handler.MockAccountNumber,
			Internal: identity.Internal{
				OrgID: "", // Missing org ID
			},
			User: &identity.User{Username: "user"},
			Type: "Associate",
		},
	}

	router := suite.setupTestHandler()
	path := fmt.Sprintf("%s/repositories/", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(suite.T(), xrhid))

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	assert.Equal(suite.T(), http.StatusBadRequest, response.StatusCode)
}

func (suite *EnforceSuite) TestSkipEndpoints() {
	collection := api.AdminTaskInfoCollectionResponse{
		Data: []api.AdminTaskInfoResponse{
			{
				UUID:  "test-uuid-1",
				OrgId: test_handler.MockOrgId, // Valid org_id
			},
			// This one should cause an expected failure
			{
				UUID:  "test-uuid-2",
				OrgId: "invalid-org-id", // Invalid org_id
			},
		},
	}
	paginationData := api.PaginationData{Limit: 100, Offset: 0}

	// Mock the database returning repositories with mixed org_ids (one invalid)
	suite.reg.AdminTask.On("List", test.MockCtx(), paginationData, api.AdminTaskFilterData{}).Return(collection, int64(2), nil)

	router := suite.setupTestHandler()
	path := fmt.Sprintf("%s/admin/tasks/", api.FullRootPath())
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	// status is 200 because the endpoint is skipped
	assert.Equal(suite.T(), http.StatusOK, response.StatusCode)
}
