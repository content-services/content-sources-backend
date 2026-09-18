package scathan_client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
)

func (s scathanImpl) GetOSV(ctx context.Context, vulnID string) ([]byte, int, error) {
	return s.get(ctx, "/osv/"+url.PathEscape(vulnID), true)
}

func (s scathanImpl) GetOSVByCoordinates(ctx context.Context, purl string) ([]byte, int, error) {
	q := url.Values{}
	q.Set("purl", purl)
	q.Set("format", "osv")
	return s.get(ctx, "/coordinates?"+q.Encode(), true)
}

func (s scathanImpl) Healthz(ctx context.Context) (int, error) {
	_, statusCode, err := s.get(ctx, "/healthz", false)
	return statusCode, err
}

// get issues a GET against the Scathan server for the given path (which may
// include a query string) and returns the raw response body. When
// forwardIdentity is true, the caller's x-rh-identity is attached so Scathan can
// enforce embargo/entitlement; it is omitted for unauthenticated probes.
func (s scathanImpl) get(ctx context.Context, path string, forwardIdentity bool) ([]byte, int, error) {
	statusCode := http.StatusInternalServerError

	server := strings.TrimSuffix(config.Get().Clients.Scathan.Server, "/")
	if server == "" {
		return nil, statusCode, fmt.Errorf("scathan client: clients.scathan.server is not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+path, nil)
	if err != nil {
		return nil, statusCode, fmt.Errorf("error creating scathan request: %w", err)
	}

	if forwardIdentity {
		encoded, err := encodedIdentity(ctx)
		if err != nil {
			return nil, statusCode, fmt.Errorf("error encoding identity for scathan: %w", err)
		}
		req.Header.Set(api.IdentityHeader, encoded)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, statusCode, fmt.Errorf("error during scathan GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 0 {
		statusCode = resp.StatusCode
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, statusCode, fmt.Errorf("error reading scathan response body: %w", err)
	}

	if statusCode < 200 || statusCode >= 300 {
		return nil, statusCode, fmt.Errorf("scathan returned unexpected status %d with body: %s", statusCode, string(body))
	}

	return body, statusCode, nil
}

// encodedIdentity base64-encodes the x-rh-identity from the request context to
// forward to Scathan. Mirrors roadmap_client.encodedIdentity.
func encodedIdentity(ctx context.Context) (string, error) {
	id := identity.GetIdentity(ctx)
	jsonIdentity, err := json.Marshal(id)
	if err != nil {
		return "", fmt.Errorf("error marshaling identity: %w", err)
	}
	return base64.StdEncoding.EncodeToString(jsonIdentity), nil
}
