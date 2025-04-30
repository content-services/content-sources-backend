package roadmap_client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/config"
)

type RoadmapClient interface {
	GetAppstreams(ctx context.Context) (AppstreamsResponse, int, error)
}

type roadmapClient struct {
	client *http.Client
	cache  cache.Cache
}

func NewRoadmapClient() (RoadmapClient, error) {
	timeout := 90 * time.Second

	transport, err := config.GetTransport(nil, nil, nil, timeout)
	if err != nil {
		return nil, fmt.Errorf("error creating http transport: %w", err)
	}
	if config.Get().Clients.Roadmap.Proxy != "" {
		proxy, err := url.Parse(config.Get().Clients.Roadmap.Proxy)
		if err != nil {
			return nil, fmt.Errorf("error parsing proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxy)
	}

	httpClient := http.Client{Transport: transport, Timeout: timeout}

	return roadmapClient{client: &httpClient, cache: cache.Initialize()}, nil
}
