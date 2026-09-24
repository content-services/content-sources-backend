package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLightwellAdvisoryName(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"x_RHLW-CVE-2021-37714-1.7.2", "CVE-2021-37714"},
		{"x_RHLW-LW-2026-4255-2.11.0.rhlw-00000-n-00008", "LW-2026-4255"},
		{"x_RHLW-CVE-2015-6748-1.7.2", "CVE-2015-6748"},
		{"FAKE-ADVISORY-001-1.0", "FAKE-ADVISORY-001-1.0"},
		{"CVE-2024-1234", "CVE-2024-1234"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			assert.Equal(t, tt.want, LightwellAdvisoryName(tt.id))
		})
	}
}
