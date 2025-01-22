package feature_service_client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/rs/zerolog/log"
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

type FeatureStatusResponse struct {
	Features []struct {
		Name     string `json:"name"`
		Entitled bool   `json:"entitled"`
	} `json:"features"`
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

func (fs featureServiceImpl) GetFeatureStatusByOrgID(ctx context.Context, orgID string) (api.FeatureStatus, int, error) {
	statusCode := http.StatusInternalServerError
	var err error

	features := config.Get().Options.FeatureFilter
	featureParams := make([]string, len(features))
	for i, feature := range features {
		featureParams[i] = fmt.Sprintf("features=%s", url.QueryEscape(feature))
	}
	path := fmt.Sprintf("/featureStatus?accountId=%s", orgID)
	fullPath := path + "&" + strings.Join(featureParams, "&")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.Get().Clients.FeatureService.Server+fullPath, nil)
	if err != nil {
		return api.FeatureStatus{}, 0, err
	}

	var body []byte
	resp, err := fs.client.Do(req)
	if resp != nil {
		defer resp.Body.Close()

		if resp.StatusCode != 0 {
			statusCode = resp.StatusCode
		}

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return api.FeatureStatus{}, http.StatusInternalServerError, fmt.Errorf("error during read response body: %w", err)
		}
	}
	if err != nil {
		return api.FeatureStatus{}, statusCode, fmt.Errorf("error during GET request: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return api.FeatureStatus{}, statusCode, fmt.Errorf("unexpected status code with body: %s", string(body))
	}

	var featStatus FeatureStatusResponse
	err = json.Unmarshal(body, &featStatus)
	if err != nil {
		return api.FeatureStatus{}, statusCode, fmt.Errorf("error during unmarshal response body: %w", err)
	}

	var entitledFeatures []string
	for _, feature := range featStatus.Features {
		if feature.Entitled {
			entitledFeatures = append(entitledFeatures, feature.Name)
		}
	}

	featStatusResp := api.FeatureStatus{
		OrgID:       orgID,
		FeatureList: entitledFeatures,
	}

	return featStatusResp, statusCode, nil
}

func (fs featureServiceImpl) GetEntitledFeatures(ctx context.Context, orgID string) ([]string, error) {
	entitledFeatures := []string{"RHEL-OS-x86_64", "RHEL-OS-aarch64"}

	if config.Get().Clients.FeatureService.Server == "" {
		if config.Get().Options.EntitleAll {
			return config.Get().Options.FeatureFilter, nil
		}
		return entitledFeatures, nil
	}

	cacheHit, err := fs.cache.GetFeatureStatus(ctx)
	if err != nil && !errors.Is(err, cache.NotFound) {
		log.Logger.Error().Err(err).Msg("featureStatus: error reading from cache")
	}
	if cacheHit != nil {
		entitledFeatures = append(entitledFeatures, cacheHit.FeatureList...)
		return entitledFeatures, nil
	}

	features, statusCode, err := fs.GetFeatureStatusByOrgID(ctx, orgID)
	if err != nil {
		return []string{}, ce.NewErrorResponse(statusCode, "error checking feature status", err.Error())
	}
	entitledFeatures = append(entitledFeatures, features.FeatureList...)

	err = fs.cache.SetFeatureStatus(ctx, features)
	if err != nil {
		log.Logger.Error().Err(err).Msg("featureStatus: error writing to cache")
	}

	return entitledFeatures, nil
}
