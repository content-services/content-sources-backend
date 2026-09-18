package pulp_client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
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

	pulpConfig := config.Get().Clients.Pulp
	config.Get().Clients.Pulp.Server = server.URL
	config.Get().Clients.Pulp.Username = "test-user"
	config.Get().Clients.Pulp.Password = "test-password"
	config.Get().Clients.Pulp.CACert = ""
	config.Get().Clients.Pulp.CACertPath = ""
	config.Get().Clients.Pulp.Proxy = ""
	t.Cleanup(func() { config.Get().Clients.Pulp = pulpConfig })

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

	pulpConfig := config.Get().Clients.Pulp
	config.Get().Clients.Pulp.Server = server.URL
	config.Get().Clients.Pulp.Username = "test-user"
	config.Get().Clients.Pulp.Password = "test-password"
	config.Get().Clients.Pulp.CACert = ""
	config.Get().Clients.Pulp.CACertPath = ""
	config.Get().Clients.Pulp.Proxy = ""
	t.Cleanup(func() { config.Get().Clients.Pulp = pulpConfig })

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

	pulpConfig := config.Get().Clients.Pulp
	config.Get().Clients.Pulp.Server = server.URL
	config.Get().Clients.Pulp.Username = "test-user"
	config.Get().Clients.Pulp.Password = "test-password"
	config.Get().Clients.Pulp.CACert = ""
	config.Get().Clients.Pulp.CACertPath = ""
	config.Get().Clients.Pulp.Proxy = ""
	t.Cleanup(func() { config.Get().Clients.Pulp = pulpConfig })

	_, _, err := GetPulpClientWithDomain("owner-domain").ListVersionPackagesWithFilters(
		context.Background(), "/pulp/version/1", 0, 100, "", nil,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pulp failure")
}
