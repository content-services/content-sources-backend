package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the public route and the real adapter together: public pagination
// must use the complete upstream inventory and must never expose partial data.
func TestPythonPackageFilesRealAdapter(t *testing.T) {
	fixture, err := os.ReadFile("../clients/pulp_client/testdata/python/rhlw-2.json")
	require.NoError(t, err)
	var inventory []zest.PythonPythonPackageContentResponse
	require.NoError(t, json.Unmarshal(fixture, &inventory))
	require.Len(t, inventory, 2)

	for _, scenario := range []string{"complete inventory", "late upstream failure", "malformed late record"} {
		t.Run(scenario, func(t *testing.T) {
			f := newPythonFilesHandlerTest(t, "limit=1&offset=1&sort_by=filename:asc")
			f.expectRepository()
			f.reg.Domain.On("Fetch", f.c.Request().Context(), config.LightwellOrg).Return(filesTestDomain, nil).Once()
			root := "/api/pulp/" + filesTestDomain + "/api/v3/"
			repoVersion := root + "repositories/python/python/fixture/versions/4/"
			var offsets []int
			mappingReads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, password, ok := r.BasicAuth()
				assert.True(t, ok)
				assert.Equal(t, "test-user", user)
				assert.Equal(t, "test-password", password)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case root + "distributions/":
					mappingReads++
					assert.Equal(t, filesTestBasePath, r.URL.Query().Get("base_path"))
					assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"count": 1, "results": []map[string]any{{
						"name": filesTestBasePath, "base_path": filesTestBasePath, "repository_version": repoVersion,
					}}}))
				case root + "content/python/packages/":
					query := r.URL.Query()
					assert.Equal(t, repoVersion, query.Get("repository_version"))
					assert.Equal(t, "lightwell-fixture", query.Get("name"))
					assert.Equal(t, filesTestVersion, query.Get("version"))
					assert.Contains(t, r.URL.RawQuery, "%2B")
					assert.Equal(t, "filename", query.Get("ordering"))
					assert.Equal(t, "100", query.Get("limit"), "upstream and public page sizes are independent")
					offset, parseErr := strconv.Atoi(query.Get("offset"))
					if !assert.NoError(t, parseErr) || !assert.True(t, offset >= 0 && offset < len(inventory)) {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					offsets = append(offsets, offset)
					if scenario == "late upstream failure" && offset == 1 {
						w.WriteHeader(http.StatusInternalServerError)
						_, writeErr := w.Write([]byte(`{"detail":"secret https://admin:password@internal.invalid/raw-body"}`))
						assert.NoError(t, writeErr)
						return
					}
					file := inventory[offset]
					if scenario == "malformed late record" && offset == 1 {
						file.Sha256 = zest.PtrString("secret malformed checksum")
					}
					assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"count": len(inventory), "next": "https://untrusted.invalid/never-follow/", "previous": nil,
						"results": []zest.PythonPythonPackageContentResponse{file},
					}))
				default:
					t.Errorf("unexpected Pulp request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			originalPulp := config.Get().Clients.Pulp
			t.Cleanup(func() { config.Get().Clients.Pulp = originalPulp })
			pulp := &config.Get().Clients.Pulp
			pulp.Server, pulp.Username, pulp.Password = server.URL, "test-user", "test-password"
			pulp.CACert, pulp.CACertPath, pulp.Proxy = "", "", ""
			f.handler.PulpClient = pulp_client.GetPulpClientWithDomain("")
			router := echo.New()
			router.HTTPErrorHandler = config.CustomHTTPErrorHandler
			router.GET("/api/lightwell/v0.1/repositories/:uuid/python_packages/:name/:version/files", f.handler.ListPythonPackageFiles)
			router.ServeHTTP(f.rr, f.c.Request())
			assert.Equal(t, 1, mappingReads, "pin the scoped repository snapshot once")
			assert.Equal(t, []int{0, 1}, offsets, "fetch every upstream page before public pagination")
			if scenario != "complete inventory" {
				require.Equal(t, http.StatusInternalServerError, f.rr.Code, f.rr.Body.String())
				for _, private := range []string{"secret", "password", "internal.invalid", "raw-body", "pulp_href", "artifact", "lightwell_fixture"} {
					assert.NotContains(t, f.rr.Body.String(), private)
				}
				var body map[string]any
				require.NoError(t, json.Unmarshal(f.rr.Body.Bytes(), &body))
				assert.Contains(t, body, "errors")
				assert.NotContains(t, body, "data")
				return
			}
			require.Equal(t, http.StatusOK, f.rr.Code, f.rr.Body.String())
			var response api.PythonPackageFileCollectionResponse
			require.NoError(t, json.Unmarshal(f.rr.Body.Bytes(), &response))
			require.Len(t, response.Data, 1)
			assert.Equal(t, inventory[1].GetFilename(), response.Data[0].Filename)
			assert.Equal(t, inventory[1].Size, response.Data[0].Size)
			assert.Equal(t, inventory[1].Sha256, response.Data[0].Sha256)
			assert.Equal(t, api.ResponseMetadata{Count: 2, Limit: 1, Offset: 1}, response.Meta)
			for _, raw := range []string{response.Links.First, response.Links.Last, response.Links.Prev} {
				link, parseErr := url.Parse(raw)
				require.NoError(t, parseErr)
				assert.Contains(t, link.EscapedPath(), "3.0.1%2Brhlw.2")
				assert.Equal(t, "filename:asc", link.Query().Get("sort_by"))
				assert.Empty(t, link.Host)
			}
			assert.Empty(t, response.Links.Next)
			assert.NotContains(t, f.rr.Body.String(), "pulp_href")
			assert.NotContains(t, f.rr.Body.String(), "artifact")
		})
	}
}
