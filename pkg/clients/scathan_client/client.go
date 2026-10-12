package scathan_client

import (
	"context"
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// ScathanClient talks to the Lightwell Vulnerability Metadata Service (Scathan),
// which mirrors OSIDB and renders OSV/VEX on demand. content-sources calls it
// service-to-service in place of reading Lightwell OSV from static Pulp files.
//
// Scathan is reached as a public (internet-exposed) service secured by mTLS:
// content-sources/console.dot runs outside the Red Hat network, so the
// content-sources -> Scathan hop is authenticated with an mTLS client cert
// (and can be locked to the consoledot CIDR list), while Scathan's own API auth
// is a service-account bearer token — the same auth as the Lightwell network API.
// Scathan stays fail-closed on embargoed flaws, so every data call forwards the
// caller's x-rh-identity so Scathan can enforce the customer's entitlement/audience.
// The transport auth is config-only: either mTLS or an OIDC service-account bearer
// token depending on config (see config.Scathan).
type ScathanClient interface {
	// GetOSV fetches the OSV record for a single vulnerability id (a CVE or a
	// Lightwell LW-* id): GET /osv/{id}.
	GetOSV(ctx context.Context, vulnID string) (record []byte, statusCode int, err error)
	// GetOSVByCoordinates fetches all OSV records affecting a package URL:
	// GET /coordinates?purl=...&format=osv.
	GetOSVByCoordinates(ctx context.Context, purl string) (record []byte, statusCode int, err error)
	// Healthz reports Scathan readiness: GET /healthz. No identity is forwarded.
	Healthz(ctx context.Context) (statusCode int, err error)
}

type scathanImpl struct {
	client http.Client
}

// NewScathanClient builds a client using whichever service-to-service auth is
// configured: an OIDC service account when clients.scathan.auth.enabled is true,
// otherwise mTLS with the internal Red Hat CA (clients.scathan.client_cert/*).
// The choice is intentionally config-only so it can be flipped without a code
// change once the content-sources team settles on one mechanism.
func NewScathanClient() (ScathanClient, error) {
	auth := config.Get().Clients.Scathan.Auth
	if auth.Enabled {
		return scathanImpl{client: *oidcHTTPClient(context.Background(), auth)}, nil
	}

	httpClient, err := config.GetHTTPClient(&config.ScathanCertUser{}, true)
	if err != nil {
		return nil, err
	}
	return scathanImpl{client: httpClient}, nil
}

// oidcHTTPClient returns an http.Client that attaches (and auto-refreshes) an
// OAuth2 client-credentials bearer token.
//
// TODO(scathan-auth): the token endpoint is derived from the issuer using the
// standard Keycloak / Red Hat SSO realm path. If the OIDC option is chosen over
// mTLS, switch this to full OIDC discovery of the token_endpoint rather than
// assuming the path.
func oidcHTTPClient(ctx context.Context, auth config.ScathanAuth) *http.Client {
	cc := clientcredentials.Config{
		ClientID:     auth.ClientID,
		ClientSecret: auth.ClientSecret,
		TokenURL:     strings.TrimSuffix(auth.OIDCIssuer, "/") + "/protocol/openid-connect/token",
		AuthStyle:    oauth2.AuthStyleInParams,
	}
	return cc.Client(ctx)
}
