package custom

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewCollector(t *testing.T) {
	var c *Collector
	if db.DB == nil {
		err := db.Connect()
		require.NoError(t, err)
	}

	pulp := pulp_client.NewMockPulpGlobalClient(t)

	// Success case
	reg := prometheus.NewRegistry()
	metrics := instrumentation.NewMetrics(reg)
	c = NewCollector(context.Background(), metrics, db.DB, pulp)
	assert.NotNil(t, c)

	// Forcing nil Context
	//nolint:staticcheck
	c = NewCollector(nil, metrics, db.DB, pulp)
	assert.Nil(t, c)

	// metrics nil
	c = NewCollector(context.Background(), nil, db.DB, pulp)
	assert.Nil(t, c)

	// db nil
	c = NewCollector(context.Background(), metrics, nil, pulp)
	assert.Nil(t, c)
}

func TestIterateNoPanic(t *testing.T) {
	var c *Collector
	if db.DB == nil {
		err := db.Connect()
		require.NoError(t, err)
	}

	// Success case
	reg := prometheus.NewRegistry()
	metrics := instrumentation.NewMetrics(reg)
	pulp := pulp_client.NewMockPulpGlobalClient(t)
	pulp.On("Livez", mock.AnythingOfType("*context.valueCtx")).Return(nil)
	c = NewCollector(context.Background(), metrics, db.DB, pulp)
	require.NotNil(t, c)

	assert.NotPanics(t, func() {
		c.iterate()
	})
}
