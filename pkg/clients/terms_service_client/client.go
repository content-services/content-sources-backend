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
}

type termsServiceImpl struct {
	client  http.Client
	baseURL string
}

func NewTermsServiceClient() (TermsServiceClient, error) {
	httpClient, err := config.GetHTTPClient(&config.TermsServiceCertUser{}, true)
	if err != nil {
		return nil, err
	}
	return &termsServiceImpl{
		client:  httpClient,
		baseURL: config.Get().Clients.TermsService.Server,
	}, nil
}

func (t *termsServiceImpl) IsTermsAcceptanceRequired(ctx context.Context, login string) (bool, error) {
	reqURL := fmt.Sprintf("%s/svcrest/terms/presentation/isrequired?login=%s&site=FIEnrollment&event=FITerms",
		t.baseURL, url.QueryEscape(login))

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
