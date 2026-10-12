package terms_service_client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/rs/zerolog/log"
)

// Terms responses are tiny booleans ("true"/"false"); keep a tight bound.
const maxTermsResponseBytes = 1 << 10 // 1 KB

// loginPattern matches Red Hat usernames used by the terms service.
var loginPattern = regexp.MustCompile(`^[A-Za-z0-9._@+-]+$`)

type TermsServiceClient interface {
	IsTermsAcceptanceRequired(ctx context.Context, login string) (bool, error)
	GetRequiredEvents(ctx context.Context, login string) ([]string, error)
}

type termsServiceImpl struct {
	client  http.Client
	baseURL string
	site    string
	events  []string
}

func NewTermsServiceClient() (TermsServiceClient, error) {
	httpClient, err := config.GetHTTPClient(&config.TermsServiceCertUser{}, true)
	if err != nil {
		return nil, err
	}
	ts := config.Get().Clients.TermsService
	return &termsServiceImpl{
		client:  httpClient,
		baseURL: ts.Server,
		site:    ts.Site,
		events:  ts.Events(),
	}, nil
}

func (t *termsServiceImpl) IsTermsAcceptanceRequired(ctx context.Context, login string) (bool, error) {
	required, err := t.GetRequiredEvents(ctx, login)
	if err != nil {
		return false, err
	}
	return len(required) > 0, nil
}

// isEventRequired checks a single event for the given user.
func (t *termsServiceImpl) isEventRequired(ctx context.Context, login string, event string) (bool, error) {
	if !loginPattern.MatchString(login) {
		return false, fmt.Errorf("invalid login for terms service request")
	}

	reqURL, err := t.buildIsRequiredURL(login, event)
	if err != nil {
		return false, fmt.Errorf("building isrequired request for event %q: %w", event, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, fmt.Errorf("building isrequired request for event %q: %w", event, err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Str("event", event).Msg("terms service isrequired call failed")
		return false, fmt.Errorf("terms service isrequired for event %q: %w", event, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTermsResponseBytes))
	if err != nil {
		return false, fmt.Errorf("reading isrequired response for event %q: %w", event, err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Ctx(ctx).Error().
			Int("status", resp.StatusCode).
			Str("event", event).
			Str("body", string(body)).
			Msg("terms service isrequired returned non-OK status")
		return false, fmt.Errorf("terms service isrequired for event %q returned %d", event, resp.StatusCode)
	}

	trimmed := strings.TrimSpace(string(body))
	return strings.EqualFold(trimmed, "true"), nil
}

// buildIsRequiredURL constructs the terms isrequired URL from the configured
// base URL (host/path) and query parameters. User-influenced values are only
// placed in the query string via url.Values encoding.
func (t *termsServiceImpl) buildIsRequiredURL(login string, event string) (string, error) {
	u, err := url.Parse(t.baseURL)
	if err != nil {
		return "", fmt.Errorf("parsing terms service base URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("unsupported terms service URL scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("terms service base URL missing host")
	}

	u = u.JoinPath("svcrest", "terms", "presentation", "isrequired")
	params := url.Values{}
	params.Set("login", login)
	params.Set("site", t.site)
	params.Set("event", event)
	u.RawQuery = params.Encode()
	return u.String(), nil
}

// GetRequiredEvents checks each configured event individually and returns only
// those that still require acceptance by the given user.
func (t *termsServiceImpl) GetRequiredEvents(ctx context.Context, login string) ([]string, error) {
	var required []string
	for _, event := range t.events {
		ok, err := t.isEventRequired(ctx, login, event)
		if err != nil {
			return nil, err
		}
		if ok {
			required = append(required, event)
		}
	}
	return required, nil
}
