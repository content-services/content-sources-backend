package pulp_client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListVersionPackagesWithFilters(t *testing.T) {
	var requestQuery url.Values
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		requestQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"count": 1,
			"results": []map[string]string{{
				"name":    "test-package",
				"arch":    "x86_64",
				"version": "1.2.3",
				"release": "4.el9",
				"epoch":   "2",
				"sha256":  "checksum",
				"summary": "test summary",
			}},
		}); err != nil {
			t.Errorf("failed to encode Pulp response: %v", err)
		}
	}))
	defer server.Close()

	configurePackageTest(t, server.URL)

	packages, total, err := GetPulpClientWithDomain("owner-domain").ListVersionPackagesWithFilters(
		context.Background(),
		"/pulp/api/v3/repositories/rpm/versions/1/",
		7,
		25,
		`test "package"`,
		[]string{"-name", "arch"},
	)

	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, packages, 1)
	assert.Equal(t, "test-package", packages[0].GetName())
	assert.Equal(t, "checksum", packages[0].GetSha256())
	assert.Equal(t, "/api/pulp/owner-domain/api/v3/content/rpm/packages/", requestPath)
	assert.Equal(t, "7", requestQuery.Get("offset"))
	assert.Equal(t, "25", requestQuery.Get("limit"))
	assert.Equal(t, "/pulp/api/v3/repositories/rpm/versions/1/", requestQuery.Get("repository_version"))
	assert.Equal(t, "-name,arch", requestQuery.Get("ordering"))
	assert.Equal(t, `test "package"`, requestQuery.Get("name__contains"))
	assert.ElementsMatch(t, RpmFields, requestQuery["fields"])
}

func TestListVersionPackagesWithFiltersWithoutOptionalFilters(t *testing.T) {
	var requestQuery url.Values
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		requestQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"count":   0,
			"results": []map[string]string{},
		}); err != nil {
			t.Errorf("failed to encode Pulp response: %v", err)
		}
	}))
	defer server.Close()

	configurePackageTest(t, server.URL)

	packages, total, err := GetPulpClientWithDomain("owner-domain").ListVersionPackagesWithFilters(
		context.Background(),
		"/pulp/api/v3/repositories/rpm/versions/1/",
		0,
		100,
		"",
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, packages)
	assert.Equal(t, "/api/pulp/owner-domain/api/v3/content/rpm/packages/", requestPath)
	assert.Equal(t, "0", requestQuery.Get("offset"))
	assert.Equal(t, "100", requestQuery.Get("limit"))
	assert.Equal(t, "/pulp/api/v3/repositories/rpm/versions/1/", requestQuery.Get("repository_version"))
	assert.False(t, requestQuery.Has("ordering"))
	assert.False(t, requestQuery.Has("name__contains"))
	assert.ElementsMatch(t, RpmFields, requestQuery["fields"])
}

func TestListVersionPackagesWithFiltersReturnsPulpError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "pulp failure", http.StatusInternalServerError)
	}))
	defer server.Close()

	configurePackageTest(t, server.URL)

	_, _, err := GetPulpClientWithDomain("owner-domain").ListVersionPackagesWithFilters(
		context.Background(), "/pulp/version/1", 0, 100, "", nil,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pulp failure")
}

// TODO: Assert the replacement prefix query parameter after upgrading Pulp and Zest.
func TestSearchVersionPackages(t *testing.T) {
	versions := []string{
		"/api/pulp/owner/api/v3/repositories/rpm/rpm/a/versions/1/",
		"/api/pulp/redhat/api/v3/repositories/rpm/rpm/b/versions/2/",
	}
	packages := []zest.ContentViewPackageResponse{
		{Name: "postgresql", Summary: "PostgreSQL client"},
		{Name: "postgresql-server", Summary: "PostgreSQL server"},
	}
	for _, tc := range []struct {
		name      string
		search    string
		limit     int
		wantLimit int
		total     int
		results   []zest.ContentViewPackageResponse
		wantError string
	}{
		{
			name:   "forwards the dated search limit in one request",
			search: "Postgres & friends", limit: 500, wantLimit: 500, total: 501, results: packages,
		},
		{
			name:  "caps an oversized limit without a name filter",
			limit: 1001, wantLimit: 1000, total: 2, results: packages,
		},
		{
			name:   "no matches",
			search: "missing", limit: 100, wantLimit: 100, results: []zest.ContentViewPackageResponse{},
		},
		{
			name:  "returns the Pulp error",
			limit: 100, wantLimit: 100, wantError: "search failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				query := r.URL.Query()
				assert.Equal(t, "/api/pulp/owner/api/v3/rpm/content-search/packages/", r.URL.Path)
				assert.Equal(t, versions, query["repository_version"])
				assert.Equal(t, strconv.Itoa(tc.wantLimit), query.Get("limit"))
				assert.False(t, query.Has("offset"))
				assert.Equal(t, tc.search, query.Get("name"))
				assert.Equal(t, tc.search != "", query.Has("name"))
				if tc.wantError != "" {
					http.Error(w, tc.wantError, http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				assert.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
					"count": tc.total, "results": tc.results,
				}))
			}))
			defer server.Close()
			configurePackageTest(t, server.URL)

			results, err := GetPulpClientWithDomain("owner").SearchVersionPackages(context.Background(), versions, tc.search, tc.limit)

			assert.Equal(t, 1, calls)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				assert.Nil(t, results)
				return
			}
			require.NoError(t, err)
			require.Len(t, results, len(tc.results))
			for i, expected := range tc.results {
				assert.Equal(t, expected.Name, results[i].Name)
				assert.Equal(t, expected.Summary, results[i].Summary)
			}
		})
	}
}

func TestSearchVersionPackagesEmptyInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("empty input should not make an HTTP request")
	}))
	defer server.Close()
	configurePackageTest(t, server.URL)
	client := GetPulpClientWithDomain("owner")
	for _, tc := range []struct {
		name     string
		versions []string
		limit    int
	}{
		{name: "no versions", limit: 100},
		{name: "zero limit", versions: []string{"/version/1/"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			packages, err := client.SearchVersionPackages(context.Background(), tc.versions, "", tc.limit)
			require.NoError(t, err)
			assert.Empty(t, packages)
		})
	}
}

func configurePackageTest(t *testing.T, serverURL string) {
	t.Helper()
	original := config.Get().Clients.Pulp
	t.Cleanup(func() { config.Get().Clients.Pulp = original })
	config.Get().Clients.Pulp.Server = serverURL
	config.Get().Clients.Pulp.Username = "test-user"
	config.Get().Clients.Pulp.Password = "test-password"
	config.Get().Clients.Pulp.CACert = ""
	config.Get().Clients.Pulp.CACertPath = ""
	config.Get().Clients.Pulp.Proxy = ""
}
