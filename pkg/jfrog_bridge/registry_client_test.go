// Some tests require local testdata fixtures (POM, OSV JSONs) under
// testdata/ that are not committed to git. They skip automatically
// when the files are absent.

package jfrog_bridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

const (
	vexTestUUID    = "11111111-2222-3333-4444-555555555555"
	vexTestSerial  = "urn:uuid:" + vexTestUUID
	vexTestGroup   = "org.example"
	vexTestName    = "demo"
	vexTestVersion = "1.2.3"
	vexOtherUUID   = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
)

func vexRef(uuid string) string {
	return "urn:cdx:" + uuid + "/1#pkg:maven/org.example/demo@1.2.3"
}

func vexDocument(refs []string, purl string, withComponent bool) string {
	doc := map[string]any{
		"bomFormat": "CycloneDX",
		"vulnerabilities": []map[string]any{
			{"affects": affectsRefs(refs)},
		},
	}
	if withComponent {
		doc["metadata"] = map[string]any{
			"component": map[string]any{"purl": purl},
		}
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func affectsRefs(refs []string) []map[string]string {
	out := make([]map[string]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, map[string]string{"ref": ref})
	}
	return out
}

func manifestLines(names ...string) string {
	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteString(",abc,10\n")
	}
	return b.String()
}

