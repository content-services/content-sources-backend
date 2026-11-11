package sonatype_bridge

import (
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
)

type bridgeConfig struct {
	Enabled         bool
	RegistryURL     string
	RegistryOSVURL  string
	RegistryVEXURL  string
	ConsumerGroupID string
	MaxRetries      int
	RequestTimeout  int
	// Registry auth reuses clients.lightwell
	RegistryUsername string
	RegistryPassword string
	// Partner S3 bucket, unrelated to any other object store in the service.
	S3URL          string
	S3Bucket       string
	S3Region       string
	S3AccessKey    string
	S3SecretKey    string
	S3FilePrefix   string
	S3UsePathStyle bool
}

func loadConfig() bridgeConfig {
	cfg := config.Get()

	return bridgeConfig{
		Enabled:          cfg.SonatypeBridge.Enabled,
		RegistryURL:      cfg.SonatypeBridge.RegistryURL,
		RegistryOSVURL:   cfg.SonatypeBridge.RegistryOSVURL,
		RegistryVEXURL:   cfg.SonatypeBridge.RegistryVEXURL,
		ConsumerGroupID:  cfg.SonatypeBridge.ConsumerGroupID,
		MaxRetries:       cfg.SonatypeBridge.MaxRetries,
		RequestTimeout:   cfg.SonatypeBridge.RequestTimeout,
		RegistryUsername: cfg.Clients.Lightwell.Username,
		RegistryPassword: cfg.Clients.Lightwell.Password,
		S3URL:            cfg.SonatypeBridge.S3.URL,
		S3Bucket:         cfg.SonatypeBridge.S3.Name,
		S3Region:         cfg.SonatypeBridge.S3.Region,
		S3AccessKey:      cfg.SonatypeBridge.S3.AccessKey,
		S3SecretKey:      cfg.SonatypeBridge.S3.SecretKey,
		S3FilePrefix:     cfg.SonatypeBridge.S3.FilePrefix,
		S3UsePathStyle:   cfg.SonatypeBridge.UsePathStyle,
	}
}

// validateS3 ensures all partner S3 settings are explicitly configured, so the
// bridge never falls back to ambient AWS credentials or a different bucket.
func (c bridgeConfig) validateS3() error {
	var missing []string
	if c.S3Bucket == "" {
		missing = append(missing, "sonatype_bridge.s3.name")
	}
	if c.S3Region == "" {
		missing = append(missing, "sonatype_bridge.s3.region")
	}
	if c.S3AccessKey == "" {
		missing = append(missing, "sonatype_bridge.s3.access_key")
	}
	if c.S3SecretKey == "" {
		missing = append(missing, "sonatype_bridge.s3.secret_key")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required partner S3 settings: %s", strings.Join(missing, ", "))
	}
	return nil
}
