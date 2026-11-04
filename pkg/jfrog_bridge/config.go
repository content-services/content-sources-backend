package jfrog_bridge

import (
	"github.com/content-services/content-sources-backend/pkg/config"
)

type bridgeConfig struct {
	Enabled         bool
	CatalogURL      string
	CatalogRepo     string
	CatalogToken    string
	RegistryURL     string
	RegistryOSVURL  string
	RegistryVEXURL  string
	ConsumerGroupID string
	MaxRetries      int
	RequestTimeout  int
	// Registry auth reuses clients.lightwell
	RegistryUsername string
	RegistryPassword string
}

func loadConfig() bridgeConfig {
	cfg := config.Get()

	return bridgeConfig{
		Enabled:          cfg.JFrogBridge.Enabled,
		CatalogURL:       cfg.JFrogBridge.CatalogURL,
		CatalogRepo:      cfg.JFrogBridge.CatalogRepo,
		CatalogToken:     cfg.JFrogBridge.CatalogToken,
		RegistryURL:      cfg.JFrogBridge.RegistryURL,
		RegistryOSVURL:   cfg.JFrogBridge.RegistryOSVURL,
		RegistryVEXURL:   cfg.JFrogBridge.RegistryVEXURL,
		ConsumerGroupID:  cfg.JFrogBridge.ConsumerGroupID,
		MaxRetries:       cfg.JFrogBridge.MaxRetries,
		RequestTimeout:   cfg.JFrogBridge.RequestTimeout,
		RegistryUsername: cfg.Clients.Lightwell.Username,
		RegistryPassword: cfg.Clients.Lightwell.Password,
	}
}
