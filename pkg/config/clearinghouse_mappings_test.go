package config

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleClearinghouseMappings = `{
  "CidOrgId": {
    "CID-00000": "20655920"
  },
  "CidTam": {
    "CID-00000": [
      "lightwell-jsherril",
      "lightwell-mmarusak"
    ]
  }
}`

func TestParseClearinghouseMappings(t *testing.T) {
	mapping := ParseClearinghouseMappings(sampleClearinghouseMappings)
	assert.Equal(t, "CID-00000", mapping.CIDForOrg("20655920"))
	assert.Equal(t, "", mapping.CIDForOrg("other"))
	assert.Equal(t, []string{"20655920"}, mapping.OrgIDsForUser("lightwell-mmarusak"))
	assert.Equal(t, []string{"20655920"}, mapping.OrgIDsForUser("lightwell-jsherril"))
	assert.Empty(t, mapping.OrgIDsForUser("someone-else"))
	assert.True(t, mapping.Allows("lightwell-mmarusak", "20655920"))
	assert.False(t, mapping.Allows("lightwell-mmarusak", "other"))
	assert.False(t, mapping.Allows("", "20655920"))
}

func TestParseClearinghouseMappingsEmpty(t *testing.T) {
	assert.Equal(t, ClearinghouseMapping{}, ParseClearinghouseMappings(""))
	assert.Equal(t, ClearinghouseMapping{}, ParseClearinghouseMappings("   "))
	assert.Equal(t, "", ParseClearinghouseMappings("not json").CIDForOrg("20655920"))
}

func TestParseClearinghouseMappingsRepeatedCID(t *testing.T) {
	logs := captureClearinghouseMappingLogs(t)
	raw := `{
		"CidOrgId": {"CID-00000": "111", "CID-00000": "20655920"},
		"CidTam": {"CID-00000": ["lightwell-mmarusak"], "CID-00000": ["lightwell-jsherril"]}
	}`
	mapping := ParseClearinghouseMappings(raw)
	assert.Equal(t, "CID-00000", mapping.CIDForOrg("20655920"))
	assert.Equal(t, "", mapping.CIDForOrg("111"))
	assert.True(t, mapping.Allows("lightwell-jsherril", "20655920"))
	assert.False(t, mapping.Allows("lightwell-mmarusak", "20655920"))

	logged := logs.String()
	assert.Contains(t, logged, "CLEARINGHOUSE MAPPING WARNING: CID is repeated")
	assert.Contains(t, logged, "CidOrgId")
	assert.Contains(t, logged, "CidTam")
	assert.Contains(t, logged, "CID-00000")
}

func TestParseClearinghouseMappingsRepeatedOrg(t *testing.T) {
	logs := captureClearinghouseMappingLogs(t)
	raw := `{
		"CidOrgId": {"CID-00002": "20655920", "CID-00001": "20655920"},
		"CidTam": {"CID-00001": ["lightwell-mmarusak"], "CID-00002": ["lightwell-jsherril"]}
	}`
	mapping := ParseClearinghouseMappings(raw)
	assert.Equal(t, "CID-00001", mapping.CIDForOrg("20655920"))
	assert.True(t, mapping.Allows("lightwell-mmarusak", "20655920"))
	assert.False(t, mapping.Allows("lightwell-jsherril", "20655920"))
	assert.Equal(t, []string{"20655920"}, mapping.OrgIDsForUser("lightwell-mmarusak"))
	assert.Empty(t, mapping.OrgIDsForUser("lightwell-jsherril"))
	assert.Contains(t, logs.String(), "org ID is assigned to more than one CID")
}

func TestParseClearinghouseMappingsMultipleCIDs(t *testing.T) {
	raw := `{
		"CidOrgId": {"CID-00002": "org-b", "CID-00001": "org-a"},
		"CidTam": {"CID-00001": ["lightwell-mmarusak"], "CID-00002": ["lightwell-mmarusak", "lightwell-jsherril"]}
	}`
	mapping := ParseClearinghouseMappings(raw)
	assert.Equal(t, []string{"org-a", "org-b"}, mapping.OrgIDsForUser("lightwell-mmarusak"))
	assert.Equal(t, []string{"org-b"}, mapping.OrgIDsForUser("lightwell-jsherril"))
}

func TestJSONObjectKeysRepeated(t *testing.T) {
	keys, err := jsonObjectKeys([]byte(`{"CID-00000":"1","CID-00001":{"nested":[1,2]},"CID-00000":"2"}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"CID-00000", "CID-00001", "CID-00000"}, keys)
}

func captureClearinghouseMappingLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() {
		log.Logger = previous
	})
	return &buf
}
