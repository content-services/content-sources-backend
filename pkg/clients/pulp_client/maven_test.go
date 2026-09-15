package pulp_client

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMavenPackagesQuery(t *testing.T) {
	tests := []struct {
		name   string
		search string
		limit  int
		offset int
	}{
		{name: "empty search", search: "", limit: 100, offset: 0},
		{name: "plain search", search: "jackson", limit: 50, offset: 10},
		{name: "group artifact search", search: "com.fasterxml.jackson.core:jackson-databind", limit: 200, offset: 0},
		{name: "whitespace search", search: "   ", limit: 100, offset: 0},
		{name: "lone colon", search: ":", limit: 100, offset: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := mavenPackagesQuery(tt.search, tt.limit, tt.offset)
			assert.Equal(t, tt.search, q.Get("search"))
			limit, err := strconv.Atoi(q.Get("limit"))
			require.NoError(t, err)
			offset, err := strconv.Atoi(q.Get("offset"))
			require.NoError(t, err)
			assert.Equal(t, tt.limit, limit)
			assert.Equal(t, tt.offset, offset)
			assert.Equal(t, "group_id,artifact_id", q.Get("ordering"))
			assert.Empty(t, q.Get("group_id__istartswith"))
			assert.Empty(t, q.Get("artifact_id__istartswith"))
			assert.Empty(t, q.Get("version__istartswith"))
		})
	}
}

func TestMavenRepoResourceURL(t *testing.T) {
	got, err := mavenRepoResourceURL("https://pulp.example", "/api/pulp/default/api/v3/repositories/maven/maven/abc/", "packages/")
	require.NoError(t, err)
	assert.Equal(t, "https://pulp.example//api/pulp/default/api/v3/repositories/maven/maven/abc/packages/", got)
}
