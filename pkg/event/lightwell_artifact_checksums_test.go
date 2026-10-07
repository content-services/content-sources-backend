package event

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchArtifactChecksumsJava(t *testing.T) {
	const testSHA = "c8f6dc3b2e92d7912f5e7b381085f5b30be378b4886367c4cad7358022512c47"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/lightwell/java/remediated/org/springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.jar.sha256"
		if r.URL.Path != expected {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(testSHA))
	}))
	defer srv.Close()

	items := []ArtifactChecksumRequest{
		{PackageName: "org.springframework:spring-core", Version: "5.3.18.rhlw-00003"},
	}

	result := FetchArtifactChecksums(
		context.Background(), srv.Client(),
		srv.URL+"/lightwell/java/remediated",
		"maven", items, "", "",
	)

	key := ArtifactChecksumKey("org.springframework:spring-core", "5.3.18.rhlw-00003")
	require.Contains(t, result, key)
	assert.Equal(t, testSHA, result[key]["spring-core-5.3.18.rhlw-00003.jar"])
}

func TestFetchArtifactChecksumsDeduplicatesVersions(t *testing.T) {
	fetchCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		_, _ = w.Write([]byte("abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"))
	}))
	defer srv.Close()

	items := []ArtifactChecksumRequest{
		{PackageName: "org.example:lib-a", Version: "1.0.0"},
		{PackageName: "org.example:lib-a", Version: "1.0.0"},
	}

	FetchArtifactChecksums(
		context.Background(), srv.Client(),
		srv.URL+"/repo", "maven", items, "", "",
	)

	assert.Equal(t, 1, fetchCount)
}

func TestFetchArtifactChecksums404(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	items := []ArtifactChecksumRequest{
		{PackageName: "org.example:lib-a", Version: "1.0.0"},
	}

	result := FetchArtifactChecksums(
		context.Background(), srv.Client(),
		srv.URL+"/repo", "maven", items, "", "",
	)

	assert.Empty(t, result)
}

func TestFetchArtifactChecksumsInvalidHex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-a-valid-sha256"))
	}))
	defer srv.Close()

	items := []ArtifactChecksumRequest{
		{PackageName: "org.example:lib-a", Version: "1.0.0"},
	}

	result := FetchArtifactChecksums(
		context.Background(), srv.Client(),
		srv.URL+"/repo", "maven", items, "", "",
	)

	assert.Empty(t, result)
}

func TestFetchArtifactChecksumsPythonReturnsEmpty(t *testing.T) {
	items := []ArtifactChecksumRequest{
		{PackageName: "requests", Version: "2.31.0.rhlw-00001"},
	}

	result := FetchArtifactChecksums(
		context.Background(), http.DefaultClient,
		"https://example.com/repo", "python", items, "", "",
	)

	assert.Nil(t, result)
}

func TestFetchArtifactChecksumsBasicAuth(t *testing.T) {
	var receivedUser, receivedPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUser, receivedPass, _ = r.BasicAuth()
		_, _ = w.Write([]byte("abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"))
	}))
	defer srv.Close()

	items := []ArtifactChecksumRequest{
		{PackageName: "org.example:lib-a", Version: "1.0.0"},
	}

	FetchArtifactChecksums(
		context.Background(), srv.Client(),
		srv.URL+"/repo", "maven", items, "testuser", "testpass",
	)

	assert.Equal(t, "testuser", receivedUser)
	assert.Equal(t, "testpass", receivedPass)
}

func TestFetchArtifactChecksumsTrimsWhitespace(t *testing.T) {
	const testSHA = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("  " + testSHA + "\n"))
	}))
	defer srv.Close()

	items := []ArtifactChecksumRequest{
		{PackageName: "org.example:lib-a", Version: "1.0.0"},
	}

	result := FetchArtifactChecksums(
		context.Background(), srv.Client(),
		srv.URL+"/repo", "maven", items, "", "",
	)

	key := ArtifactChecksumKey("org.example:lib-a", "1.0.0")
	require.Contains(t, result, key)
	assert.Equal(t, testSHA, result[key]["lib-a-1.0.0.jar"])
}

