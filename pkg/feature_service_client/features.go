package feature_service_client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/config"
)

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

func (fs featureServiceImpl) ListFeatures(ctx context.Context) (FeaturesResponse, int, error) {
	statusCode := http.StatusInternalServerError
	var err error

	server := config.Get().Clients.FeatureService.Server + "/features/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server, nil)
	if err != nil {
		return FeaturesResponse{}, statusCode, err
	}

	var body []byte
	resp, err := fs.client.Do(req)
	if resp != nil {
		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return FeaturesResponse{}, statusCode, fmt.Errorf("error during read response body: %w", err)
		}

		if resp.StatusCode != 0 {
			statusCode = resp.StatusCode
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
