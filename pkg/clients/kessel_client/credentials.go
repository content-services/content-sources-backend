package kessel_client

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/project-kessel/kessel-sdk-go/kessel/auth"
)

var (
	credentialsOnce sync.Once
	credentials     *auth.OAuth2ClientCredentials
	credentialsErr  error
)

// The SDK holds a shared lock while refreshing, so a hung token endpoint must not block forever
var tokenHTTPClient = &http.Client{Timeout: 10 * time.Second}

// Credentials returns the service account credentials shared by the Kessel gRPC client
// and bearer-authenticated HTTP calls. The SDK caches and refreshes the token.
func Credentials() (*auth.OAuth2ClientCredentials, error) {
	credentialsOnce.Do(func() {
		a := config.Get().Clients.Kessel.Auth
		if a.ClientID == "" || a.ClientSecret == "" || a.OIDCIssuer == "" {
			credentialsErr = fmt.Errorf("service account credentials not configured: clients.kessel.auth client_id, client_secret and oidc_issuer are required")
			return
		}
		c := auth.NewOAuth2ClientCredentials(a.ClientID, a.ClientSecret, a.OIDCIssuer)
		credentials = &c
	})
	return credentials, credentialsErr
}

// RbacHTTPClient returns an HTTP client for RBAC that trusts the Clowder-provided CA when set,
// and adds the gateway bearer token when the Clowder v2 rbac endpoint is authenticated.
func RbacHTTPClient(timeout time.Duration) (*http.Client, error) {
	clients := config.Get().Clients
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("http.DefaultTransport is not an *http.Transport")
	}
	transport := defaultTransport.Clone()
	if clients.RbacCaCertPath != "" {
		pem, err := os.ReadFile(clients.RbacCaCertPath)
		if err != nil {
			return nil, fmt.Errorf("could not read rbac ca certificate: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in rbac ca certificate %s", clients.RbacCaCertPath)
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	var rt http.RoundTripper = transport
	if clients.RbacAuthenticated {
		rbacURL, err := url.Parse(clients.RbacBaseUrl)
		if err != nil || rbacURL.Scheme != "https" || rbacURL.Host == "" {
			return nil, fmt.Errorf("rbac endpoint is authenticated but base url %q is not https", clients.RbacBaseUrl)
		}
		if _, err := Credentials(); err != nil {
			return nil, fmt.Errorf("rbac endpoint is authenticated: %w", err)
		}
		rt = bearerTransport{base: transport}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: rt,
		// Never follow redirects, so the token and identity headers only go to the rbac url
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

// bearerTransport fails closed: a request is never sent without the token.
type bearerTransport struct {
	base http.RoundTripper
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := gatewayToken(req)
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(req)
}

func gatewayToken(req *http.Request) (string, error) {
	creds, err := Credentials()
	if err != nil {
		return "", fmt.Errorf("rbac endpoint is authenticated: %w", err)
	}
	resp, err := creds.GetToken(req.Context(), auth.GetTokenOptions{HttpClient: tokenHTTPClient})
	if err != nil {
		return "", fmt.Errorf("rbac endpoint is authenticated, could not get token: %w", err)
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("rbac endpoint is authenticated, token endpoint returned an empty token")
	}
	return resp.AccessToken, nil
}
