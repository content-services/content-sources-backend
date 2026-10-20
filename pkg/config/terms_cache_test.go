package config

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTermsCacheConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		ttlEnv      string
		versionEnv  string
		wantTTL     time.Duration
		wantVersion string
	}{
		{
			name:        "defaults",
			wantTTL:     24 * time.Hour,
			wantVersion: "1",
		},
		{
			name:        "YAML duration and version",
			yaml:        "clients:\n  redis:\n    expiration:\n      terms: 45m\n  terms_service:\n    cache_version: \"2\"\n",
			wantTTL:     45 * time.Minute,
			wantVersion: "2",
		},
		{
			name:        "YAML zero disables caching",
			yaml:        "clients:\n  redis:\n    expiration:\n      terms: 0s\n",
			wantVersion: "1",
		},
		{
			name:        "YAML negative disables caching",
			yaml:        "clients:\n  redis:\n    expiration:\n      terms: -1h\n",
			wantTTL:     -time.Hour,
			wantVersion: "1",
		},
		{
			name:        "environment overrides YAML including explicit zero",
			yaml:        "clients:\n  redis:\n    expiration:\n      terms: 24h\n  terms_service:\n    cache_version: \"1\"\n",
			ttlEnv:      "0s",
			versionEnv:  "release-2",
			wantVersion: "release-2",
		},
		{
			name:        "environment positive duration",
			ttlEnv:      "168h",
			versionEnv:  "3",
			wantTTL:     7 * 24 * time.Hour,
			wantVersion: "3",
		},
		{
			name:        "environment negative duration",
			ttlEnv:      "-5s",
			wantTTL:     -5 * time.Second,
			wantVersion: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLIENTS_REDIS_EXPIRATION_TERMS", tt.ttlEnv)
			t.Setenv("CLIENTS_TERMS_SERVICE_CACHE_VERSION", tt.versionEnv)

			// Match Load's Viper setup without changing the global configuration.
			v := viper.New()
			v.SetConfigType("yaml")
			require.NoError(t, v.ReadConfig(strings.NewReader(tt.yaml)))
			v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
			v.AutomaticEnv()
			setDefaults(v)

			var loaded Configuration
			require.NoError(t, v.Unmarshal(&loaded))
			assert.Equal(t, tt.wantTTL, loaded.Clients.Redis.Expiration.Terms)
			assert.Equal(t, tt.wantVersion, loaded.Clients.TermsService.CacheVersion)
		})
	}
}
