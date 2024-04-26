package rbac

import (
	"context"
	"net/http"
	"testing"

	"github.com/RedHatInsights/rbac-client-go"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/config"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	mocks_rbac "github.com/content-services/content-sources-backend/pkg/test/mocks/rbac"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type RbacTestSuite struct {
	suite.Suite
	echo      *echo.Echo
	rbac      ClientWrapperImpl
	mockCache *cache.MockCache
}

func (s *RbacTestSuite) SetupTest() {
	// Start the mock rbac service
	config.Get().Clients.RbacEnabled = true
	config.Get().Clients.RbacBaseUrl = "http://localhost:9932"
	config.Get().Mocks.Rbac.UserReadWrite = []string{"foo"}

	s.echo = echo.New()
	s.echo.HideBanner = true
	s.echo.Add(echo.GET, mocks_rbac.RbacV1Access, mocks_rbac.MockRbac)
	go func() {
		err := s.echo.Start(":9932")
		assert.True(s.T(), err == http.ErrServerClosed, "Unexpected error %v", err)
	}()
	s.mockCache = cache.NewMockCache(s.T())
	// Configure the client to use the mock rbac service
	//   manually create an ClientWrapperImpl so we can pass in our mock cache
	s.rbac = ClientWrapperImpl{
		client:  rbac.NewClient("http://localhost:9932", application),
		timeout: 0,
		cache:   s.mockCache,
	}
}

func (s *RbacTestSuite) TearDownTest() {
	err := s.echo.Shutdown(context.Background())
	assert.NoError(s.T(), err)
}

func TestRbacSuite(t *testing.T) {
	r := RbacTestSuite{}
	suite.Run(t, &r)
}

func (s *RbacTestSuite) TestCachesWhenNotFound() {
	ctx := context.Background()
	ctx = identity.WithIdentity(ctx, test_handler.MockIdentity)
	var emptyList rbac.AccessList
	s.mockCache.On("GetAccessList", ctx).Return(nil, cache.NotFound)
	s.mockCache.On("SetAccessList", ctx, emptyList).Return(nil, cache.NotFound)
	allowed, err := s.rbac.Allowed(ctx, "repositories", "read")
	assert.NoError(s.T(), err)
	assert.False(s.T(), allowed)
}

func (s *RbacTestSuite) TestCachesWhenNotFoundAgain() {
	ctx := context.Background()
	ctx = identity.WithIdentity(ctx, test_handler.MockIdentity)
	var emptyList rbac.AccessList

	s.mockCache.On("GetAccessList", ctx).Return(emptyList, nil)
	allowed, err := s.rbac.Allowed(ctx, "repositories", "read")
	assert.NoError(s.T(), err)
	assert.False(s.T(), allowed)
}

func (s *RbacTestSuite) TestOrgAdminSkip() {
	ctx := context.Background()
	mockIdentity := test_handler.MockIdentity
	mockIdentity.Identity.User = &identity.User{
		OrgAdmin: true,
	}

	ctx = identity.WithIdentity(ctx, mockIdentity)

	allowed, err := s.rbac.Allowed(ctx, "repositories", "read")
	assert.NoError(s.T(), err)
	assert.True(s.T(), allowed)
}
