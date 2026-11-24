package pulp_client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pythonTestBasePath = "python/remediated"
	pythonTestName     = "lightwell-fixture"
	pythonTestRepo     = "/api/pulp/lightwell/api/v3/repositories/python/python/repository/"
	pythonTestVersion  = pythonTestRepo + "versions/4/"
	pythonTestContent  = "/api/pulp/lightwell/api/v3/content/python/packages/"
)

func TestListPythonPackageFilesExactReleaseAndPagination(t *testing.T) {
	build1 := pythonInventoryFixture(t, "rhlw-1")
	build2 := pythonInventoryFixture(t, "rhlw-2")
	platformWheel := build2[0]
	platformWheel.PulpHref = zest.PtrString(pythonTestContent + "platform-wheel/")
	platformWheel.Artifact = zest.PtrString("/api/pulp/lightwell/api/v3/artifacts/platform-wheel/")
	platformWheel.Filename = zest.PtrString("lightwell_fixture-3.0.1+rhlw.2-cp313-cp313-manylinux_2_17_x86_64.whl")
	build2 = append(build2, platformWheel)
	all := append(append([]zest.PythonPythonPackageContentResponse{}, build1...), build2...)

	for _, tc := range []struct {
		version string
		want    []zest.PythonPythonPackageContentResponse
	}{
		{version: "3.0.1+rhlw.1", want: build1},
		{version: "3.0.1+rhlw.2", want: build2},
		{version: "3.0.1+rhlw.999", want: []zest.PythonPythonPackageContentResponse{}},
		{version: "3.0.1", want: []zest.PythonPythonPackageContentResponse{}},
	} {
		t.Run(tc.version, func(t *testing.T) {
			var offsets []string
			repositoryReads := 0
			client := pythonInventoryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/repositories/") {
					repositoryReads++
				}
				if servePythonInventoryMapping(t, w, r) {
					return
				}
				assert.Equal(t, pythonTestContent, r.URL.Path)
				query := r.URL.Query()
				assert.Equal(t, pythonTestVersion, query.Get("repository_version"))
				assert.Equal(t, pythonTestName, query.Get("name"))
				assert.Equal(t, tc.version, query.Get("version"))
				assert.NotContains(t, query, "version__startswith")
				assert.Equal(t, "filename", query.Get("ordering"))
				assert.Equal(t, "100", query.Get("limit"))
				assert.ElementsMatch(t, []string{"pulp_href", "artifact", "name", "version", "filename", "packagetype", "size", "sha256"}, query["fields"])
				if strings.Contains(tc.version, "+") {
					assert.Contains(t, r.URL.RawQuery, "%2B")
				}
				user, password, ok := r.BasicAuth()
				assert.True(t, ok)
				assert.Equal(t, "test-user", user)
				assert.Equal(t, "test-password", password)
				offsets = append(offsets, query.Get("offset"))
				offset, err := strconv.Atoi(query.Get("offset"))
				assert.NoError(t, err)
				filtered := []zest.PythonPythonPackageContentResponse{}
				for _, file := range all {
					if file.GetName() == query.Get("name") && file.GetVersion() == query.Get("version") {
						filtered = append(filtered, file)
					}
				}
				page := pythonInventoryPage(len(filtered))
				if offset < len(filtered) {
					// Simulate a server cap smaller than the requested limit.
					page["results"] = filtered[offset : offset+1]
					if offset+1 < len(filtered) {
						page["next"] = "https://untrusted.invalid/other-repository/?version=wrong"
					}
				}
				writePythonInventoryJSON(t, w, page)
			})

			files, err := client.ListPythonPackageFiles(context.Background(), pythonTestBasePath, pythonTestName, tc.version)
			require.NoError(t, err)
			sort.Slice(tc.want, func(i, j int) bool { return tc.want[i].GetFilename() < tc.want[j].GetFilename() })
			assert.Equal(t, tc.want, files)
			assert.Equal(t, 1, repositoryReads, "resolve the snapshot only once")
			if len(files) == 0 {
				assert.Equal(t, []string{"0"}, offsets)
			} else {
				require.Len(t, offsets, len(files))
				for i := range offsets {
					assert.Equal(t, strconv.Itoa(i), offsets[i])
				}
			}
		})
	}
}

