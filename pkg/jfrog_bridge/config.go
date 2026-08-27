package jfrog_bridge

import (
	"os"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/rs/zerolog/log"
)

type bridgeConfig struct {
	Enabled         bool
	CatalogURL      string
	CatalogRepo     string
	CatalogToken    string
	RegistryURL     string
	RegistryOSVURL  string
	SigningKeyPEM   string
	SigningKeyAlias string
	ConsumerGroupID string
	MaxRetries      int
	RequestTimeout  int
	// Registry auth reuses clients.lightwell
	RegistryUsername string
	RegistryPassword string
}

func loadConfig() bridgeConfig {
	cfg := config.Get()

	signingKey := cfg.JFrogBridge.SigningKeyPEM
	if signingKey == "" && cfg.JFrogBridge.SigningKeyPath != "" {
		data, err := os.ReadFile(cfg.JFrogBridge.SigningKeyPath)
		if err != nil {
			log.Error().Err(err).Str("path", cfg.JFrogBridge.SigningKeyPath).
				Msg("failed to read signing key file")
		} else {
			signingKey = string(data)
		}
	}

	return bridgeConfig{
		Enabled:          cfg.JFrogBridge.Enabled,
		CatalogURL:       cfg.JFrogBridge.CatalogURL,
		CatalogRepo:      cfg.JFrogBridge.CatalogRepo,
		CatalogToken:     cfg.JFrogBridge.CatalogToken,
		RegistryURL:      cfg.JFrogBridge.RegistryURL,
		RegistryOSVURL:   cfg.JFrogBridge.RegistryOSVURL,
		SigningKeyPEM:    signingKey,
		SigningKeyAlias:  cfg.JFrogBridge.SigningKeyAlias,
		ConsumerGroupID:  cfg.JFrogBridge.ConsumerGroupID,
		MaxRetries:       cfg.JFrogBridge.MaxRetries,
		RequestTimeout:   cfg.JFrogBridge.RequestTimeout,
		RegistryUsername: cfg.Clients.Lightwell.Username,
		RegistryPassword: cfg.Clients.Lightwell.Password,
	}
}
