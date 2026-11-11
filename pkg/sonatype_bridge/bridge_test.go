package sonatype_bridge

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
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
		t.Fatal("WaitGroup should complete immediately when the bridge does not start")
	}
}

func TestStart_Disabled(t *testing.T) {
	config.LoadedConfig.SonatypeBridge = config.SonatypeBridge{
		Enabled: false,
	}
	config.LoadedConfig.Loaded = true

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	Start(ctx, &wg)

	// Should not add to WaitGroup when disabled
	waitGroupDone(t, &wg)
}

func TestStart_MissingS3Config(t *testing.T) {
	config.LoadedConfig.SonatypeBridge = config.SonatypeBridge{
		Enabled: true,
		S3:      config.ObjectStore{Name: "partner-bucket"},
	}
	config.LoadedConfig.Loaded = true

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	Start(ctx, &wg)

	// Should not start the consumer when partner S3 settings are incomplete
	waitGroupDone(t, &wg)
}

func TestLoadConfig(t *testing.T) {
	config.LoadedConfig.SonatypeBridge = config.SonatypeBridge{
		Enabled: true,
		S3: config.ObjectStore{
			URL:        "http://localhost:9002",
			Name:       "partner-bucket",
			Region:     "us-east-1",
			AccessKey:  "partner-access-key",
			SecretKey:  "partner-secret-key",
			FilePrefix: "maven2",
		},
		UsePathStyle:    true,
		RegistryURL:     "https://test.registry/java",
		RegistryOSVURL:  "https://test.registry/osv",
		RegistryVEXURL:  "https://test.registry/vex",
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
	assert.Equal(t, "http://localhost:9002", cfg.S3URL)
	assert.Equal(t, "partner-bucket", cfg.S3Bucket)
	assert.Equal(t, "us-east-1", cfg.S3Region)
	assert.Equal(t, "partner-access-key", cfg.S3AccessKey)
	assert.Equal(t, "partner-secret-key", cfg.S3SecretKey)
	assert.Equal(t, "maven2", cfg.S3FilePrefix)
	assert.True(t, cfg.S3UsePathStyle)
	assert.Equal(t, "https://test.registry/java", cfg.RegistryURL)
	assert.Equal(t, "https://test.registry/osv", cfg.RegistryOSVURL)
	assert.Equal(t, "https://test.registry/vex", cfg.RegistryVEXURL)
	assert.Equal(t, "test-group", cfg.ConsumerGroupID)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 30, cfg.RequestTimeout)
	assert.Equal(t, "user", cfg.RegistryUsername)
	assert.Equal(t, "pass", cfg.RegistryPassword)
}

func TestValidateS3(t *testing.T) {
	complete := bridgeConfig{
		S3Bucket:    "partner-bucket",
		S3Region:    "us-east-1",
		S3AccessKey: "ak",
		S3SecretKey: "sk",
	}
	require.NoError(t, complete.validateS3())

	cases := map[string]func(c *bridgeConfig){
		"sonatype_bridge.s3.name":       func(c *bridgeConfig) { c.S3Bucket = "" },
		"sonatype_bridge.s3.region":     func(c *bridgeConfig) { c.S3Region = "" },
		"sonatype_bridge.s3.access_key": func(c *bridgeConfig) { c.S3AccessKey = "" },
		"sonatype_bridge.s3.secret_key": func(c *bridgeConfig) { c.S3SecretKey = "" },
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			c := complete
			mutate(&c)
			err := c.validateS3()
			require.Error(t, err)
			assert.Contains(t, err.Error(), field)
		})
	}

	_, err := newS3Uploader(bridgeConfig{})
	require.Error(t, err, "uploader must not be created without explicit partner credentials")
}