func TestListPythonPackageFilesEquivalentNames(t *testing.T) {
	for _, storedName := range []string{
		"Lightwell-Fixture",
		"lightwell_fixture",
		"lightwell.fixture",
		"Lightwell__.-Fixture",
	} {
		t.Run(storedName, func(t *testing.T) {
			files := pythonInventoryFixture(t, "rhlw-2")
			for i := range files {
				files[i].Name = zest.PtrString(storedName)
			}
			client := pythonInventoryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if servePythonInventoryMapping(t, w, r) {
					return
				}
				assert.Equal(t, pythonTestContent, r.URL.Path)
				assert.Equal(t, pythonTestName, r.URL.Query().Get("name"))
				assert.Equal(t, "3.0.1+rhlw.2", r.URL.Query().Get("version"))
				// Pulp filters by normalized name but preserves the metadata name.
				writePythonInventoryJSON(t, w, pythonInventoryPage(len(files), files...))
			})

			got, err := client.ListPythonPackageFiles(context.Background(), pythonTestBasePath, pythonTestName, "3.0.1+rhlw.2")
			require.NoError(t, err)
			assert.Equal(t, files, got, "preserve the original metadata names")
		})
	}
}

func TestListPythonPackageFilesDistributionMapping(t *testing.T) {
	for _, tc := range []struct {
		name         string
		distribution map[string]any
		repositories []map[string]any
		wantVersion  string
		wantError    string
	}{
		{name: "missing distribution", wantError: "distribution not found"},
		{name: "missing repository mapping", distribution: map[string]any{}, wantError: "no repository mapping"},
		{name: "missing repository", distribution: map[string]any{"repository": pythonTestRepo}, wantError: "no latest version"},
		{name: "missing latest version", distribution: map[string]any{"repository": pythonTestRepo}, repositories: []map[string]any{{"name": "python/validated"}}, wantError: "no latest version"},
		{name: "pinned distribution", distribution: map[string]any{"repository_version": pythonTestVersion, "repository": pythonTestRepo}, wantVersion: pythonTestVersion},
		{name: "another repository", distribution: map[string]any{"repository": pythonTestRepo + "other/"}, repositories: []map[string]any{{"name": "python/validated", "latest_version_href": pythonTestRepo + "other/versions/9/"}}, wantVersion: pythonTestRepo + "other/versions/9/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inventoryCalls := 0
			repositoryCalls := 0
			client := pythonInventoryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/distributions/"):
					assert.Equal(t, "python/validated", r.URL.Query().Get("base_path"))
					results := []map[string]any{}
					if tc.distribution != nil {
						tc.distribution["base_path"] = "python/validated"
						tc.distribution["name"] = "python/validated"
						results = append(results, tc.distribution)
					}
					writePythonInventoryJSON(t, w, map[string]any{"count": len(results), "results": results})
				case strings.HasSuffix(r.URL.Path, "/repositories/"):
					repositoryCalls++
					assert.Equal(t, tc.distribution["repository"], r.URL.Query().Get("pulp_href__in"))
					results := tc.repositories
					if results == nil {
						results = []map[string]any{}
					}
					writePythonInventoryJSON(t, w, map[string]any{"count": len(results), "results": results})
				case r.URL.Path == pythonTestContent:
					inventoryCalls++
					assert.Equal(t, tc.wantVersion, r.URL.Query().Get("repository_version"))
					writePythonInventoryJSON(t, w, pythonInventoryPage(0))
				default:
					t.Errorf("unexpected request: %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			})
			files, err := client.ListPythonPackageFiles(context.Background(), "python/validated", pythonTestName, "3.0.1+rhlw.2")
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				assert.Nil(t, files)
				assert.Zero(t, inventoryCalls)
			} else {
				require.NoError(t, err)
				assert.Empty(t, files)
				assert.Equal(t, 1, inventoryCalls)
			}
			if tc.name == "pinned distribution" {
				assert.Zero(t, repositoryCalls, "use the published snapshot rather than latest")
			}
		})
	}
}

