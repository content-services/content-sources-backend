// Some tests require local testdata fixtures (POM, OSV JSONs) under
// testdata/ that are not committed to git. They skip automatically
// when the files are absent.

package sonatype_bridge

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/external_repos/vulnerability_parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testdataPath(parts ...string) string {
	return filepath.Join(append([]string{"testdata"}, parts...)...)
}

func skipIfFixturesAbsent(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Skipf("fixture not found: %s", p)
		}
	}
}

var (
	fixturePOM    = testdataPath("registry", "org", "springframework", "spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.pom")
	fixtureOSVDir = testdataPath("osv", "java", "remediated", "PULP_MANIFEST")
)

func TestRegistryClient_FetchPOM(t *testing.T) {
	skipIfFixturesAbsent(t, fixturePOM)
	pomData, err := os.ReadFile(testdataPath("registry", "org", "springframework",
		"spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.pom"))
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(pomData)
	}))
	defer server.Close()

	client := &httpRegistryClient{
		httpClient:  server.Client(),
		registryURL: server.URL,
		maxRetries:  0,
	}

	data, err := client.FetchPOM(context.Background(),
		"org.springframework", "spring-core", "5.3.18.rhlw-00003")
	require.NoError(t, err)
	assert.Contains(t, string(data), "<artifactId>spring-core</artifactId>")
}

func TestRegistryClient_FetchJAR(t *testing.T) {
	jarPath := testdataPath("registry", "org", "springframework",
		"spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.jar")
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		t.Skip("JAR fixture not found")
	}

	jarData, err := os.ReadFile(jarPath)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jarData)
	}))
	defer server.Close()

	client := &httpRegistryClient{
		httpClient:  server.Client(),
		registryURL: server.URL,
		maxRetries:  0,
	}

	data, sha, err := client.FetchJAR(context.Background(),
		"org.springframework", "spring-core", "5.3.18.rhlw-00003")
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.Len(t, sha, 64)
}

func TestRegistryClient_FetchOSVRecords(t *testing.T) {
	skipIfFixturesAbsent(t, fixtureOSVDir)
	server := httptest.NewServer(http.FileServer(http.Dir(testdataPath("osv", "java", "remediated"))))
	defer server.Close()

	client := &httpRegistryClient{
		httpClient: server.Client(),
		osvURL:     server.URL,
		maxRetries: 0,
	}

	records, err := client.FetchOSVRecords(context.Background(), "org.springframework", "spring-core", "5.3.18")
	require.NoError(t, err)
	assert.Len(t, records, 6)

	cveIDs := make([]string, len(records))
	for i, r := range records {
		cveIDs[i] = r.CVEID
	}
	assert.Contains(t, cveIDs, "CVE-2023-20860")
	assert.Contains(t, cveIDs, "CVE-2025-41249")
}

func TestParseOSVFile(t *testing.T) {
	skipIfFixturesAbsent(t, fixtureOSVDir)
	data, err := os.ReadFile(testdataPath("osv", "java", "remediated", "x_RHLW-CVE-2023-20860-5.3.18.json"))
	require.NoError(t, err)

	recs, err := parseOSVRecords(data)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	rec := recs[0]

	assert.Equal(t, "CVE-2023-20860", rec.CVEID)
	assert.Contains(t, rec.Description, "mvcRequestMatcher")
	assert.Equal(t, "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N", rec.CVSSVector)
	assert.InDelta(t, 7.5, rec.CVSSScore, 0.1)
	assert.Equal(t, "high", rec.Severity)
	assert.Contains(t, rec.Aliases, "GHSA-7phw-cxx7-q9vq")
}

func TestComputeCVSS31BaseScore(t *testing.T) {
	tests := []struct {
		vector   string
		expected float64
	}{
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N", 7.5},
		{"CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:N/A:H", 6.5},
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:N/I:N/A:L", 4.3},
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N", 7.5},
	}
	for _, tt := range tests {
		t.Run(tt.vector, func(t *testing.T) {
			score := computeCVSS31BaseScore(tt.vector)
			assert.InDelta(t, tt.expected, score, 0.1)
		})
	}
}

func TestRegistryClient_BasicAuthHeader(t *testing.T) {
	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("<project><artifactId>test</artifactId></project>"))
	}))
	defer server.Close()

	client := &httpRegistryClient{
		httpClient:  server.Client(),
		registryURL: server.URL,
		username:    "testuser",
		password:    "testpass",
		maxRetries:  0,
	}

	_, err := client.FetchPOM(context.Background(), "org.test", "test", "1.0.rhlw-00001")
	require.NoError(t, err)

	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass"))
	assert.Equal(t, expected, capturedAuth)
}

