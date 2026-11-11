package config

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleCIDOrgID = `{
  "CID-00000": [
    "20007461",
    "20655920"
  ]
}`

const sampleCIDUsers = `{
  "CID-00000": [
    "content7",
    "mmarusak-et"
  ]
}`

func TestParseClearinghouseMappings(t *testing.T) {
	mapping := ParseClearinghouseMappings(sampleCIDOrgID, sampleCIDUsers)
	assert.Equal(t, "CID-00000", mapping.CIDForOrg("20655920"))
	assert.Equal(t, "CID-00000", mapping.CIDForOrg("20007461"))
	assert.Equal(t, "", mapping.CIDForOrg("other"))
	assert.Equal(t, []string{"20007461", "20655920"}, mapping.OrgIDsForUser("mmarusak-et"))
	assert.Equal(t, []string{"20007461", "20655920"}, mapping.OrgIDsForUser("content7"))
	assert.Empty(t, mapping.OrgIDsForUser("someone-else"))
	assert.True(t, mapping.Allows("mmarusak-et", "20655920"))
	assert.True(t, mapping.Allows("content7", "20007461"))
	assert.False(t, mapping.Allows("mmarusak-et", "other"))
	assert.False(t, mapping.Allows("", "20655920"))
	assert.True(t, mapping.HasOrg("20655920"))
	assert.False(t, mapping.HasOrg("other"))
	assert.True(t, mapping.HasUser("content7"))
	assert.False(t, mapping.HasUser("someone-else"))
	assert.Equal(t, []string{"CID-00000"}, mapping.CIDsForUser("mmarusak-et"))
	assert.Empty(t, mapping.CIDsForUser(""))
}

func TestParseClearinghouseMappingsSingleQuotes(t *testing.T) {
	orgs := "{'CID-00000': ['20007461', '20012872']}"
	users := "{'CID-00000': ['content7', 'mmarusak-et']}"
	mapping := ParseClearinghouseMappings(orgs, users)
	assert.Equal(t, "CID-00000", mapping.CIDForOrg("20007461"))
	assert.Equal(t, "CID-00000", mapping.CIDForOrg("20012872"))
	assert.Equal(t, []string{"20007461", "20012872"}, mapping.OrgIDsForUser("content7"))
	assert.True(t, mapping.Allows("mmarusak-et", "20012872"))
}

func TestParseClearinghouseMappingsEmpty(t *testing.T) {
	assert.Equal(t, ClearinghouseMapping{}, ParseClearinghouseMappings("", ""))
	assert.Equal(t, ClearinghouseMapping{}, ParseClearinghouseMappings("   ", "   "))
	assert.Equal(t, "", ParseClearinghouseMappings("not json", sampleCIDUsers).CIDForOrg("20655920"))
	assert.Empty(t, ParseClearinghouseMappings(sampleCIDOrgID, "not json").OrgIDsForUser("mmarusak-et"))
}

func TestParseClearinghouseMappingsRepeatedCID(t *testing.T) {
	logs := captureClearinghouseMappingLogs(t)
	orgs := `{"CID-00000": ["111"], "CID-00000": ["20655920", "20007461"]}`
	users := `{"CID-00000": ["mmarusak-et"], "CID-00000": ["content7"]}`
	mapping := ParseClearinghouseMappings(orgs, users)
	assert.Equal(t, "CID-00000", mapping.CIDForOrg("20655920"))
	assert.Equal(t, "CID-00000", mapping.CIDForOrg("20007461"))
	assert.Equal(t, "", mapping.CIDForOrg("111"))
	assert.True(t, mapping.Allows("content7", "20655920"))
	assert.False(t, mapping.Allows("mmarusak-et", "20655920"))

	logged := logs.String()
	assert.Contains(t, logged, "CLEARINGHOUSE MAPPING WARNING: CID is repeated")
	assert.Contains(t, logged, "cid_org_id")
	assert.Contains(t, logged, "cid_user_mapping")
	assert.Contains(t, logged, "CID-00000")
}

func TestParseClearinghouseMappingsRepeatedOrg(t *testing.T) {
	logs := captureClearinghouseMappingLogs(t)
	orgs := `{"CID-00002": ["20655920"], "CID-00001": ["20655920", "other"]}`
	users := `{"CID-00001": ["mmarusak-et"], "CID-00002": ["content7"]}`
	mapping := ParseClearinghouseMappings(orgs, users)
	assert.Equal(t, "CID-00001", mapping.CIDForOrg("20655920"))
	assert.Equal(t, "CID-00001", mapping.CIDForOrg("other"))
	assert.True(t, mapping.Allows("mmarusak-et", "20655920"))
	assert.False(t, mapping.Allows("content7", "20655920"))
	assert.Equal(t, []string{"20655920", "other"}, mapping.OrgIDsForUser("mmarusak-et"))
	assert.Empty(t, mapping.OrgIDsForUser("content7"))
	assert.Contains(t, logs.String(), "org ID is assigned to more than one CID")
}

func TestParseClearinghouseMappingsMultipleCIDs(t *testing.T) {
	orgs := `{"CID-00002": ["org-b", "org-c"], "CID-00001": ["org-a"]}`
	users := `{"CID-00001": ["mmarusak-et"], "CID-00002": ["mmarusak-et", "content7"]}`
	mapping := ParseClearinghouseMappings(orgs, users)
	assert.Equal(t, []string{"org-a", "org-b", "org-c"}, mapping.OrgIDsForUser("mmarusak-et"))
	assert.Equal(t, []string{"org-b", "org-c"}, mapping.OrgIDsForUser("content7"))
}

func TestJSONObjectKeysRepeated(t *testing.T) {
	keys, err := jsonObjectKeys([]byte(`{"CID-00000":["1"],"CID-00001":{"nested":[1,2]},"CID-00000":["2"]}`))
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
