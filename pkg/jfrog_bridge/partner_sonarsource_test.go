package jfrog_bridge

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSonarSource_DisabledByDefault(t *testing.T) {
	setBridgeConfig(true, completeSonatypeBlock(true))

	assert.False(t, sonarSourcePartner.enabled())
	assert.Equal(t, "content-sources-sonarsource-bridge", sonarSourcePartner.consumerGroupID())
	assert.False(t, sonarSourcePartner.adminFeature().Enabled)

	consumers := enabledPartnerConsumers(newBridgeMetrics(prometheus.NewRegistry()))
	assert.NotContains(t, consumerGroups(consumers), sonarSourcePartnerName, "the SonarSource consumer must not start")
}

func TestSonarSource_ProcessNeverSucceeds(t *testing.T) {
	handler, err := sonarSourcePartner.newHandler()
	require.NoError(t, err)
	assert.Equal(t, sonarSourcePartnerName, handler.Name())

	err = handler.Process(context.Background(), springRemediation())
	assert.ErrorIs(t, err, errSonarSourceContractUndefined)

	healthy, _ := handler.Health(context.Background())
	assert.False(t, healthy)
}
