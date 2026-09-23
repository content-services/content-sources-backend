package candlepin_client

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/stretchr/testify/assert"
)

const overrideExposedHeader = "x-rh-override-exposed"

func TestGetCandlepinClientSetsOverrideExposedHeader(t *testing.T) {
	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Clients.Candlepin.OverrideExposed = "some-secret-value"
	defer func() { config.LoadedConfig.Clients.Candlepin.OverrideExposed = "" }()

	_, client, err := getCandlepinClient(context.Background())
	assert.NoError(t, err)

	assert.Equal(t, "some-secret-value", client.GetConfig().DefaultHeader[overrideExposedHeader])
}

func TestGetCandlepinClientOmitsOverrideExposedHeaderWhenUnset(t *testing.T) {
	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Clients.Candlepin.OverrideExposed = ""

	_, client, err := getCandlepinClient(context.Background())
	assert.NoError(t, err)

	_, present := client.GetConfig().DefaultHeader[overrideExposedHeader]
	assert.False(t, present)
}
