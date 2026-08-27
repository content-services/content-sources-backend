package jfrog_bridge

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestStart_Disabled(t *testing.T) {
	config.LoadedConfig.JFrogBridge = config.JFrogBridge{
		Enabled: false,
	}
	config.LoadedConfig.Loaded = true

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	Start(ctx, &wg)

	// Should not add to WaitGroup when disabled
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WaitGroup should complete immediately when bridge is disabled")
	}
}

func TestLoadConfig(t *testing.T) {
	config.LoadedConfig.JFrogBridge = config.JFrogBridge{
		Enabled:         true,
		CatalogURL:      "https://test.jfrog.io",
		CatalogRepo:     "test-repo",
		CatalogToken:    "test-token",
		RegistryURL:     "https://test.registry/java",
		RegistryOSVURL:  "https://test.registry/osv",
		SigningKeyPEM:   "",
		SigningKeyAlias: "test-alias",
		ConsumerGroupID: "test-group",
		MaxRetries:      5,
		RequestTimeout:  30,
	}
	config.LoadedConfig.Clients.Lightwell = config.Lightwell{
		Username: "user",
		Password: "pass",
	}
	config.LoadedConfig.Loaded = true

	cfg := loadConfig()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, "https://test.jfrog.io", cfg.CatalogURL)
	assert.Equal(t, "test-repo", cfg.CatalogRepo)
	assert.Equal(t, "test-token", cfg.CatalogToken)
	assert.Equal(t, "https://test.registry/java", cfg.RegistryURL)
	assert.Equal(t, "test-alias", cfg.SigningKeyAlias)
	assert.Equal(t, "test-group", cfg.ConsumerGroupID)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 30, cfg.RequestTimeout)
	assert.Equal(t, "user", cfg.RegistryUsername)
	assert.Equal(t, "pass", cfg.RegistryPassword)
}
