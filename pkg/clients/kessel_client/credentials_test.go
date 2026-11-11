package kessel_client

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/stretchr/testify/suite"
)

type CredentialsTestSuite struct {
	suite.Suite
	origClients config.Clients
	tokenHits   atomic.Int32
	tokenStatus int
	tokenBody   string
	tokenServer *httptest.Server
}

func TestCredentialsSuite(t *testing.T) {
	suite.Run(t, new(CredentialsTestSuite))
}

func resetCredentials() {
	credentialsOnce = sync.Once{}
	credentials = nil
	credentialsErr = nil
}

func (s *CredentialsTestSuite) SetupTest() {
	resetCredentials()
	s.origClients = config.Get().Clients
	s.tokenHits.Store(0)
	s.tokenStatus = http.StatusOK
	s.tokenBody = `{"access_token":"tok","token_type":"Bearer","expires_in":3600}`
	s.tokenServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.tokenHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.tokenStatus)
		if s.tokenStatus == http.StatusOK {
			_, _ = w.Write([]byte(s.tokenBody))
		}
	}))

	auth := &config.Get().Clients.Kessel.Auth
	auth.ClientID = "id"
	auth.ClientSecret = "secret"
	auth.OIDCIssuer = s.tokenServer.URL
}

func (s *CredentialsTestSuite) TearDownTest() {
	s.tokenServer.Close()
	config.Get().Clients = s.origClients
	resetCredentials()
}

// rbacServer is a TLS server, trusted via RbacCaCertPath, that records the headers of every request it receives
func (s *CredentialsTestSuite) rbacServer(handler http.HandlerFunc) (*httptest.Server, *[]http.Header) {
	var mu sync.Mutex
	var seen []http.Header
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Clone())
		mu.Unlock()
		if handler != nil {
			handler(w, r)
		}
	}))
	s.T().Cleanup(srv.Close)
	config.Get().Clients.RbacCaCertPath = s.writeFile("ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
	return srv, &seen
}

func (s *CredentialsTestSuite) setRbac(baseURL string, authenticated bool) {
	config.Get().Clients.RbacBaseUrl = baseURL + "/api/rbac/v1"
	config.Get().Clients.RbacAuthenticated = authenticated
}

func (s *CredentialsTestSuite) get(ctx context.Context, url string) (int, error) {
	client, err := RbacHTTPClient(5 * time.Second)
	s.Require().NoError(err)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	s.Require().NoError(err)
	req.Header.Set("x-rh-identity", "identity")
	req.Header.Set("x-rh-rbac-org-id", "org")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

func (s *CredentialsTestSuite) TestAuthenticatedSendsBearer() {
	srv, seen := s.rbacServer(nil)
	s.setRbac(srv.URL, true)

	_, err := s.get(context.Background(), srv.URL+"/api/rbac/v1/access/")
	s.Require().NoError(err)
	s.Require().Len(*seen, 1)
	h := (*seen)[0]
	s.Equal([]string{"Bearer tok"}, h.Values("Authorization"))
	s.Equal("identity", h.Get("x-rh-identity"))
	s.Equal("org", h.Get("x-rh-rbac-org-id"))
	s.Equal(int32(1), s.tokenHits.Load())

	// token is cached by the SDK
	_, err = s.get(context.Background(), srv.URL+"/api/rbac/v1/access/")
	s.Require().NoError(err)
	s.Equal(int32(1), s.tokenHits.Load())
}

func (s *CredentialsTestSuite) TestUnauthenticatedNoBearer() {
	srv, seen := s.rbacServer(nil)
	s.setRbac(srv.URL, false)

	_, err := s.get(context.Background(), srv.URL+"/api/rbac/v1/access/")
	s.Require().NoError(err)
	s.Require().Len(*seen, 1)
	s.Empty((*seen)[0].Get("Authorization"))
	s.Equal("identity", (*seen)[0].Get("x-rh-identity"))
	s.Equal(int32(0), s.tokenHits.Load())
}

func (s *CredentialsTestSuite) TestAuthenticatedMissingCredentials() {
	srv, _ := s.rbacServer(nil)
	s.setRbac(srv.URL, true)
	config.Get().Clients.Kessel.Auth.ClientSecret = ""

	_, err := RbacHTTPClient(time.Second)
	s.Require().Error(err)
	s.Contains(err.Error(), "service account credentials not configured")

	config.Get().Clients.Kessel.Server = "kessel:9000"
	_, err = NewKesselClient()
	s.Require().Error(err)
	s.Equal(int32(0), s.tokenHits.Load())
}

func (s *CredentialsTestSuite) TestAuthenticatedRequiresHttps() {
	srv, _ := s.rbacServer(nil)
	s.setRbac("http://"+srv.Listener.Addr().String(), true)

	_, err := RbacHTTPClient(time.Second)
	s.Require().Error(err)
	s.Contains(err.Error(), "is not https")

	config.Get().Clients.Kessel.Server = "kessel:9000"
	_, err = NewKesselClient()
	s.Require().Error(err)
	s.Equal(int32(0), s.tokenHits.Load())
}

func (s *CredentialsTestSuite) TestAuthenticatedTokenFailureFailsClosed() {
	srv, seen := s.rbacServer(nil)
	s.setRbac(srv.URL, true)
	s.tokenStatus = http.StatusInternalServerError

	// the SDK retries 5xx with backoff, the deadline keeps the test fast
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := s.get(ctx, srv.URL+"/api/rbac/v1/access/")
	s.Require().Error(err)
	s.Empty(*seen)
	s.GreaterOrEqual(s.tokenHits.Load(), int32(1))
}

func (s *CredentialsTestSuite) TestAuthenticatedEmptyTokenFailsClosed() {
	srv, seen := s.rbacServer(nil)
	s.setRbac(srv.URL, true)
	s.tokenBody = `{"access_token":"","token_type":"Bearer","expires_in":3600}`

	_, err := s.get(context.Background(), srv.URL+"/api/rbac/v1/access/")
	s.Require().Error(err)
	s.Empty(*seen)
	s.GreaterOrEqual(s.tokenHits.Load(), int32(1))
}

func (s *CredentialsTestSuite) TestRedirectNotFollowed() {
	for _, authenticated := range []bool{false, true} {
		resetCredentials()
		other, otherSeen := s.rbacServer(nil)
		srv, seen := s.rbacServer(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.URL+"/elsewhere", http.StatusFound)
		})
		s.setRbac(srv.URL, authenticated)

		status, err := s.get(context.Background(), srv.URL+"/api/rbac/v1/access/")
		s.Require().NoError(err, "authenticated %v", authenticated)
		s.Equal(http.StatusFound, status)
		s.Require().Len(*seen, 1)
		s.Empty(*otherSeen, "authenticated %v", authenticated)
	}
}

