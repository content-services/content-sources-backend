package roadmap_client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
)

type Meta struct {
	Count int `json:"count"`
	Total int `json:"total"`
}

type AppstreamsResponse struct {
	Meta Meta              `json:"meta"`
	Data []AppstreamEntity `json:"data"`
}

type AppstreamEntity struct {
	Name      string `json:"name"`
	Stream    string `json:"stream"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Version   string `json:"version"`
	Impl      string `json:"impl"`
}

type LifecycleResponse struct {
	Data []LifecycleEntity `json:"data"`
}

type LifecycleEntity struct {
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Major     int    `json:"major"`
	Minor     int    `json:"minor"`
}

func encodedIdentity(ctx context.Context) (string, error) {
	id := identity.GetIdentity(ctx)
	jsonIdentity, err := json.Marshal(id)
	if err != nil {
		return "", fmt.Errorf("error marshaling json: %w", err)
	}
	return base64.StdEncoding.EncodeToString(jsonIdentity), nil
}

func (rc roadmapClient) GetAppstreams(ctx context.Context) (AppstreamsResponse, int, error) {
	statusCode := http.StatusInternalServerError
	server := config.Get().Clients.Roadmap.Server
	var err error
	var appStreamResp AppstreamsResponse
	var body []byte

	appstreams, err := rc.cache.GetRoadmapAppstreams(ctx)
	if err != nil && !errors.Is(err, cache.NotFound) {
		log.Error().Err(err).Msg("GetAppstreams - error reading from cache")
	}
	if appstreams != nil {
		err = json.Unmarshal(appstreams, &appStreamResp)
		if err != nil {
			return AppstreamsResponse{}, statusCode, fmt.Errorf("error during unmarshal response body: %w", err)
		}
		return appStreamResp, http.StatusOK, nil
	}

	fullPath := server + "/lifecycle/app-streams"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullPath, nil)
	if err != nil {
		return AppstreamsResponse{}, statusCode, fmt.Errorf("error building request: %w", err)
	}

	if config.Get().Clients.Roadmap.Username != "" && config.Get().Clients.Roadmap.Password != "" {
		req.SetBasicAuth(config.Get().Clients.Roadmap.Username, config.Get().Clients.Roadmap.Password)
	}

	encodedXRHID, err := encodedIdentity(ctx)
	if err != nil {
		return AppstreamsResponse{}, statusCode, fmt.Errorf("error getting encoded XRHID: %w", err)
	}
	req.Header.Set(api.IdentityHeader, encodedXRHID)

	resp, err := rc.client.Do(req)
	if resp != nil {
		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return AppstreamsResponse{}, statusCode, fmt.Errorf("error reading response body: %w", err)
		}
		if resp.StatusCode != 0 {
			statusCode = resp.StatusCode
		}
	}
	if err != nil {
		return AppstreamsResponse{}, statusCode, fmt.Errorf("error sending request: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return AppstreamsResponse{}, statusCode, fmt.Errorf("unexpected status code with body: %s", string(body))
	}

	rc.cache.SetRoadmapAppstreams(ctx, body)

	err = json.Unmarshal(body, &appStreamResp)
	if err != nil {
		return AppstreamsResponse{}, statusCode, fmt.Errorf("error during unmarshal response body: %w", err)
	}

	return appStreamResp, statusCode, nil
}

func (rc roadmapClient) GetRhelLifecycle(ctx context.Context) (LifecycleResponse, int, error) {
	statusCode := http.StatusInternalServerError
	server := config.Get().Clients.Roadmap.Server
	var err error
	var lifecycleResponse LifecycleResponse
	var body []byte

	appstreams, err := rc.cache.GetRoadmapRhelLifecycle(ctx)
	if err != nil && !errors.Is(err, cache.NotFound) {
		log.Error().Err(err).Msg("GetAppstreams - error reading from cache")
	}
	if appstreams != nil {
		err = json.Unmarshal(appstreams, &lifecycleResponse)
		if err != nil {
			return LifecycleResponse{}, statusCode, fmt.Errorf("error during unmarshal response body: %w", err)
		}
		return lifecycleResponse, http.StatusOK, nil
	}

	fullPath := server + "/lifecycle/rhel"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullPath, nil)
	if err != nil {
		return LifecycleResponse{}, statusCode, fmt.Errorf("error building request: %w", err)
	}

	if config.Get().Clients.Roadmap.Username != "" && config.Get().Clients.Roadmap.Password != "" {
		req.SetBasicAuth(config.Get().Clients.Roadmap.Username, config.Get().Clients.Roadmap.Password)
	}

	encodedXRHID, err := encodedIdentity(ctx)
	if err != nil {
		return LifecycleResponse{}, statusCode, fmt.Errorf("error getting encoded XRHID: %w", err)
	}
	req.Header.Set(api.IdentityHeader, encodedXRHID)

	resp, err := rc.client.Do(req)
	if resp != nil {
		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return LifecycleResponse{}, statusCode, fmt.Errorf("error reading response body: %w", err)
		}
		if resp.StatusCode != 0 {
			statusCode = resp.StatusCode
		}
	}
	if err != nil {
		return LifecycleResponse{}, statusCode, fmt.Errorf("error sending request: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return LifecycleResponse{}, statusCode, fmt.Errorf("unexpected status code with body: %s", string(body))
	}

	rc.cache.SetRoadmapRhelLifecycle(ctx, body)

	err = json.Unmarshal(body, &lifecycleResponse)
	if err != nil {
		return LifecycleResponse{}, statusCode, fmt.Errorf("error during unmarshal response body: %w", err)
	}

	return lifecycleResponse, statusCode, nil
}

func (rc roadmapClient) GetRhelLifecycleForLatestMajorVersions(ctx context.Context) (map[int]LifecycleEntity, error) {
	lifecycleResp, _, err := rc.GetRhelLifecycle(ctx)
	if err != nil {
		return nil, err
	}

	rhelEolMap := make(map[int]LifecycleEntity)
	for _, item := range lifecycleResp.Data {
		if existing, found := rhelEolMap[item.Major]; !found || (item.Minor > existing.Minor) {
			rhelEolMap[item.Major] = item
		}
	}
	return rhelEolMap, nil
}
