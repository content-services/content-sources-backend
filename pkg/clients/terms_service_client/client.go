package terms_service_client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/cache"
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
	client       http.Client
	endpointURL  string // scheme+host+path only; no query string
	site         string
	events       []string
	cache        cache.Cache
	cacheTTL     time.Duration
	cacheVersion string
}

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
	return &termsServiceImpl{
		client:       httpClient,
		endpointURL:  endpointURL,
		site:         ts.Site,
		events:       ts.Events(),
		cache:        cache.InitializeTerms(),
		cacheTTL:     config.Get().Clients.Redis.Expiration.Terms,
		cacheVersion: ts.CacheVersion,
	}, nil
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
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.RawQuery, u.Fragment = "", ""
	u.User = nil
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
		return false, fmt.Errorf("invalid login for terms service request")
	}

	if err := ctx.Err(); err != nil {
		return false, err
	}
	scope := cache.TermsScope{Endpoint: t.endpointURL, Site: t.site, Login: login, Event: event, Version: t.cacheVersion}
	cacheEnabled := t.cacheTTL > 0 && t.cache != nil
	cacheStatus := "disabled"
	if cacheEnabled {
		hit, err := t.cache.GetTermsNotRequired(ctx, scope)
		if hit && err == nil {
			log.Ctx(ctx).Debug().Str("event", event).Str("cache_status", "hit").Msg("terms cache lookup")
			return false, nil
		}
		cacheStatus = "miss"
		if err != nil && !errors.Is(err, cache.ErrNotFound) {
			log.Ctx(ctx).Warn().Err(err).Str("event", event).Msg("terms cache read failed")
		}
	}
	log.Ctx(ctx).Debug().Str("event", event).Str("cache_status", cacheStatus).Msg("terms cache lookup")
	started := time.Now()
	required, err := t.fetchEventRequired(ctx, login, event)
	log.Ctx(ctx).Debug().Str("event", event).Dur("duration_ms", time.Since(started)).Msg("terms upstream check")
	if err != nil {
		return false, err
	}
	if !required && cacheEnabled {
		if err := t.cache.SetTermsNotRequired(ctx, scope, t.cacheTTL); err != nil {
			log.Ctx(ctx).Warn().Err(err).Str("event", event).Msg("terms cache write failed")
		}
	}
	return required, nil
}

func (t *termsServiceImpl) fetchEventRequired(ctx context.Context, login, event string) (bool, error) {
	// Build the request against the configured endpoint only, then attach
	// query parameters afterward so user-influenced values never enter the
	// URL passed to NewRequestWithContext.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.endpointURL, nil)
	if err != nil {
		return false, fmt.Errorf("building isrequired request for event %q: %w", event, err)
	}
	params := url.Values{}
	params.Set("login", login)
	params.Set("site", t.site)
	params.Set("event", event)
	req.URL.RawQuery = params.Encode()

	resp, err := t.client.Do(req)
	if err != nil {
		// net/http errors include the query URL (and login). Preserve the cause only.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		log.Ctx(ctx).Error().Err(err).Str("event", event).Msg("terms service isrequired call failed")
		return false, fmt.Errorf("terms service isrequired for event %q: %w", event, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTermsResponseBytes+1))
	if err != nil {
		return false, fmt.Errorf("reading isrequired response for event %q: %w", event, err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Ctx(ctx).Error().
			Int("status", resp.StatusCode).
			Str("event", event).
			Msg("terms service isrequired returned non-OK status")
		return false, fmt.Errorf("terms service isrequired for event %q returned %d", event, resp.StatusCode)
	}

	if len(body) > maxTermsResponseBytes {
		return false, fmt.Errorf("isrequired response for event %q exceeds size limit", event)
	}
	trimmed := strings.TrimSpace(string(body))
	switch {
	case strings.EqualFold(trimmed, "true"):
		return true, nil
	case strings.EqualFold(trimmed, "false"):
		if err := ctx.Err(); err != nil {
			return false, err
		}
		return false, nil
	default:
		return false, fmt.Errorf("invalid isrequired boolean for event %q", event)
	}
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
