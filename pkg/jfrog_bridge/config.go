package jfrog_bridge

import (
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
)

// registryConfig holds the Lightwell registry settings shared by every partner.
type registryConfig struct {
	RegistryURL    string
	RegistryOSVURL string
	RegistryVEXURL string
	MaxRetries     int
	RequestTimeout int
	// Registry auth reuses clients.lightwell
	RegistryUsername string
	RegistryPassword string
}

type jfrogConfig struct {
	registryConfig
	Enabled         bool
	CatalogURL      string
	CatalogRepo     string
	CatalogToken    string
	ConsumerGroupID string
}

type sonatypeConfig struct {
	registryConfig
	Enabled         bool
	ConsumerGroupID string
	// Partner S3 bucket, unrelated to any other object store in the service.
	S3URL          string
	S3Bucket       string
	S3Region       string
	S3AccessKey    string
	S3SecretKey    string
	S3UsePathStyle bool
}

func loadJFrogConfig() jfrogConfig {
	cfg := config.Get()

	return jfrogConfig{
		registryConfig: registryConfig{
			RegistryURL:      cfg.JFrogBridge.RegistryURL,
			RegistryOSVURL:   cfg.JFrogBridge.RegistryOSVURL,
			RegistryVEXURL:   cfg.JFrogBridge.RegistryVEXURL,
			MaxRetries:       cfg.JFrogBridge.MaxRetries,
			RequestTimeout:   cfg.JFrogBridge.RequestTimeout,
			RegistryUsername: cfg.Clients.Lightwell.Username,
			RegistryPassword: cfg.Clients.Lightwell.Password,
		},
		Enabled:         cfg.JFrogBridge.Enabled,
		CatalogURL:      cfg.JFrogBridge.CatalogURL,
		CatalogRepo:     cfg.JFrogBridge.CatalogRepo,
		CatalogToken:    cfg.JFrogBridge.CatalogToken,
		ConsumerGroupID: cfg.JFrogBridge.ConsumerGroupID,
	}
}

func loadSonatypeConfig() sonatypeConfig {
	cfg := config.Get()

	return sonatypeConfig{
		registryConfig: registryConfig{
			RegistryURL:      cfg.SonatypeBridge.RegistryURL,
			RegistryOSVURL:   cfg.SonatypeBridge.RegistryOSVURL,
			RegistryVEXURL:   cfg.SonatypeBridge.RegistryVEXURL,
			MaxRetries:       cfg.SonatypeBridge.MaxRetries,
			RequestTimeout:   cfg.SonatypeBridge.RequestTimeout,
			RegistryUsername: cfg.Clients.Lightwell.Username,
			RegistryPassword: cfg.Clients.Lightwell.Password,
		},
		Enabled:         cfg.SonatypeBridge.Enabled,
		ConsumerGroupID: cfg.SonatypeBridge.ConsumerGroupID,
		S3URL:           cfg.SonatypeBridge.S3.URL,
		S3Bucket:        cfg.SonatypeBridge.S3.Name,
		S3Region:        cfg.SonatypeBridge.S3.Region,
		S3AccessKey:     cfg.SonatypeBridge.S3.AccessKey,
		S3SecretKey:     cfg.SonatypeBridge.S3.SecretKey,
		S3UsePathStyle:  cfg.SonatypeBridge.UsePathStyle,
	}
}

// validateS3 ensures all partner S3 settings are explicitly configured, so the
// bridge never falls back to ambient AWS credentials or a different bucket.
func (c sonatypeConfig) validateS3() error {
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