func fetchVEXForTest(t *testing.T, manifest string, files map[string]string, cveIDs ...string) ([]byte, []string, error) {
	t.Helper()
	var requestURIs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURIs = append(requestURIs, r.RequestURI)
		if strings.HasSuffix(r.URL.Path, "PULP_MANIFEST") {
			_, _ = w.Write([]byte(manifest))
			return
		}
		for name, body := range files {
			if r.URL.Path == "/"+name {
				if body == "" {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write([]byte(body))
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	client := &httpRegistryClient{httpClient: server.Client(), vexURL: server.URL, maxRetries: 0}
	got, err := client.FetchVEX(context.Background(), cveIDs, vexTestGroup, vexTestName, vexTestVersion, vexTestSerial)
	return got, requestURIs, err
}

func fileRequests(uris []string) []string {
	var out []string
	for _, uri := range uris {
		if strings.Contains(uri, "PULP_MANIFEST") {
			continue
		}
		out = append(out, uri)
	}
	return out
}

func TestFetchVEX_OneCVERequestsPerCVEName(t *testing.T) {
	const ts = "2026-07-15T14:02:27+00:00"
	perCVE := "CVE-2025-41249-" + ts + ".bom.json"
	bundled := ts + ".bom.json"
	body := vexDocument([]string{vexRef(vexTestUUID)}, "", false)
	got, uris, err := fetchVEXForTest(t, manifestLines(perCVE, bundled), map[string]string{
		perCVE:  body,
		bundled: vexDocument([]string{vexRef(vexOtherUUID)}, "", false),
	}, "CVE-2025-41249")
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
	require.Len(t, uris, 2)
	assert.Contains(t, uris[0], "PULP_MANIFEST")
	assert.Equal(t, []string{"/" + strings.ReplaceAll(perCVE, "+", "%2B")}, fileRequests(uris))
}

func TestFetchVEX_TwoCVEsRequestBundledFile(t *testing.T) {
	const ts = "2026-07-15T14:02:27Z"
	bundled := ts + ".bom.json"
	perA := "CVE-2025-1-" + ts + ".bom.json"
	perB := "CVE-2025-2-" + ts + ".bom.json"
	body := vexDocument([]string{vexRef(vexTestUUID)}, "", false)
	got, uris, err := fetchVEXForTest(t, manifestLines(perA, perB, bundled), map[string]string{
		bundled: body,
		perA:    "unused",
		perB:    "unused",
	}, "CVE-2025-1", "CVE-2025-2")
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
	assert.Equal(t, []string{"/" + bundled}, fileRequests(uris))
}

func TestFetchVEX_TwoCVEsBundledAbsentUsesMatchingPerCVE(t *testing.T) {
	const ts = "2026-07-15T14:02:27Z"
	perA := "CVE-2025-1-" + ts + ".bom.json"
	perB := "CVE-2025-2-" + ts + ".bom.json"
	match := vexDocument([]string{vexRef(vexTestUUID)}, "", false)
	got, uris, err := fetchVEXForTest(t, manifestLines(perA, perB), map[string]string{
		perA: vexDocument([]string{vexRef(vexOtherUUID)}, "", false),
		perB: match,
	}, "CVE-2025-1", "CVE-2025-2")
	require.NoError(t, err)
	assert.Equal(t, match, string(got))
	assert.Equal(t, []string{"/" + perA, "/" + perB}, fileRequests(uris))
}

func TestFetchVEX_URNSerialAndCDXRef(t *testing.T) {
	const ts = "2026-07-15T14:02:27Z"
	name := "CVE-2025-1-" + ts + ".bom.json"
	body := vexDocument([]string{"urn:cdx:" + vexTestUUID + "/1#pkg:maven/org.example/demo@1.2.3?type=jar"}, "", false)
	got, _, err := fetchVEXForTest(t, manifestLines(name), map[string]string{name: body}, "CVE-2025-1")
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}

func TestFetchVEX_OtherUUIDTriesNextTimestamp(t *testing.T) {
	const (
		newer = "2026-02-01T00:00:00Z"
		older = "2026-01-01T00:00:00Z"
	)
	newName := "CVE-2025-1-" + newer + ".bom.json"
	oldName := "CVE-2025-1-" + older + ".bom.json"
	match := vexDocument([]string{vexRef(vexTestUUID)}, "", false)
	got, uris, err := fetchVEXForTest(t, manifestLines(newName, older+".bom.json", oldName), map[string]string{
		newName:             vexDocument([]string{vexRef(vexOtherUUID)}, "", false),
		older + ".bom.json": match,
		oldName:             match,
	}, "CVE-2025-1")
	require.NoError(t, err)
	assert.Equal(t, match, string(got))
	assert.Equal(t, []string{"/" + newName, "/" + oldName}, fileRequests(uris))
}

func TestFetchVEX_BundledMismatchSkipsPerCVEAtSameTimestamp(t *testing.T) {
	const (
		newer = "2026-02-01T00:00:00Z"
		older = "2026-01-01T00:00:00Z"
	)
	newBundle := newer + ".bom.json"
	oldBundle := older + ".bom.json"
	perA := "CVE-2025-1-" + newer + ".bom.json"
	perB := "CVE-2025-2-" + newer + ".bom.json"
	match := vexDocument([]string{vexRef(vexTestUUID)}, "", false)
	got, uris, err := fetchVEXForTest(t,
		manifestLines(perA, perB, newBundle, "CVE-2025-1-"+older+".bom.json", "CVE-2025-2-"+older+".bom.json", oldBundle),
		map[string]string{
			newBundle: vexDocument([]string{vexRef(vexOtherUUID)}, "", false),
			perA:      match,
			perB:      match,
			oldBundle: match,
		},
		"CVE-2025-1", "CVE-2025-2")
	require.NoError(t, err)
	assert.Equal(t, match, string(got))
	assert.Equal(t, []string{"/" + newBundle, "/" + oldBundle}, fileRequests(uris))
}

func TestFetchVEX_CVEIDDoesNotMatchLongerID(t *testing.T) {
	const ts = "2026-07-15T14:02:27Z"
	short := "CVE-2025-1-" + ts + ".bom.json"
	long := "CVE-2025-12-" + ts + ".bom.json"
	body := vexDocument([]string{vexRef(vexTestUUID)}, "", false)
	_, uris, err := fetchVEXForTest(t, manifestLines(long, short), map[string]string{
		short: body,
		long:  body,
	}, "CVE-2025-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"/" + short}, fileRequests(uris))
}

func TestFetchVEX_PurlQualifiersMatchThisGAV(t *testing.T) {
	const ts = "2026-07-15T14:02:27Z"
	name := "CVE-2025-1-" + ts + ".bom.json"
	body := vexDocument(
		[]string{vexRef(vexTestUUID)},
		"pkg:maven/org.example/demo@1.2.3?repository_url=https://repo.test/maven&type=jar",
		true,
	)
	got, _, err := fetchVEXForTest(t, manifestLines(name), map[string]string{name: body}, "CVE-2025-1")
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}

func TestFetchVEX_PurlOtherArtifactRejected(t *testing.T) {
	const ts = "2026-07-15T14:02:27Z"
	name := "CVE-2025-1-" + ts + ".bom.json"
	body := vexDocument(
		[]string{vexRef(vexTestUUID)},
		"pkg:maven/org.example/other@1.2.3?type=jar",
		true,
	)
	_, _, err := fetchVEXForTest(t, manifestLines(name), map[string]string{name: body}, "CVE-2025-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no document matches")
}

func TestFetchVEX_NoComponentAccepted(t *testing.T) {
	const ts = "2026-07-15T14:02:27Z"
	name := "CVE-2025-1-" + ts + ".bom.json"
	body := vexDocument([]string{vexRef(vexTestUUID)}, "", false)
	got, _, err := fetchVEXForTest(t, manifestLines(name), map[string]string{name: body}, "CVE-2025-1")
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}

func TestFetchVEX_TwoRefsOneUUIDAccepted(t *testing.T) {
	const ts = "2026-07-15T14:02:27Z"
	name := "CVE-2025-1-" + ts + ".bom.json"
	body := vexDocument([]string{vexRef(vexOtherUUID), vexRef(vexTestUUID)}, "", false)
	got, _, err := fetchVEXForTest(t, manifestLines(name), map[string]string{name: body}, "CVE-2025-1")
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}

func TestFetchVEX_NoMatchingBody(t *testing.T) {
	const ts = "2026-07-15T14:02:27Z"
	name := "CVE-2025-1-" + ts + ".bom.json"
	_, _, err := fetchVEXForTest(t, manifestLines(name), map[string]string{
		name: vexDocument([]string{vexRef(vexOtherUUID)}, "", false),
	}, "CVE-2025-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no document matches")
}

func TestFetchVEX_DownloadErrorStopsWalk(t *testing.T) {
	const (
		newer = "2026-02-01T00:00:00Z"
		older = "2026-01-01T00:00:00Z"
	)
	newName := "CVE-2025-1-" + newer + ".bom.json"
	oldName := "CVE-2025-1-" + older + ".bom.json"
	_, uris, err := fetchVEXForTest(t, manifestLines(newName, oldName), map[string]string{
		newName: "",
		oldName: vexDocument([]string{vexRef(vexTestUUID)}, "", false),
	}, "CVE-2025-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), newName)
	assert.Equal(t, []string{"/" + newName}, fileRequests(uris))
}

func TestFetchVEX_InvalidJSONStopsWalk(t *testing.T) {
	const (
		newer = "2026-02-01T00:00:00Z"
		older = "2026-01-01T00:00:00Z"
	)
	newName := "CVE-2025-1-" + newer + ".bom.json"
	oldName := "CVE-2025-1-" + older + ".bom.json"
	_, uris, err := fetchVEXForTest(t, manifestLines(newName, oldName), map[string]string{
		newName: "not-json",
		oldName: vexDocument([]string{vexRef(vexTestUUID)}, "", false),
	}, "CVE-2025-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), newName)
	assert.Equal(t, []string{"/" + newName}, fileRequests(uris))
}

func TestFetchVEX_EmptySerialDoesNotReadManifest(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte("CVE-2025-1-2026-01-01T00:00:00Z.bom.json,abc,10\n"))
	}))
	defer server.Close()
	client := &httpRegistryClient{httpClient: server.Client(), vexURL: server.URL, maxRetries: 0}
	_, err := client.FetchVEX(context.Background(), []string{"CVE-2025-1"}, vexTestGroup, vexTestName, vexTestVersion, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "serialNumber")
	assert.Equal(t, 0, hits)
}

func TestFetchVEX_BadPurlTriesNextFile(t *testing.T) {
	const ts = "2026-07-15T14:02:27Z"
	perA := "CVE-2025-1-" + ts + ".bom.json"
	perB := "CVE-2025-2-" + ts + ".bom.json"
	match := vexDocument([]string{vexRef(vexTestUUID)}, "pkg:maven/org.example/demo@1.2.3", true)
	got, uris, err := fetchVEXForTest(t, manifestLines(perA, perB), map[string]string{
		perA: vexDocument([]string{vexRef(vexTestUUID)}, "not a purl", true),
		perB: match,
	}, "CVE-2025-1", "CVE-2025-2")
	require.NoError(t, err)
	assert.Equal(t, match, string(got))
	assert.Equal(t, []string{"/" + perA, "/" + perB}, fileRequests(uris))
}
