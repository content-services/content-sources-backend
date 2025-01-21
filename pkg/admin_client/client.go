package admin_client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/config"
)

type AdminClient interface {
	ListFeatures(ctx context.Context) (features FeaturesResponse, statusCode int, err error)
}

type adminClientImpl struct {
	client http.Client
}

func NewAdminClient() (AdminClient, error) {
	httpClient, err := config.GetHTTPClient(&config.SubsAsFeatsCertUser{})
	if err != nil {
		return nil, err
	}
	return adminClientImpl{client: httpClient}, nil
}

type FeaturesResponse struct {
	Content []Content `json:"content"`
}

type Content struct {
	Name  string `json:"name"`
	Rules Rules  `json:"rules"`
}

type Rules struct {
	MatchProducts []MatchProducts `json:"matchProducts"`
}

type MatchProducts struct {
	EngIDs []int `json:"engIds"`
}

func (ac adminClientImpl) ListFeatures(ctx context.Context) (FeaturesResponse, int, error) {
	statusCode := http.StatusInternalServerError
	var err error

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.Get().Clients.FeatureService.Server, nil)
	if err != nil {
		return FeaturesResponse{}, 0, err
	}

	var body []byte
	resp, err := ac.client.Do(req)
	if resp != nil {
		defer resp.Body.Close()

		if resp.StatusCode != 0 {
			statusCode = resp.StatusCode
		}

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return FeaturesResponse{}, http.StatusInternalServerError, fmt.Errorf("error during read response body: %w", err)
		}
	}
	if err != nil {
		return FeaturesResponse{}, statusCode, fmt.Errorf("error during GET request: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return FeaturesResponse{}, statusCode, fmt.Errorf("unexpected status code with body: %s", string(body))
	}

	var featResp FeaturesResponse
	err = json.Unmarshal(body, &featResp)
	if err != nil {
		return FeaturesResponse{}, statusCode, fmt.Errorf("error during unmarshal response body: %w", err)
	}

	return featResp, statusCode, nil
}
