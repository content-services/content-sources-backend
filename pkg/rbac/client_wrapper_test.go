package rbac

import (
	"context"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	"github.com/stretchr/testify/require"
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

// Only test in this package that reaches kessel_client.Credentials(): the result is cached once per process.
func TestNewClientWrapperImplAuthenticated(t *testing.T) {
	orig := config.Get().Clients
	t.Cleanup(func() { config.Get().Clients = orig })

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	var seen http.Header
	rbacServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		_, _ = w.Write([]byte(`{"meta":{"count":0},"links":{},"data":[]}`))
	}))
	defer rbacServer.Close()
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rbacServer.Certificate().Raw}), 0600))

	baseURL := rbacServer.URL + "/api/rbac/v1"
	c := config.Get()
	c.Clients.RbacBaseUrl = baseURL
	c.Clients.RbacAuthenticated = true
	c.Clients.RbacCaCertPath = caPath
	c.Clients.Redis.Host = ""
	c.Clients.Kessel.Auth.ClientID = "id"
	c.Clients.Kessel.Auth.ClientSecret = "secret"
	c.Clients.Kessel.Auth.OIDCIssuer = tokenServer.URL

	wrapper, ok := NewClientWrapperImpl(baseURL, 5*time.Second).(*ClientWrapperImpl)
	require.True(t, ok)
	_, err := wrapper.client.GetAccess(context.Background(), "identity", "")
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok", seen.Get("Authorization"))
	assert.Equal(t, "identity", seen.Get("x-rh-identity"))
}

func TestNewClientWrapperImplUnauthenticated(t *testing.T) {
	orig := config.Get().Clients
	t.Cleanup(func() { config.Get().Clients = orig })

	var seen http.Header
	rbacServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		_, _ = w.Write([]byte(`{"meta":{"count":0},"links":{},"data":[]}`))
	}))
	defer rbacServer.Close()

	baseURL := rbacServer.URL + "/api/rbac/v1"
	c := config.Get()
	c.Clients.RbacBaseUrl = baseURL
	c.Clients.RbacAuthenticated = false
	c.Clients.RbacCaCertPath = ""
	c.Clients.Redis.Host = ""

	wrapper, ok := NewClientWrapperImpl(baseURL, 5*time.Second).(*ClientWrapperImpl)
	require.True(t, ok)
	_, err := wrapper.client.GetAccess(context.Background(), "identity", "")
	require.NoError(t, err)
	assert.Empty(t, seen.Get("Authorization"))
	assert.Equal(t, "identity", seen.Get("x-rh-identity"))
}

func TestNewClientWrapperImplBadCaCertificate(t *testing.T) {
	orig := config.Get().Clients
	t.Cleanup(func() { config.Get().Clients = orig })
	config.Get().Clients.RbacCaCertPath = filepath.Join(t.TempDir(), "missing.pem")

	assert.Nil(t, NewClientWrapperImpl("http://rbac:8000/api/rbac/v1", time.Second))
}
