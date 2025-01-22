package feature_service_client

import (
	"context"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/config"
)

type FeatureServiceClient interface {
	ListFeatures(ctx context.Context) (features FeaturesResponse, statusCode int, err error)
}

type featureServiceImpl struct {
	client http.Client
}

func NewFeatureServiceClient() (FeatureServiceClient, error) {
	httpClient, err := config.GetHTTPClient(&config.FeatureServiceCertUser{})
	if err != nil {
		return nil, err
	}
	return featureServiceImpl{client: httpClient}, nil
}
