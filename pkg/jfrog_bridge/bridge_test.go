package jfrog_bridge

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waitGroupDone(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WaitGroup should complete immediately when no partner starts")
	}
}

func setBridgeConfig(jfrogEnabled bool, sonatype config.SonatypeBridge) {
	config.LoadedConfig.JFrogBridge = config.JFrogBridge{
		Enabled:         jfrogEnabled,
		ConsumerGroupID: "content-sources-jfrog-bridge",
	}
	config.LoadedConfig.SonatypeBridge = sonatype
	config.LoadedConfig.Loaded = true
}

func completeSonatypeBlock(enabled bool) config.SonatypeBridge {
	return config.SonatypeBridge{
		Enabled:         enabled,
		ConsumerGroupID: "content-sources-sonatype-bridge",
		S3: config.ObjectStore{
			Name:      "partner-bucket",
			Region:    "us-east-1",
			AccessKey: "ak",
			SecretKey: "sk",
		},
	}
}

func consumerGroups(consumers []partnerConsumer) map[string]string {
	out := map[string]string{}
	for _, c := range consumers {
		out[c.name] = c.consumerGroupID
	}
	return out
}

func TestStart_AllDisabled(t *testing.T) {
	setBridgeConfig(false, config.SonatypeBridge{Enabled: false})

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	Start(ctx, &wg)

	// Should not add to WaitGroup when every partner is disabled
	waitGroupDone(t, &wg)
}

func TestStart_SonatypeMissingS3Config(t *testing.T) {
	setBridgeConfig(false, config.SonatypeBridge{
		Enabled: true,
		S3:      config.ObjectStore{Name: "partner-bucket"},
	})

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	Start(ctx, &wg)

	// Should not start the consumer when partner S3 settings are incomplete
	waitGroupDone(t, &wg)
}

func TestEnabledPartnerConsumers(t *testing.T) {
	tests := map[string]struct {
		jfrog    bool
		sonatype config.SonatypeBridge
		want     map[string]string
	}{
		"none enabled": {
			jfrog:    false,
			sonatype: completeSonatypeBlock(false),
			want:     map[string]string{},
		},
		"jfrog only": {
			jfrog:    true,
			sonatype: completeSonatypeBlock(false),
			want:     map[string]string{jfrogPartnerName: "content-sources-jfrog-bridge"},
		},
		"sonatype only": {
			jfrog:    false,
			sonatype: completeSonatypeBlock(true),
			want:     map[string]string{sonatypePartnerName: "content-sources-sonatype-bridge"},
		},
		"both enabled": {
			jfrog:    true,
			sonatype: completeSonatypeBlock(true),
			want: map[string]string{
				jfrogPartnerName:    "content-sources-jfrog-bridge",
				sonatypePartnerName: "content-sources-sonatype-bridge",
			},
		},
		"sonatype enabled without S3 settings": {
			jfrog:    true,
			sonatype: config.SonatypeBridge{Enabled: true, ConsumerGroupID: "content-sources-sonatype-bridge"},
			want:     map[string]string{jfrogPartnerName: "content-sources-jfrog-bridge"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			setBridgeConfig(tc.jfrog, tc.sonatype)

			consumers := enabledPartnerConsumers(newBridgeMetrics(prometheus.NewRegistry()))
			assert.Equal(t, tc.want, consumerGroups(consumers))
			for _, c := range consumers {
				require.NotNil(t, c.handler)
				assert.Equal(t, c.name, c.handler.partner.Name())
			}
		})
	}
}

func TestPartnerRegistry(t *testing.T) {
	for _, name := range []string{jfrogPartnerName, sonatypePartnerName, sonarSourcePartnerName} {
		p, ok := lookupPartner(name)
		require.True(t, ok, name)
		assert.Equal(t, name, p.name)
	}
	_, ok := lookupPartner("unknown")
	assert.False(t, ok)
}
