package rbac

import (
	"context"
	"net"
	"testing"
	"time"

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
	baseURL   string
	rbac      ClientWrapperImpl
	mockCache *cache.MockCache
}

func (s *RbacTestSuite) SetupSuite() {
	// Start the mock rbac service once for the suite to avoid fixed-port races
	// between tests (EADDRINUSE) and asserts from Start's goroutine after teardown.
	config.Get().Clients.RbacEnabled = true
	config.Get().Mocks.Rbac.UserReadWrite = []string{"foo"}

	s.echo = echo.New()
	s.echo.HideBanner = true
	s.echo.Add(echo.GET, mocks_rbac.RbacV1Access, mocks_rbac.MockRbacV1Access)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	s.baseURL = "http://" + ln.Addr().String()
	config.Get().Clients.RbacBaseUrl = s.baseURL
	s.echo.Listener = ln

	go func() {
		// Ignore ErrServerClosed from Shutdown; do not assert here — Go panics if
		// the test has already finished when Start returns.
		_ = s.echo.Start("")
	}()

	s.Require().Eventually(func() bool {
		conn, err := net.DialTimeout("tcp", ln.Addr().String(), 50*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 2*time.Second, 10*time.Millisecond)
}

func (s *RbacTestSuite) TearDownSuite() {
	err := s.echo.Shutdown(context.Background())
	s.Require().NoError(err)
}

func (s *RbacTestSuite) SetupTest() {
	s.mockCache = cache.NewMockCache(s.T())
	s.rbac = ClientWrapperImpl{
		client:  rbac.NewClient(s.baseURL, application),
		timeout: 0,
		cache:   s.mockCache,
	}
}

func TestRbacSuite(t *testing.T) {
	r := RbacTestSuite{}
	suite.Run(t, &r)
}

func (s *RbacTestSuite) TestCachesWhenNotFound() {
	ctx := context.Background()
	ctx = identity.WithIdentity(ctx, test_handler.MockIdentity)
	var emptyList rbac.AccessList
	s.mockCache.On("GetAccessList", ctx).Return(nil, cache.ErrNotFound)
	s.mockCache.On("SetAccessList", ctx, emptyList).Return(nil, cache.ErrNotFound)
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