func TestListPythonPackageFilesMetadataAndDuplicates(t *testing.T) {
	files := pythonInventoryFixture(t, "rhlw-2")
	files[0].Size = nil
	files[0].Sha256 = nil
	files[0].Packagetype = nil
	files[1].Size = zest.PtrInt64(0)
	files[1].Sha256 = zest.PtrString("")
	files[1].Packagetype = zest.PtrString("unfamiliar-distribution")
	unknown := files[1]
	unknown.PulpHref = zest.PtrString(pythonTestContent + "unknown/")
	unknown.Filename = zest.PtrString("A-unfamiliar.distribution")
	unknown.Sha256 = zest.PtrString(strings.Repeat("AB", 32))
	files = append(files, unknown, files[0])
	client := pythonInventoryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !servePythonInventoryMapping(t, w, r) {
			writePythonInventoryJSON(t, w, pythonInventoryPage(len(files), files...))
		}
	})

	got, err := client.ListPythonPackageFiles(context.Background(), pythonTestBasePath, pythonTestName, "3.0.1+rhlw.2")
	require.NoError(t, err)
	require.Len(t, got, 3, "collapse identical content records and retain unknown distributions")
	assert.Equal(t, "A-unfamiliar.distribution", got[0].GetFilename())
	assert.Equal(t, strings.Repeat("ab", 32), got[0].GetSha256())
	assert.Equal(t, "unfamiliar-distribution", got[0].GetPackagetype())
	assert.Nil(t, got[1].Size)
	assert.Nil(t, got[1].Sha256)
	assert.Nil(t, got[1].Packagetype)
	require.NotNil(t, got[2].Size)
	assert.Zero(t, *got[2].Size)
	assert.Nil(t, got[2].Sha256)
}

func TestListPythonPackageFilesDuplicateContentFields(t *testing.T) {
	for _, tc := range []struct {
		name      string
		change    func(*zest.PythonPythonPackageContentResponse)
		wantError bool
	}{
		{name: "unrequested metadata", change: func(f *zest.PythonPythonPackageContentResponse) {
			f.Summary = zest.PtrString("updated package summary")
			f.Prn = zest.PtrString("prn:content:extra-metadata")
		}},
		{name: "equivalent digest", change: func(f *zest.PythonPythonPackageContentResponse) {
			f.Sha256 = zest.PtrString(strings.ToUpper(f.GetSha256()))
		}},
		{name: "different artifact", wantError: true, change: func(f *zest.PythonPythonPackageContentResponse) {
			f.Artifact = zest.PtrString("/api/pulp/lightwell/api/v3/artifacts/other/")
		}},
		{name: "different package type", wantError: true, change: func(f *zest.PythonPythonPackageContentResponse) {
			f.Packagetype = zest.PtrString("unfamiliar-distribution")
		}},
		{name: "different digest", wantError: true, change: func(f *zest.PythonPythonPackageContentResponse) {
			f.Sha256 = zest.PtrString(strings.Repeat("ab", 32))
		}},
		{name: "unknown size versus zero bytes", wantError: true, change: func(f *zest.PythonPythonPackageContentResponse) {
			f.Size = nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := pythonInventoryFixture(t, "rhlw-2")[0]
			file.Size = zest.PtrInt64(0)
			duplicate := file
			tc.change(&duplicate)
			client := pythonInventoryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if !servePythonInventoryMapping(t, w, r) {
					writePythonInventoryJSON(t, w, pythonInventoryPage(2, file, duplicate))
				}
			})

			got, err := client.ListPythonPackageFiles(context.Background(), pythonTestBasePath, pythonTestName, "3.0.1+rhlw.2")
			if tc.wantError {
				require.ErrorContains(t, err, "conflicting Python inventory content records")
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []zest.PythonPythonPackageContentResponse{file}, got)
			}
		})
	}
}

