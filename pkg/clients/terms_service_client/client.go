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
	client      http.Client
	endpointURL string // scheme+host+path only; no query string
	site        string
	events      []string
}

// pingProbeLogin is used only to verify the terms service is reachable with
// the configured client certs. The required true/false result is ignored.
const pingProbeLogin = "content-sources-ping"

func NewTermsServiceClient() (TermsServiceClient, error) {
	httpClient, err := config.GetHTTPClient(&config.TermsServiceCertUser{}, true)
	if err != nil {
		return nil, err
	}
	ts := config.Get().Clients.TermsService
	endpointURL, err := buildEndpointURL(ts.Server)
	if err != nil {
		return nil, err
	}
	client := &termsServiceImpl{
		client:      httpClient,
		endpointURL: endpointURL,
		site:        ts.Site,
		events:      ts.Events(),
	}
	if err := client.Ping(context.Background()); err != nil {
		log.Error().Err(err).
			Str("endpoint", endpointURL).
			Str("site", ts.Site).
			Strs("events", client.events).
			Msg("terms service ping failed")
	} else {
		log.Info().
			Str("endpoint", endpointURL).
			Str("site", ts.Site).
			Strs("events", client.events).
			Msg("terms service ping ok")
	}
	return client, nil
}

// Ping checks that the terms service is reachable with the configured certs,
// site, and at least one event. A 200 response with body "false" still counts
// as success; only transport or non-OK status failures are reported.
func (t *termsServiceImpl) Ping(ctx context.Context) error {
	if t.site == "" {
		return fmt.Errorf("terms site is empty")
	}
	if len(t.events) == 0 {
		return fmt.Errorf("no configured terms events")
	}
	_, err := t.isEventRequired(ctx, pingProbeLogin, t.events[0])
	if err != nil {
		return fmt.Errorf("terms service ping: %w", err)
	}
	return nil
}

func buildEndpointURL(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parsing terms service base URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("unsupported terms service URL scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("terms service base URL missing host")
	}
	return u.JoinPath("svcrest", "terms", "presentation", "isrequired").String(), nil
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
		log.Ctx(ctx).Debug().
			Str("login", login).
			Str("site", t.site).
			Str("event", event).
			Str("url", t.endpointURL).
			Msg("login pattern doesn't match")
		return false, fmt.Errorf("invalid login for terms service request")
	}

	// Build the request against the configured endpoint only, then attach
	// query parameters afterward so user-influenced values never enter the
	// URL passed to NewRequestWithContext.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.endpointURL, nil)
	if err != nil {
		log.Ctx(ctx).Debug().
			Str("login", login).
			Str("site", t.site).
			Str("event", event).
			Str("url", t.endpointURL).
			Str("err", err.Error()).
			Msg("building request")
		return false, fmt.Errorf("building isrequired request for event %q: %w", event, err)
	}
	params := url.Values{}
	params.Set("login", login)
	params.Set("site", t.site)
	params.Set("event", event)
	req.URL.RawQuery = params.Encode()

	resp, err := t.client.Do(req)
	if err != nil {
		log.Ctx(ctx).Debug().
			Str("login", login).
			Str("site", t.site).
			Str("event", event).
			Str("url", t.endpointURL).
			Str("err", err.Error()).
			Msg("making request")
		log.Ctx(ctx).Error().Err(err).Str("event", event).Msg("terms service isrequired call failed")
		return false, fmt.Errorf("terms service isrequired for event %q: %w", event, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTermsResponseBytes))
	if err != nil {
		log.Ctx(ctx).Debug().
			Str("login", login).
			Str("site", t.site).
			Str("event", event).
			Str("url", t.endpointURL).
			Str("err", err.Error()).
			Int("resp", resp.StatusCode).
			Str("body", string(body)).
			Msg("reading response")
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

	log.Ctx(ctx).Debug().
		Str("login", login).
		Str("site", t.site).
		Str("event", event).
		Str("url", req.URL.String()).
		Int("status", resp.StatusCode).
		Str("body", trimmed).
		Bool("required", strings.EqualFold(trimmed, "true")).
		Msg("iseventrequired end")

	return strings.EqualFold(trimmed, "true"), nil
}

// GetRequiredEvents checks each configured event individually and returns only
// those that still require acceptance by the given user.
func (t *termsServiceImpl) GetRequiredEvents(ctx context.Context, login string) ([]string, error) {
	var required []string
	log.Ctx(ctx).Debug().Msgf("GetRequiredEvents - t.events: %v", t.events)
	log.Ctx(ctx).Debug().Msgf("GetRequiredEvents - config events: %v", config.Get().Clients.TermsService.Events())
	for _, event := range config.Get().Clients.TermsService.Events() {
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
