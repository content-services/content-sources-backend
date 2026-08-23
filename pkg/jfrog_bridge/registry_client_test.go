package jfrog_bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testdataPath(parts ...string) string {
	return filepath.Join(append([]string{"testdata"}, parts...)...)
}

func TestRegistryClient_FetchPOM(t *testing.T) {
	pomData, err := os.ReadFile(testdataPath("registry", "org", "springframework",
		"spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.pom"))
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(pomData)
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
		w.Write(jarData)
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
	server := httptest.NewServer(http.FileServer(http.Dir(testdataPath("osv"))))
	defer server.Close()

	client := &httpRegistryClient{
		httpClient: server.Client(),
		osvURL:     server.URL,
		maxRetries: 0,
	}

	records, err := client.FetchOSVRecords(context.Background(), "5.3.18")
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
	data, err := os.ReadFile(testdataPath("osv", "x_RHLW-CVE-2023-20860-5.3.18.json"))
	require.NoError(t, err)

	rec, err := parseOSVFile(data)
	require.NoError(t, err)

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