func TestListPythonPackageFilesRejectsInvalidRecords(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*zest.PythonPythonPackageContentResponse)
	}{
		{name: "missing filename", change: func(f *zest.PythonPythonPackageContentResponse) { f.Filename = nil }},
		{name: "missing identity", change: func(f *zest.PythonPythonPackageContentResponse) { f.PulpHref = nil }},
		{name: "negative size", change: func(f *zest.PythonPythonPackageContentResponse) { f.Size = zest.PtrInt64(-1) }},
		{name: "short digest", change: func(f *zest.PythonPythonPackageContentResponse) { f.Sha256 = zest.PtrString("abcd") }},
		{name: "invalid digest", change: func(f *zest.PythonPythonPackageContentResponse) { f.Sha256 = zest.PtrString(strings.Repeat("z", 64)) }},
		{name: "wrong package", change: func(f *zest.PythonPythonPackageContentResponse) { f.Name = zest.PtrString("another-package") }},
		{name: "wrong build", change: func(f *zest.PythonPythonPackageContentResponse) { f.Version = zest.PtrString("3.0.1+rhlw.1") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := pythonInventoryFixture(t, "rhlw-2")
			tc.change(&files[1])
			client := pythonInventoryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if !servePythonInventoryMapping(t, w, r) {
					writePythonInventoryJSON(t, w, pythonInventoryPage(len(files), files...))
				}
			})
			got, err := client.ListPythonPackageFiles(context.Background(), pythonTestBasePath, pythonTestName, "3.0.1+rhlw.2")
			require.Error(t, err)
			assert.Nil(t, got, "discard preceding valid records too")
		})
	}
}

func TestListPythonPackageFilesPageFailures(t *testing.T) {
	files := pythonInventoryFixture(t, "rhlw-2")
	conflict := files[0]
	conflict.Filename = zest.PtrString("conflicting.whl")
	filenameConflict := files[0]
	filenameConflict.PulpHref = zest.PtrString(pythonTestContent + "conflicting/")
	for _, tc := range []struct {
		name  string
		pages []any
	}{
		{name: "upstream error", pages: []any{http.StatusServiceUnavailable}},
		{name: "later upstream error", pages: []any{pythonInventoryPage(2, files[0]), http.StatusServiceUnavailable}},
		{name: "malformed JSON", pages: []any{"{"}},
		{name: "missing results", pages: []any{map[string]any{"count": 0}}},
		{name: "negative count", pages: []any{pythonInventoryPage(-1)}},
		{name: "too many records", pages: []any{pythonInventoryPage(1, files...)}},
		{name: "early empty page", pages: []any{pythonInventoryPage(2, files[0]), pythonInventoryPage(2)}},
		{name: "changing count", pages: []any{pythonInventoryPage(2, files[0]), pythonInventoryPage(3, files[1])}},
		{name: "repeated page", pages: []any{pythonInventoryPage(2, files[0]), pythonInventoryPage(2, files[0])}},
		{name: "conflicting content identity", pages: []any{pythonInventoryPage(2, files[0]), pythonInventoryPage(2, conflict)}},
		{name: "conflicting filename", pages: []any{pythonInventoryPage(2, files[0], filenameConflict)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := 0
			client := pythonInventoryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if servePythonInventoryMapping(t, w, r) {
					return
				}
				if page >= len(tc.pages) {
					t.Error("unexpected extra inventory request")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				response := tc.pages[page]
				page++
				switch response := response.(type) {
				case int:
					w.WriteHeader(response)
				case string:
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(response))
				default:
					writePythonInventoryJSON(t, w, response)
				}
			})
			got, err := client.ListPythonPackageFiles(context.Background(), pythonTestBasePath, pythonTestName, "3.0.1+rhlw.2")
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Equal(t, len(tc.pages), page)
		})
	}
}

func TestListPythonPackageFilesUpstreamErrorDetails(t *testing.T) {
	const upstreamBody = `{"detail":"Pulp failure at https://pulp.internal/storage/private; credential=test-only-secret"}`
	for _, tc := range []struct {
		name   string
		status int
		later  bool
	}{
		{name: "authentication failure", status: http.StatusUnauthorized},
		{name: "server failure", status: http.StatusServiceUnavailable},
		{name: "later page failure", status: http.StatusServiceUnavailable, later: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := pythonInventoryFixture(t, "rhlw-2")[0]
			client := pythonInventoryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if servePythonInventoryMapping(t, w, r) {
					return
				}
				if tc.later && r.URL.Query().Get("offset") == "0" {
					writePythonInventoryJSON(t, w, pythonInventoryPage(2, file))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(upstreamBody))
			})

			files, err := client.ListPythonPackageFiles(context.Background(), pythonTestBasePath, pythonTestName, "3.0.1+rhlw.2")
			require.ErrorContains(t, err, "error listing Python package files")
			assert.ErrorContains(t, err, upstreamBody, "retain Pulp diagnostics internally")
			assert.Nil(t, files, "discard any files fetched before the failure")
			var upstreamErr *zest.GenericOpenAPIError
			require.ErrorAs(t, err, &upstreamErr, "preserve the wrapped client error")
			assert.Equal(t, upstreamBody, string(upstreamErr.Body()))

			// The adapter returns an internal error, not a public ErrorResponse.
			// Keep diagnostics out of the existing HTTP error handler's response.
			recorder := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", http.NoBody), recorder)
			config.CustomHTTPErrorHandler(err, c)
			assert.Equal(t, http.StatusInternalServerError, recorder.Code)
			assert.JSONEq(t, `{"errors":[{"status":500,"detail":"Internal Server Error"}]}`, recorder.Body.String())
		})
	}
}