func TestFetchArtifactChecksumsDistinctPackagesSameVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a checksum derived from the artifact name so each package's
		// sidecar is distinguishable.
		if strings.Contains(r.URL.Path, "lib-a") {
			_, _ = w.Write([]byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
			return
		}
		if strings.Contains(r.URL.Path, "lib-b") {
			_, _ = w.Write([]byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	items := []ArtifactChecksumRequest{
		{PackageName: "org.example:lib-a", Version: "1.0.0"},
		{PackageName: "org.example:lib-b", Version: "1.0.0"},
	}

	result := FetchArtifactChecksums(
		context.Background(), srv.Client(),
		srv.URL+"/repo", "maven", items, "", "",
	)

	keyA := ArtifactChecksumKey("org.example:lib-a", "1.0.0")
	keyB := ArtifactChecksumKey("org.example:lib-b", "1.0.0")
	require.Contains(t, result, keyA)
	require.Contains(t, result, keyB)
	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		result[keyA]["lib-a-1.0.0.jar"])
	assert.Equal(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		result[keyB]["lib-b-1.0.0.jar"])
}

func TestFetchArtifactChecksumsHashFilenameFormat(t *testing.T) {
	const testSHA = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testSHA + "  lib-a-1.0.0.jar\n"))
	}))
	defer srv.Close()

	items := []ArtifactChecksumRequest{
		{PackageName: "org.example:lib-a", Version: "1.0.0"},
	}

	result := FetchArtifactChecksums(
		context.Background(), srv.Client(),
		srv.URL+"/repo", "maven", items, "", "",
	)

	key := ArtifactChecksumKey("org.example:lib-a", "1.0.0")
	require.Contains(t, result, key)
	assert.Equal(t, testSHA, result[key]["lib-a-1.0.0.jar"])
}

func TestExtractSHA256(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"bare hex", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
		{"hash with filename", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789  lib-a-1.0.0.jar", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
		{"uppercase hex", "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
		{"too short", "abcdef", ""},
		{"empty", "", ""},
		{"garbage", "not-a-hash", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractSHA256(tt.input))
		})
	}
}

func TestMavenSidecarURL(t *testing.T) {
	tests := []struct {
		name        string
		base        string
		packageName string
		version     string
		wantFile    string
		wantURL     string
		wantErr     bool
	}{
		{
			name:        "standard Maven coordinates",
			base:        "https://packages.redhat.com/lightwell/java/remediated",
			packageName: "org.springframework:spring-core",
			version:     "5.3.18.rhlw-00003",
			wantFile:    "spring-core-5.3.18.rhlw-00003.jar",
			wantURL:     "https://packages.redhat.com/lightwell/java/remediated/org/springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.jar.sha256",
		},
		{
			name:        "nested group ID",
			base:        "https://example.com/repo",
			packageName: "ch.qos.logback:logback-classic",
			version:     "1.4.14.rhlw-00001",
			wantFile:    "logback-classic-1.4.14.rhlw-00001.jar",
			wantURL:     "https://example.com/repo/ch/qos/logback/logback-classic/1.4.14.rhlw-00001/logback-classic-1.4.14.rhlw-00001.jar.sha256",
		},
		{
			name:        "trailing slash on base",
			base:        "https://example.com/repo/",
			packageName: "org.example:lib-a",
			version:     "1.0.0",
			wantFile:    "lib-a-1.0.0.jar",
			wantURL:     "https://example.com/repo/org/example/lib-a/1.0.0/lib-a-1.0.0.jar.sha256",
		},
		{
			name:        "missing colon",
			base:        "https://example.com/repo",
			packageName: "requests",
			version:     "1.0.0",
			wantErr:     true,
		},
		{
			name:        "empty group",
			base:        "https://example.com/repo",
			packageName: ":lib-a",
			version:     "1.0.0",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename, url, err := mavenSidecarURL(tt.base, tt.packageName, tt.version)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFile, filename)
			assert.Equal(t, tt.wantURL, url)
		})
	}
}
