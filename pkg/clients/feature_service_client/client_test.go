package feature_service_client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestListFeatures(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"features": [{"name": "feature1"}, {"name": "feature2"}]}`))
	}))

	defer httpServer.Close()

	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Clients.FeatureService.Server = httpServer.URL

	fs := featureServiceImpl{
		client: http.Client{},
		cache:  cache.NewMockCache(t),
	}

	features, statusCode, err := fs.ListFeatures(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, FeaturesResponse{Features: []Feature{{Name: "feature1"}, {Name: "feature2"}}}, features)
}

func TestGetFeatureStatusByOrgID(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"features": [{"name": "feature1"}, {"name": "feature2"}]}`))
	}))

	defer httpServer.Close()

	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Clients.FeatureService.Server = httpServer.URL

	fs := featureServiceImpl{
		client: http.Client{},
		cache:  cache.NewMockCache(t),
	}

	featureStatus, statusCode, err := fs.GetFeatureStatusByOrgID(context.Background(), "123")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, api.FeatureStatus{OrgID: "123", FeatureList: []string{"feature1", "feature2"}}, featureStatus)
}

func TestGetEntitledFeatures(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"features": [{"name": "feature1"}, {"name": "feature2"}]}`))
	}))

	defer httpServer.Close()

	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Clients.FeatureService.Server = httpServer.URL

	mockCache := cache.NewMockCache(t)
	fs := featureServiceImpl{
		client: http.Client{},
		cache:  mockCache,
	}

	ctx := context.Background()

	mockCache.On("GetFeatureStatus", ctx).Return(nil, cache.ErrNotFound)
	mockCache.On("SetFeatureStatus", ctx, api.FeatureStatus{OrgID: "123", FeatureList: []string{"feature1", "feature2"}}).Return(nil)
	entitledFeatures, err := fs.GetEntitledFeatures(ctx, "123")
	assert.NoError(t, err)
	assert.Equal(t, []string{"RHEL-OS-x86_64", "feature1", "feature2"}, entitledFeatures)
}
