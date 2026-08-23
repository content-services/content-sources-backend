package jfrog_bridge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRemediations_FullEnvelope(t *testing.T) {
	msg := `{
		"version": "2.0.0",
		"bundle": "lightwell",
		"application": "lightwell",
		"event_type": "java-remediated",
		"timestamp": "2026-08-22T12:00:00.000Z",
		"org_id": "internal",
		"context": {},
		"events": [{
			"metadata": {},
			"payload": {
				"package_name": "org.springframework:spring-core",
				"releases": [{
					"release_names": [{"name": "5.3.18.rhlw-00004"}],
					"related_cve": [{"cve": "CVE-2025-41249", "severity": "important"}]
				}]
			}
		}],
		"recipients": []
	}`

	rems, err := ParseRemediations([]byte(msg))
	require.NoError(t, err)
	require.Len(t, rems, 1)

	assert.Equal(t, "org.springframework", rems[0].GroupID)
	assert.Equal(t, "spring-core", rems[0].ArtifactID)
	assert.Equal(t, "5.3.18.rhlw-00004", rems[0].Version)
	assert.Equal(t, "5.3.18", rems[0].BaseVersion)
	assert.Equal(t, []string{"CVE-2025-41249"}, rems[0].CVEsFixed)
}

func TestParseRemediations_Simplified(t *testing.T) {
	msg := `{
		"package_name": "org.springframework:spring-core",
		"releases": [{"name": "5.3.18.rhlw-00004", "cves_fixed": ["CVE-2025-41249"]}]
	}`

	rems, err := ParseRemediations([]byte(msg))
	require.NoError(t, err)
	require.Len(t, rems, 1)

	assert.Equal(t, "org.springframework", rems[0].GroupID)
	assert.Equal(t, "spring-core", rems[0].ArtifactID)
	assert.Equal(t, "5.3.18.rhlw-00004", rems[0].Version)
	assert.Equal(t, "5.3.18", rems[0].BaseVersion)
	assert.Equal(t, []string{"CVE-2025-41249"}, rems[0].CVEsFixed)
}

func TestParseRemediations_MultipleReleases(t *testing.T) {
	msg := `{
		"package_name": "org.springframework:spring-core",
		"releases": [
			{"name": "5.3.18.rhlw-00003", "cves_fixed": ["CVE-2023-20860"]},
			{"name": "5.3.18.rhlw-00004", "cves_fixed": ["CVE-2025-41249"]}
		]
	}`

	rems, err := ParseRemediations([]byte(msg))
	require.NoError(t, err)
	assert.Len(t, rems, 2)
	assert.Equal(t, "5.3.18.rhlw-00003", rems[0].Version)
	assert.Equal(t, "5.3.18.rhlw-00004", rems[1].Version)
}

func TestParseRemediations_InvalidJSON(t *testing.T) {
	_, err := ParseRemediations([]byte(`{invalid`))
	assert.Error(t, err)
}

func TestParseRemediations_MissingPackageName(t *testing.T) {
	_, err := ParseRemediations([]byte(`{"releases": [{"name": "1.0"}]}`))
	assert.Error(t, err)
}

func TestStripRHLWSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"5.3.18.rhlw-00003", "5.3.18"},
		{"5.3.18.rhlw-00004", "5.3.18"},
		{"2.8.0.rhlw-00001", "2.8.0"},
		{"5.3.18", "5.3.18"},
		{"1.0.0", "1.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, stripRHLWSuffix(tt.input))
		})
	}
}

func TestSplitPackageName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		group     string
		artifact  string
		expectErr bool
	}{
		{"valid", "org.springframework:spring-core", "org.springframework", "spring-core", false},
		{"no colon", "invalid", "", "", true},
		{"empty group", ":artifact", "", "", true},
		{"empty artifact", "group:", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, a, err := splitPackageName(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.group, g)
				assert.Equal(t, tt.artifact, a)
			}
		})
	}
}