func (s *CredentialsTestSuite) writeFile(name string, content []byte) string {
	path := filepath.Join(s.T().TempDir(), name)
	s.Require().NoError(os.WriteFile(path, content, 0600))
	return path
}

func (s *CredentialsTestSuite) TestCaCertificate() {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	s.setRbac(srv.URL, false)
	url := srv.URL + "/api/rbac/v1/access/"

	// without the CA the self-signed server is not trusted
	_, err := s.get(context.Background(), url)
	s.Require().Error(err)
	s.Contains(err.Error(), "certificate")

	config.Get().Clients.RbacCaCertPath = s.writeFile("ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
	status, err := s.get(context.Background(), url)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, status)
}

func (s *CredentialsTestSuite) TestCaCertificateInvalid() {
	config.Get().Clients.RbacCaCertPath = filepath.Join(s.T().TempDir(), "missing.pem")
	_, err := RbacHTTPClient(time.Second)
	s.Require().Error(err)
	s.Contains(err.Error(), "could not read rbac ca certificate")

	config.Get().Clients.RbacCaCertPath = s.writeFile("bad.pem", []byte("not a cert"))
	_, err = RbacHTTPClient(time.Second)
	s.Require().Error(err)
	s.Contains(err.Error(), "no certificates found")
}

func (s *CredentialsTestSuite) TestCredentialsShared() {
	first, err := Credentials()
	s.Require().NoError(err)
	second, err := Credentials()
	s.Require().NoError(err)
	s.Same(first, second)
}

func (s *CredentialsTestSuite) TestGetDefaultWorkspaceIDSendsBearer() {
	for _, kesselAuth := range []bool{false, true} {
		resetCredentials()
		srv, seen := s.rbacServer(func(w http.ResponseWriter, r *http.Request) {
			s.Equal("/api/rbac/v2/workspaces/", r.URL.Path)
			_, _ = w.Write([]byte(`{"data":[{"id":"ws-id"}]}`))
		})
		s.setRbac(srv.URL, true)
		config.Get().Clients.Kessel.Server = "kessel:9000"
		config.Get().Clients.Kessel.Timeout = 5 * time.Second
		config.Get().Clients.Kessel.Auth.Enabled = kesselAuth

		k, err := NewKesselClient()
		s.Require().NoError(err)
		id, status, err := k.GetDefaultWorkspaceID(context.Background(), "org")
		s.Require().NoError(err, "kessel auth %v", kesselAuth)
		s.Equal("ws-id", id)
		s.Equal(http.StatusOK, status)
		s.Require().Len(*seen, 1)
		s.Equal([]string{"Bearer tok"}, (*seen)[0].Values("Authorization"), "kessel auth %v", kesselAuth)
		s.Equal("org", (*seen)[0].Get("x-rh-rbac-org-id"))
	}
}
