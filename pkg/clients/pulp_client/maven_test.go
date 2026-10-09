package pulp_client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZestHrefPathParam(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "pulp href with leading slash",
			in:   "/api/pulp/lightwell/api/v3/repositories/maven/maven/01a0b664-ece5-70ed-b095-ee51f9515234/",
			want: "api/pulp/lightwell/api/v3/repositories/maven/maven/01a0b664-ece5-70ed-b095-ee51f9515234/",
		},
		{
			name: "already relative",
			in:   "api/pulp/lightwell/api/v3/repositories/maven/maven/abc/",
			want: "api/pulp/lightwell/api/v3/repositories/maven/maven/abc/",
		},
		{
			name: "double leading slash",
			in:   "//api/pulp/lightwell/api/v3/repositories/maven/maven/abc/",
			want: "api/pulp/lightwell/api/v3/repositories/maven/maven/abc/",
		},
		{name: "empty", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, zestHrefPathParam(tt.in))
		})
	}
}

// TestMavenZestCallsDoNotDoubleSlashHref is the regression for pulp HTTP :8080:
// zest concatenates {server}/{href}packages/, and Pulp hrefs already start with /,
// which produced //api/pulp/... (404 on Django, 200 on the TLS proxy at :8443).
func TestMavenZestCallsDoNotDoubleSlashHref(t *testing.T) {
	repoHref := "/api/pulp/lightwell/api/v3/repositories/maven/maven/01a0b664-ece5-70ed-b095-ee51f9515234/"

	for _, tc := range []struct {
		name          string
		trailingSlash bool
	}{
		{name: "server without trailing slash", trailingSlash: false},
		{name: "server with trailing slash", trailingSlash: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requested []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requested = append(requested, r.RequestURI)
				rawPath := r.RequestURI
				if i := strings.IndexByte(rawPath, '?'); i >= 0 {
					rawPath = rawPath[:i]
				}
				if strings.HasPrefix(rawPath, "//") {
					w.Header().Set("Content-Type", "text/html")
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte("<!doctype html><title>Not Found</title>"))
					return
				}

				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(rawPath, "/packages/") || strings.Contains(rawPath, "/packages/?"):
					_, _ = w.Write([]byte(`{"count":0,"next":null,"previous":null,"results":[]}`))
				case strings.HasSuffix(rawPath, "/metrics/") || strings.Contains(rawPath, "/metrics/?"):
					_, _ = w.Write([]byte(`{"package_count":0,"version_count":0,"build_count":0}`))
				default:
					_, _ = w.Write([]byte(`{"name":"java/validated","latest_version_href":"/api/pulp/lightwell/api/v3/repositories/maven/maven/01a0b664-ece5-70ed-b095-ee51f9515234/versions/1/"}`))
				}
			}))
			t.Cleanup(srv.Close)

			pulp := &config.Get().Clients.Pulp
			orig := *pulp
			t.Cleanup(func() { config.Get().Clients.Pulp = orig })
			server := srv.URL
			if tc.trailingSlash {
				server += "/"
			}
			pulp.Server = server
			pulp.Username = "admin"
			pulp.Password = "password"
			pulp.ClientCert = ""
			pulp.ClientKey = ""
			pulp.CACert = ""
			pulp.ClientCertPath = ""
			pulp.ClientKeyPath = ""
			pulp.CACertPath = ""

			impl := getPulpImpl()
			ctx := context.Background()

			_, err := impl.ListMavenPackages(ctx, repoHref, "", 10, 0)
			require.NoError(t, err, "ListMavenPackages request URIs: %v", requested)

			_, err = impl.GetMavenRepositoryMetrics(ctx, repoHref)
			require.NoError(t, err, "GetMavenRepositoryMetrics request URIs: %v", requested)

			_, err = impl.mavenLatestVersionHref(ctx, repoHref)
			require.NoError(t, err, "mavenLatestVersionHref request URIs: %v", requested)

			require.NotEmpty(t, requested)
			for _, uri := range requested {
				path := uri
				if i := strings.IndexByte(path, '?'); i >= 0 {
					path = path[:i]
				}
				assert.Falsef(t, strings.HasPrefix(path, "//"), "zest requested double-slash path %q", uri)
				assert.Truef(t, strings.HasPrefix(path, "/api/pulp/"), "zest requested %q, want /api/pulp/...", uri)
			}
			assert.Contains(t, requested[0], "/packages/")
		})
	}
}

func TestListMavenFlatPackagesPagesByOffset(t *testing.T) {
	repoHref := "/api/pulp/lightwell/api/v3/repositories/maven/maven/01a0b664-ece5-70ed-b095-ee51f9515234/"
	var offsets []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, "300", r.URL.Query().Get("limit"))
		offsets = append(offsets, r.URL.Query().Get("offset"))
		switch r.URL.Query().Get("offset") {
		case "0":
			_, _ = w.Write([]byte(`{"count":2,"next":null,"previous":null,"results":[{"group_id":"org.apache","artifact_id":"commons","version":"1.2.3","last_updated":"2020-01-01T00:00:00Z","description":"upstream","licenses":[{"name":"Apache-2.0","url":""}]}]}`))
		case "300":
			_, _ = w.Write([]byte(`{"count":2,"next":null,"previous":null,"results":[{"group_id":"org.apache","artifact_id":"commons","version":"1.2.3.rhlw-00001","last_updated":"2020-02-01T00:00:00Z","description":"rebuild","licenses":[]}]}`))
		default:
			http.Error(w, "unexpected offset", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	pulp := &config.Get().Clients.Pulp
	orig := *pulp
	t.Cleanup(func() { config.Get().Clients.Pulp = orig })
	pulp.Server = srv.URL
	pulp.Username = "admin"
	pulp.Password = "password"
	pulp.ClientCert = ""
	pulp.ClientKey = ""
	pulp.CACert = ""
	pulp.ClientCertPath = ""
	pulp.ClientKeyPath = ""
	pulp.CACertPath = ""

	impl := getPulpImpl()
	resp, err := impl.ListMavenFlatPackages(context.Background(), repoHref)
	require.NoError(t, err)
	assert.Equal(t, []string{"0", "300"}, offsets)
	require.Len(t, resp.Results, 2)
	assert.Equal(t, "1.2.3", resp.Results[0].Version)
	assert.Equal(t, "1.2.3.rhlw-00001", resp.Results[1].Version)
	assert.Equal(t, int32(2), resp.Count)
}