func TestListPythonPackageFilesCancellation(t *testing.T) {
	files := pythonInventoryFixture(t, "rhlw-2")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := pythonInventoryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !servePythonInventoryMapping(t, w, r) {
			writePythonInventoryJSON(t, w, pythonInventoryPage(2, files[0]))
			cancel()
		}
	})
	got, err := client.ListPythonPackageFiles(ctx, pythonTestBasePath, pythonTestName, "3.0.1+rhlw.2")
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, got)
}

func TestListPythonPackageFilesRequiresCoordinates(t *testing.T) {
	client := pythonInventoryTestClient(t, func(_ http.ResponseWriter, _ *http.Request) { t.Error("unexpected Pulp request") })
	for _, coordinates := range [][3]string{
		{"", pythonTestName, "3.0.1+rhlw.2"},
		{pythonTestBasePath, "", "3.0.1+rhlw.2"},
		{pythonTestBasePath, pythonTestName, ""},
	} {
		got, err := client.ListPythonPackageFiles(context.Background(), coordinates[0], coordinates[1], coordinates[2])
		require.Error(t, err)
		assert.Nil(t, got)
	}
}

func pythonInventoryFixture(t *testing.T, build string) []zest.PythonPythonPackageContentResponse {
	t.Helper()
	data, err := os.ReadFile("testdata/python/" + build + ".json")
	require.NoError(t, err)
	var files []zest.PythonPythonPackageContentResponse
	require.NoError(t, json.Unmarshal(data, &files))
	return files
}

func pythonInventoryTestClient(t *testing.T, handler http.HandlerFunc) PulpClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	original := config.Get().Clients.Pulp
	t.Cleanup(func() { config.Get().Clients.Pulp = original })
	pulp := &config.Get().Clients.Pulp
	pulp.Server = server.URL + "/"
	pulp.Username = "test-user"
	pulp.Password = "test-password"
	pulp.CACert = ""
	pulp.CACertPath = ""
	pulp.Proxy = ""
	return GetPulpClientWithDomain("lightwell")
}

func servePythonInventoryMapping(t *testing.T, w http.ResponseWriter, r *http.Request) bool {
	t.Helper()
	switch r.URL.Path {
	case "/api/pulp/lightwell/api/v3/distributions/":
		assert.Equal(t, pythonTestBasePath, r.URL.Query().Get("base_path"))
		writePythonInventoryJSON(t, w, map[string]any{"count": 1, "results": []map[string]any{{
			"name": pythonTestBasePath, "base_path": pythonTestBasePath, "repository": pythonTestRepo,
		}}})
	case "/api/pulp/lightwell/api/v3/repositories/":
		assert.Equal(t, pythonTestRepo, r.URL.Query().Get("pulp_href__in"))
		writePythonInventoryJSON(t, w, map[string]any{"count": 1, "results": []map[string]any{{
			"name": pythonTestBasePath, "latest_version_href": pythonTestVersion,
		}}})
	default:
		return false
	}
	return true
}

func pythonInventoryPage(count int, files ...zest.PythonPythonPackageContentResponse) map[string]any {
	if files == nil {
		files = []zest.PythonPythonPackageContentResponse{}
	}
	return map[string]any{"count": count, "next": nil, "previous": nil, "results": files}
}

func writePythonInventoryJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	assert.NoError(t, json.NewEncoder(w).Encode(value))
}
