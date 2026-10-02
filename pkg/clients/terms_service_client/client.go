package terms_service_client

import (
	"bytes"
	"context"
	"encoding/json"
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
	GetRequiredTerms(ctx context.Context, login string) ([]TermDetail, error)
	AcceptTerm(ctx context.Context, login string, pdfID string) error
}

// TermTranslation represents a single locale-specific translation of a term.
type TermTranslation struct {
	ID                     string  `json:"id"`
	TermsPdfID             string  `json:"termsPdfId"`
	LocaleCode             string  `json:"localeCode"`
	TranslatedTermsName    string  `json:"translatedTermsName"`
	TranslatedDescription  *string `json:"translatedDescription"`
	TranslatedInstructions *string `json:"translatedInstructions"`
	IsDefault              bool    `json:"isDefault"`
	PdfDownloadURL         string  `json:"pdfDownloadUrl"`
}

// TermDetail represents a single term returned by the terms service.
type TermDetail struct {
	ID                   string            `json:"id"`
	URLToDisplayThisTerm *string           `json:"urlToDisplayThisTerm"`
	URLToDisplayAllTerms string            `json:"urlToDisplayAllTerms"`
	IsOptional           bool              `json:"isOptional"`
	Translations         []TermTranslation `json:"translations"`
}

type termsServiceImpl struct {
	client http.Client
}

func NewTermsServiceClient() (TermsServiceClient, error) {
	httpClient, err := config.GetHTTPClient(&config.TermsServiceCertUser{}, true)
	if err != nil {
		return nil, err
	}
	return &termsServiceImpl{client: httpClient}, nil
}

func (t *termsServiceImpl) IsTermsAcceptanceRequired(ctx context.Context, login string) (bool, error) {
	reqURL := fmt.Sprintf("%s/svcrest/terms/presentation/isrequired?login=%s&site=FIEnrollment&event=FITerms",
		config.Get().Clients.TermsService.Server, url.QueryEscape(login))

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

func (t *termsServiceImpl) GetRequiredTerms(ctx context.Context, login string) ([]TermDetail, error) {
	reqURL := fmt.Sprintf("%s/svcrest/terms/presentation/required?login=%s&site=FIEnrollment&event=FITerms",
		config.Get().Clients.TermsService.Server, url.QueryEscape(login))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building required terms request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("terms service required call failed")
		return nil, fmt.Errorf("terms service required: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("terms service required returned %d: %s", resp.StatusCode, string(body))
	}

	var terms []TermDetail
	if err := json.NewDecoder(resp.Body).Decode(&terms); err != nil {
		return nil, fmt.Errorf("decoding required terms response: %w", err)
	}
	return terms, nil
}

func (t *termsServiceImpl) AcceptTerm(ctx context.Context, login string, pdfID string) error {
	// TODO: Confirm exact acceptance endpoint path from terms service docs
	reqURL := fmt.Sprintf("%s/svcrest/terms/presentation/accept", config.Get().Clients.TermsService.Server)

	payload := map[string]string{
		"login":      login,
		"termsPdfId": pdfID,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling accept request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("building accept request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("terms service accept call failed")
		return fmt.Errorf("terms service accept: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("terms service accept returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
