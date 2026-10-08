package terms_service_client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/rs/zerolog/log"
)

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
		events:  ts.Events,
	}, nil
}

func (t *termsServiceImpl) IsTermsAcceptanceRequired(ctx context.Context, login string) (bool, error) {
	params := url.Values{}
	params.Set("login", login)
	params.Set("site", t.site)
	for _, event := range t.events {
		params.Add("event", event)
	}
	reqURL := fmt.Sprintf("%s/svcrest/terms/presentation/isrequired?%s", t.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, fmt.Errorf("building isrequired request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("terms service isrequired call failed")
		return false, fmt.Errorf("terms service isrequired: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("reading isrequired response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("terms service isrequired returned %d: %s", resp.StatusCode, string(body))
	}

	// Defensive: handles both JSON boolean (true/false) and plain-text ("True"/"False").
	// Wire format unconfirmed — see open items.
	trimmed := strings.TrimSpace(string(body))
	return strings.EqualFold(trimmed, "true"), nil
}

// isEventRequired checks a single event for the given user.
func (t *termsServiceImpl) isEventRequired(ctx context.Context, login string, event string) (bool, error) {
	params := url.Values{}
	params.Set("login", login)
	params.Set("site", t.site)
	params.Set("event", event)
	reqURL := fmt.Sprintf("%s/svcrest/terms/presentation/isrequired?%s", t.baseURL, params.Encode())

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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("reading isrequired response for event %q: %w", event, err)
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("terms service isrequired for event %q returned %d: %s", event, resp.StatusCode, string(body))
	}

	trimmed := strings.TrimSpace(string(body))
	return strings.EqualFold(trimmed, "true"), nil
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
