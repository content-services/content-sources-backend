package feature_service_client

import (
	"context"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/config"
)

type FeatureServiceClient interface {
	ListFeatures(ctx context.Context) (features FeaturesResponse, statusCode int, err error)
	GetFeatureStatusByOrgID(ctx context.Context, orgID string) (featureStatus api.FeatureStatus, statusCode int, err error)
	GetEntitledFeatures(ctx context.Context, orgID string) (features []string, err error)
}

type featureServiceImpl struct {
	client http.Client
	cache  cache.Cache
}

func NewFeatureServiceClient() (FeatureServiceClient, error) {
	httpClient, err := config.GetHTTPClient(&config.FeatureServiceCertUser{})
	if err != nil {
		return nil, err
	}
	return featureServiceImpl{
		client: httpClient,
		cache:  cache.Initialize(),
	}, nil
}