func TestFetchOSVRecords_MatchesPackageAndBaseVersion(t *testing.T) {
	tmpDir := t.TempDir()

	manifest := "RHLW-2026-0000000000000018.json,abc123,1234\n" +
		"RHLW-2026-0000000000000001.json,def456,1234\n" +
		"x_RHLW-CVE-2099-99999-5.3.1.json,ghi789,1234\n" +
		"legacy/RHLW-2026-0000000000000099.json,jkl012,1234\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "PULP_MANIFEST"), []byte(manifest), 0600))

	matching := `{
		"id": "RHLW-2026-0000000000000001",
		"details": "test",
		"upstream": ["CVE-2099-99999", "GHSA-test"],
		"severity": [{"type": "CVSS_V3", "score": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N"}],
		"affected": [{
			"package": {"name": "org.springframework:spring-core", "purl": "pkg:maven/org.springframework/spring-core"},
			"versions": ["5.3.1"],
			"ranges": [{"type": "ECOSYSTEM", "events": [{"introduced": "0"}, {"fixed": "5.3.1.rhlw-00001"}]}]
		}]
	}`
	otherVersion := `{
		"id": "RHLW-2026-0000000000000018",
		"upstream": ["CVE-2023-20860"],
		"affected": [{
			"package": {"name": "org.springframework:spring-core", "purl": "pkg:maven/org.springframework/spring-core"},
			"versions": ["5.3.18"]
		}]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "RHLW-2026-0000000000000001.json"), []byte(matching), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "RHLW-2026-0000000000000018.json"), []byte(otherVersion), 0600))

	server := httptest.NewServer(http.FileServer(http.Dir(tmpDir)))
	defer server.Close()

	client := &httpRegistryClient{
		httpClient: server.Client(),
		osvURL:     server.URL,
		maxRetries: 0,
	}

	records, err := client.FetchOSVRecords(context.Background(), "org.springframework", "spring-core", "5.3.1")
	require.NoError(t, err)
	require.Len(t, records, 1, "only the root RHLW record for this package and base version should match")
	assert.Equal(t, "CVE-2099-99999", records[0].CVEID)
	assert.Contains(t, records[0].Aliases, "GHSA-test")
}

func TestParseOSVRecords_ExpandsUpstreamCVEs(t *testing.T) {
	data := []byte(`{
		"id": "RHLW-2026-055d7bde52d9946f",
		"details": "two cves",
		"upstream": ["CVE-2023-6378", "GHSA-vmq6-5m68-f53m", "CVE-2024-12801"],
		"severity": [{"type": "CVSS_V3", "score": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N"}]
	}`)
	records, err := parseOSVRecords(data)
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "CVE-2023-6378", records[0].CVEID)
	assert.Equal(t, "CVE-2024-12801", records[1].CVEID)
	assert.Equal(t, []string{"GHSA-vmq6-5m68-f53m"}, records[0].Aliases)
	assert.Equal(t, []string{"GHSA-vmq6-5m68-f53m"}, records[1].Aliases)
}

func TestFetchOSVRecords_AllFilesFail(t *testing.T) {
	manifest := "RHLW-2026-0000000000000018.json,abc123,1234\nRHLW-2026-0000000000000049.json,def456,1234\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/PULP_MANIFEST" {
			_, _ = w.Write([]byte(manifest))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &httpRegistryClient{
		httpClient: server.Client(),
		osvURL:     server.URL,
		maxRetries: 0,
	}

	_, err := client.FetchOSVRecords(context.Background(), "org.springframework", "spring-core", "5.3.18")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch")
	assert.Contains(t, err.Error(), "5.3.18")
}

func TestFetchSBOM_Success(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, ".bom.json.asc"):
			_, _ = w.Write([]byte("sig"))
		case strings.HasSuffix(r.URL.Path, ".bom.json"):
			_, _ = w.Write([]byte("bom"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &httpRegistryClient{httpClient: server.Client(), registryURL: server.URL, maxRetries: 0}
	bom, asc, err := client.FetchSBOM(context.Background(), "org.example", "lib", "1.0.rhlw-00001")
	require.NoError(t, err)
	assert.Equal(t, []byte("bom"), bom)
	assert.Equal(t, []byte("sig"), asc)
	require.Len(t, paths, 2)
	assert.Equal(t, "/org/example/lib/1.0.rhlw-00001/lib-1.0.rhlw-00001.bom.json", paths[0])
	assert.Equal(t, "/org/example/lib/1.0.rhlw-00001/lib-1.0.rhlw-00001.bom.json.asc", paths[1])
}

func TestFetchSBOM_NotFound(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &httpRegistryClient{httpClient: server.Client(), registryURL: server.URL, maxRetries: 0}
	_, _, err := client.FetchSBOM(context.Background(), "org.example", "lib", "1.0.rhlw-00001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch SBOM")
	assert.Equal(t, 1, hits)
}

func TestFetchSBOM_SignatureNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".bom.json") && !strings.HasSuffix(r.URL.Path, ".asc") {
			_, _ = w.Write([]byte("bom"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &httpRegistryClient{httpClient: server.Client(), registryURL: server.URL, maxRetries: 0}
	_, _, err := client.FetchSBOM(context.Background(), "org.example", "lib", "1.0.rhlw-00001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature")
}

func TestMultiCVEVEXFilename(t *testing.T) {
	entries := []vulnerability_parser.ManifestEntry{
		{Filename: "CVE-2025-41249-2026-07-15T14:02:27+00:00.bom.json"},
		{Filename: "CVE-2023-20860-2026-06-01T00:00:00+00:00.bom.json"},
		{Filename: "CVE-2023-20860-2026-07-15T14:02:27+00:00.bom.json"},
		{Filename: "2026-07-15T14:02:27+00:00.bom.json"},
		{Filename: "x_RHLW-CVE-2025-41249-5.3.18.json"},
		{Filename: "CVE-2025-1-2026-07-15T14:02:27+00:00.bom.json"},
		{Filename: "CVE-2025-12-2026-01-01T00:00:00+00:00.bom.json"},
	}

	name, err := multiCVEVEXFilename(entries, []string{"CVE-2025-41249", "CVE-2023-20860"})
	require.NoError(t, err)
	assert.Equal(t, "2026-07-15T14:02:27+00:00.bom.json", name)

	short, err := multiCVEVEXFilename(entries, []string{"CVE-2025-1"})
	require.NoError(t, err)
	assert.Equal(t, "2026-07-15T14:02:27+00:00.bom.json", short)

	_, err = multiCVEVEXFilename(entries, nil)
	require.Error(t, err)

	_, err = multiCVEVEXFilename(entries, []string{"CVE-2025-41249", "CVE-2099-0001"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CVE-2025-41249")
	assert.Contains(t, err.Error(), "CVE-2099-0001")

	multi := []vulnerability_parser.ManifestEntry{
		{Filename: "CVE-2024-1-2026-01-01T00:00:00+00:00.bom.json"},
		{Filename: "CVE-2024-1-2026-02-01T00:00:00+00:00.bom.json"},
	}
	chosen, err := multiCVEVEXFilename(multi, []string{"CVE-2024-1"})
	require.NoError(t, err)
	assert.Equal(t, "2026-02-01T00:00:00+00:00.bom.json", chosen)
}

func TestFetchVEX_Success(t *testing.T) {
	const (
		ts   = "2026-07-15T14:02:27+00:00"
		body = `{"bomFormat":"CycloneDX"}`
	)
	var requestURIs []string
	vexServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURIs = append(requestURIs, r.RequestURI)
		if strings.HasSuffix(r.URL.Path, "PULP_MANIFEST") {
			_, _ = w.Write([]byte("CVE-2025-41249-" + ts + ".bom.json,abc,10\n"))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer vexServer.Close()

	osvServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("OSV server must not be contacted, got %s", r.URL.Path)
	}))
	defer osvServer.Close()

	client := &httpRegistryClient{
		httpClient: vexServer.Client(),
		vexURL:     vexServer.URL,
		osvURL:     osvServer.URL,
		maxRetries: 0,
	}
	got, err := client.FetchVEX(context.Background(), []string{"CVE-2025-41249"})
	require.NoError(t, err)
	assert.Equal(t, []byte(body), got)
	require.Len(t, requestURIs, 2)
	assert.Contains(t, requestURIs[0], "PULP_MANIFEST")
	assert.Contains(t, requestURIs[1], "2026-07-15T14:02:27%2B00:00.bom.json")
}

func TestFetchVEX_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "PULP_MANIFEST") {
			_, _ = w.Write([]byte("CVE-2025-41249-2026-07-15T14:02:27+00:00.bom.json,abc,10\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &httpRegistryClient{httpClient: server.Client(), vexURL: server.URL, maxRetries: 0}
	_, err := client.FetchVEX(context.Background(), []string{"CVE-2025-41249"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2026-07-15T14:02:27+00:00.bom.json")
}

func TestFetchVEX_NoIntersection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("CVE-2025-1-2026-01-01T00:00:00+00:00.bom.json,abc,10\nCVE-2025-2-2026-02-01T00:00:00+00:00.bom.json,def,10\n"))
	}))
	defer server.Close()

	client := &httpRegistryClient{httpClient: server.Client(), vexURL: server.URL, maxRetries: 0}
	_, err := client.FetchVEX(context.Background(), []string{"CVE-2025-1", "CVE-2025-2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no shared VEX timestamp")
}

func TestFetchClassifier_Success(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte("src"))
	}))
	defer server.Close()

	client := &httpRegistryClient{httpClient: server.Client(), registryURL: server.URL, maxRetries: 0}
	data, err := client.FetchClassifier(context.Background(), "org.example", "lib", "1.0.rhlw-00001", "sources")
	require.NoError(t, err)
	assert.Equal(t, []byte("src"), data)
	assert.Equal(t, "/org/example/lib/1.0.rhlw-00001/lib-1.0.rhlw-00001-sources.jar", path)
}

func TestFetchClassifier_NotFoundIsAbsentWithoutRetry(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &httpRegistryClient{httpClient: server.Client(), registryURL: server.URL, maxRetries: 3}
	data, err := client.FetchClassifier(context.Background(), "org.example", "lib", "1.0", "javadoc")
	require.NoError(t, err)
	assert.Nil(t, data)
	assert.Equal(t, 1, hits, "404 must not be retried")
}

func TestFetchClassifier_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &httpRegistryClient{httpClient: server.Client(), registryURL: server.URL, maxRetries: 0}
	_, err := client.FetchClassifier(context.Background(), "org.example", "lib", "1.0", "sources")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
}
