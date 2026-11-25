// Some tests require local testdata fixtures (POM, OSV JSONs) under
// testdata/ that are not committed to git. They skip automatically
// when the files are absent.

package jfrog_bridge

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadJFrogConfig(t *testing.T) {
	config.LoadedConfig.JFrogBridge = config.JFrogBridge{
		Enabled:         true,
		CatalogURL:      "https://test.jfrog.io",
		CatalogRepo:     "test-repo",
		CatalogToken:    "test-token",
		RegistryURL:     "https://test.registry/java",
		RegistryOSVURL:  "https://test.registry/osv",
		RegistryVEXURL:  "https://test.registry/vex",
		ConsumerGroupID: "test-group",
		MaxRetries:      5,
		RequestTimeout:  30,
	}
	config.LoadedConfig.Clients.Lightwell = config.Lightwell{
		Username: "user",
		Password: "pass",
	}
	config.LoadedConfig.Loaded = true

	cfg := loadJFrogConfig()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, "https://test.jfrog.io", cfg.CatalogURL)
	assert.Equal(t, "test-repo", cfg.CatalogRepo)
	assert.Equal(t, "test-token", cfg.CatalogToken)
	assert.Equal(t, "https://test.registry/java", cfg.RegistryURL)
	assert.Equal(t, "https://test.registry/osv", cfg.RegistryOSVURL)
	assert.Equal(t, "https://test.registry/vex", cfg.RegistryVEXURL)
	assert.Equal(t, "test-group", cfg.ConsumerGroupID)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 30, cfg.RequestTimeout)
	assert.Equal(t, "user", cfg.RegistryUsername)
	assert.Equal(t, "pass", cfg.RegistryPassword)
}

func TestJFrogHandler_UploadMatchesCurrentPath(t *testing.T) {
	registry := newFakeRegistry()
	jfrog := newFakeJFrogClient()
	handler := newJFrogHandler(registry, jfrog, "https://registry.example/java")

	require.NoError(t, handler.Process(context.Background(), springRemediation()))

	base := "org/springframework/spring-core/5.3.18.rhlw-00003"
	jarPath := base + "/spring-core-5.3.18.rhlw-00003.jar"
	sbomPath := base + "/sbom/spring-core-5.3.18.rhlw-00003.bom.json"
	vexPath := base + "/vex/spring-core-5.3.18.rhlw-00003.bom.json"
	metadataPath := "org/springframework/spring-core/maven-metadata.xml"

	assert.Equal(t, []string{
		jarPath,
		base + "/spring-core-5.3.18.rhlw-00003.pom",
		sbomPath,
		sbomPath + ".asc",
		vexPath,
		metadataPath,
	}, jfrog.order)

	expected := map[string]uploadedObject{
		jarPath: {registry.jar, "application/java-archive"},
		base + "/spring-core-5.3.18.rhlw-00003.pom": {registry.pom, "application/xml"},
		sbomPath:          {testSBOMBody, "application/json"},
		sbomPath + ".asc": {testASCBody, "application/pgp-signature"},
		vexPath:           {testVEXBody, "application/json"},
	}
	for key, want := range expected {
		got := jfrog.objects[key]
		assert.Equal(t, want.data, got.data, key)
		assert.Equal(t, want.contentType, got.contentType, key)
	}
	assert.Equal(t, "application/xml", jfrog.objects[metadataPath].contentType)

	assert.Equal(t, map[string]map[string]string{
		jarPath: {
			"catalog.name":                   "org.springframework:spring-core",
			"catalog.version":                "5.3.18.rhlw-00003",
			"catalog.compatible_with":        "5.3.18",
			"catalog.vendor_remote_repo_url": "https://registry.example/java",
			"license":                        "Apache-2.0",
		},
	}, jfrog.properties)
	assert.Equal(t, []string{jarPath}, jfrog.verified)

	assert.Empty(t, registry.classifierCalls, "JFrog does not fetch classifier JARs")
	assert.Equal(t, "urn:uuid:"+testSBOMUUID, registry.vexSerial)
}

func TestJFrogHandler_PassesNotificationCVEsUnchanged(t *testing.T) {
	registry := newFakeRegistry()
	handler := newJFrogHandler(registry, newFakeJFrogClient(), "")

	rem := springRemediation()
	rem.CVEsFixed = []string{"CVE-2025-41249", "CVE-2025-41249"}
	require.NoError(t, handler.Process(context.Background(), rem))
	assert.Equal(t, rem.CVEsFixed, registry.vexCVEs)
}

func TestJFrogHandler_EmbargoedOSVNotUploaded(t *testing.T) {
	registry := newFakeRegistry()
	registry.osv = []OSVRecord{{CVEID: "LTWL-0001"}}
	jfrog := newFakeJFrogClient()
	handler := newJFrogHandler(registry, jfrog, "")

	err := handler.Process(context.Background(), springRemediation())
	var embargoErr *EmbargoError
	require.True(t, errors.As(err, &embargoErr))
	assert.Empty(t, jfrog.objects)
}

func TestJFrogHandler_ChecksumMismatchNotUploaded(t *testing.T) {
	jfrog := newFakeJFrogClient()
	handler := newJFrogHandler(newFakeRegistry(), jfrog, "")

	rem := springRemediation()
	rem.Checksums = map[string]string{"spring-core-5.3.18.rhlw-00003.jar": "deadbeef"}
	err := handler.Process(context.Background(), rem)
	var checksumErr *ChecksumMismatchError
	require.True(t, errors.As(err, &checksumErr))
	assert.Empty(t, jfrog.objects)
}

func TestJFrogHandler_Name(t *testing.T) {
	assert.Equal(t, "jfrog", newJFrogHandler(nil, nil, "").Name())
}

func TestJFrogHandler_Status(t *testing.T) {
	config.LoadedConfig.JFrogBridge = config.JFrogBridge{
		Enabled:        true,
		CatalogURL:     "https://test.jfrog.io",
		CatalogRepo:    "test-repo",
		CatalogToken:   "secret-token",
		RegistryURL:    "https://test.registry",
		RegistryOSVURL: "https://test.osv",
	}
	config.LoadedConfig.Loaded = true

	status := newJFrogHandler(nil, nil, "").Status()
	assert.Equal(t, true, status["enabled"])
	assert.Equal(t, "https://test.jfrog.io", status["catalog_url"])
	assert.Equal(t, "test-repo", status["catalog_repo"])
	assert.NotContains(t, status, "catalog_token")
}

func TestProcessRemediation_SpringCore(t *testing.T) {
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)
	pomPath := testdataPath("registry", "org", "springframework",
		"spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.pom")
	pomData, err := os.ReadFile(pomPath)
	require.NoError(t, err)

	// Use POM data as a stand-in for JAR when real JAR is absent
	jarData := pomData
	jarPath := testdataPath("registry", "org", "springframework",
		"spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.jar")
	if _, err := os.Stat(jarPath); err == nil {
		jarData, _ = os.ReadFile(jarPath)
	}

	registryServer := httptest.NewServer(pipelineRegistryHandler(jarData, pomData, testSBOMBody, testASCBody))
	defer registryServer.Close()

	osvServer := httptest.NewServer(pipelineOSVHandler())
	defer osvServer.Close()

	var mu sync.Mutex
	jfrogPuts := make(map[string]int)
	capturedBodies := make(map[string][]byte)
	var capturedProperties string

	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case http.MethodPut:
			if strings.Contains(r.URL.RawQuery, "properties=") {
				capturedProperties = r.URL.RawQuery
			} else {
				body, _ := io.ReadAll(r.Body)
				capturedBodies[r.URL.Path] = body
			}
			jfrogPuts[r.URL.Path]++
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer jfrogServer.Close()

	handler := newJFrogPipelineHandler(registryServer, osvServer, jfrogServer)

	err = handler.Process(context.Background(), springRemediation())
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()

	// Assert: JAR uploaded
	jarKey := "/artifactory/test-repo/org/springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.jar"
	assert.Equal(t, 1, jfrogPuts[jarKey], "JAR should be uploaded")

	// Assert: POM uploaded
	pomKey := "/artifactory/test-repo/org/springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.pom"
	assert.Equal(t, 1, jfrogPuts[pomKey], "POM should be uploaded")

	base := "/artifactory/test-repo/org/springframework/spring-core/5.3.18.rhlw-00003"
	sbomKey := base + "/sbom/spring-core-5.3.18.rhlw-00003.bom.json"
	assert.Equal(t, 1, jfrogPuts[sbomKey], "SBOM should be uploaded")
	assert.Equal(t, 1, jfrogPuts[sbomKey+".asc"], "SBOM signature should be uploaded")
	vexKey := base + "/vex/spring-core-5.3.18.rhlw-00003.bom.json"
	assert.Equal(t, 1, jfrogPuts[vexKey], "VEX should be uploaded")
	require.True(t, bytes.Equal(testVEXBody, capturedBodies[vexKey]), "VEX uploaded to JFrog must be byte-identical to what the registry served")
	require.True(t, bytes.Equal(testSBOMBody, capturedBodies[sbomKey]), "SBOM uploaded to JFrog must be byte-identical to what the registry served")
	require.True(t, bytes.Equal(testASCBody, capturedBodies[sbomKey+".asc"]), "SBOM signature uploaded to JFrog must be byte-identical to what the registry served")

	metaKey := "/artifactory/test-repo/org/springframework/spring-core/maven-metadata.xml"
	assert.Equal(t, 1, jfrogPuts[metaKey], "maven-metadata.xml should be uploaded")

	assert.Contains(t, capturedProperties, "catalog.name=org.springframework%3Aspring-core")
	assert.Contains(t, capturedProperties, "catalog.compatible_with=5.3.18")
}

func TestProcessRemediation_UploadFailure(t *testing.T) {
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)
	pomData := readSpringPOM(t)

	registryServer := httptest.NewServer(pipelineRegistryHandler(pomData, pomData, testSBOMBody, testASCBody))
	defer registryServer.Close()
	osvServer := httptest.NewServer(pipelineOSVHandler())
	defer osvServer.Close()
	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer jfrogServer.Close()

	handler := newJFrogPipelineHandler(registryServer, osvServer, jfrogServer)
	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload")
}

func TestProcessRemediation_SBOMFetchFailure(t *testing.T) {
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)
	pomData := readSpringPOM(t)

	var puts int
	registryServer := httptest.NewServer(pipelineRegistryHandler(pomData, pomData, nil, testASCBody))
	defer registryServer.Close()
	osvServer := httptest.NewServer(pipelineOSVHandler())
	defer osvServer.Close()
	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts++
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer jfrogServer.Close()

	handler := newJFrogPipelineHandler(registryServer, osvServer, jfrogServer)
	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch SBOM")
	assert.Zero(t, puts)
}

func TestProcessRemediation_SBOMSignatureFailure(t *testing.T) {
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)
	pomData := readSpringPOM(t)

	registryServer := httptest.NewServer(pipelineRegistryHandler(pomData, pomData, testSBOMBody, nil))
	defer registryServer.Close()
	osvServer := httptest.NewServer(pipelineOSVHandler())
	defer osvServer.Close()
	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer jfrogServer.Close()

	handler := newJFrogPipelineHandler(registryServer, osvServer, jfrogServer)
	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature")
}

func TestProcessRemediation_VEXFetchFailure(t *testing.T) {
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)
	pomData := readSpringPOM(t)

	registryServer := httptest.NewServer(pipelineRegistryHandler(pomData, pomData, testSBOMBody, testASCBody))
	defer registryServer.Close()
	osvServer := httptest.NewServer(http.FileServer(http.Dir(testdataPath("osv", "java", "remediated"))))
	defer osvServer.Close()
	vexServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer vexServer.Close()
	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer jfrogServer.Close()

	handler := newJFrogPipelineHandlerVEX(registryServer, osvServer, vexServer, jfrogServer)
	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch VEX")
}

func newJFrogPipelineHandler(registryServer, osvServer, jfrogServer *httptest.Server) *jfrogHandler {
	return newJFrogPipelineHandlerVEX(registryServer, osvServer, osvServer, jfrogServer)
}

func newJFrogPipelineHandlerVEX(registryServer, osvServer, vexServer, jfrogServer *httptest.Server) *jfrogHandler {
	registry := &httpRegistryClient{
		httpClient:  registryServer.Client(),
		registryURL: registryServer.URL,
		osvURL:      osvServer.URL,
		vexURL:      vexServer.URL,
		maxRetries:  0,
	}
	jfrog := &httpJFrogClient{
		httpClient:  jfrogServer.Client(),
		catalogURL:  jfrogServer.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
		maxRetries:  0,
	}
	return newJFrogHandler(registry, jfrog, "")
}

func TestConsumeClaim_ChecksumMismatchCommitsOffset(t *testing.T) {
	// A message whose checksums map does not match the fetched JAR should
	// increment checksumRejections, increment messagesFailed, and still
	// commit the Kafka offset (non-retryable).
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)

	pomData := readSpringPOM(t)
	jarData := pomData

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".jar") {
			_, _ = w.Write(jarData)
		} else if strings.HasSuffix(r.URL.Path, ".pom") {
			_, _ = w.Write(pomData)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer registryServer.Close()

	registry := &httpRegistryClient{
		httpClient:  registryServer.Client(),
		registryURL: registryServer.URL,
		osvURL:      registryServer.URL,
		maxRetries:  0,
	}

	reg := prometheus.NewRegistry()
	handler := newTestBridgeHandler(newJFrogHandler(registry, nil, ""), reg)

	// Build a CloudEvents message with a wrong checksum.
	payload := `{"specversion":"1.0","type":"com.redhat.console.lightwelll.lightwell-advisory-created","eventtype":"java-remediated","data":[{"metadata":{},"payload":{"package_name":"org.springframework:spring-core","releases":[{"release_names":[{"name":"5.3.18.rhlw-00003"}],"related_cve":[{"cve":"CVE-2025-41249","severity":"important"}],"artifact_checksums":{"spring-core-5.3.18.rhlw-00003.jar":"0000000000000000000000000000000000000000000000000000000000000000"}}]}}]}`

	session := &mockConsumerGroupSession{ctx: context.Background()}
	require.NoError(t, handler.ConsumeClaim(session, claimWith(payload)))

	// Offset should have been committed (message marked).
	assert.Len(t, session.marked, 1, "offset should be committed for non-retryable checksum error")
	assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_checksum_rejections_total", jfrogPartnerName),
		"checksumRejections should be incremented")
	assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_messages_failed_total", jfrogPartnerName),
		"messagesFailed should be incremented")
}

func TestGenerateMavenMetadata_SpecialChars(t *testing.T) {
	data := generateMavenMetadata("<foo>&bar", "test-artifact", "1.0.0")
	require.NotEmpty(t, data)

	xmlStr := string(data)
	assert.Contains(t, xmlStr, "&lt;foo&gt;&amp;bar")
	assert.Contains(t, xmlStr, "<artifactId>test-artifact</artifactId>")
	assert.Contains(t, xmlStr, "<version>1.0.0</version>")

	var meta mavenMetadata
	require.NoError(t, xml.Unmarshal(data, &meta), "output must be well-formed XML")
	assert.Equal(t, "<foo>&bar", meta.GroupID)
}

func newJFrogHealthHandler(jfrogStatus, registryStatus int) (*jfrogHandler, func()) {
	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(jfrogStatus)
	}))
	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(registryStatus)
	}))
	jfrog := &httpJFrogClient{
		httpClient:  jfrogServer.Client(),
		catalogURL:  jfrogServer.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
	}
	registry := &httpRegistryClient{
		httpClient: registryServer.Client(),
		osvURL:     registryServer.URL,
	}
	return newJFrogHandler(registry, jfrog, ""), func() {
		jfrogServer.Close()
		registryServer.Close()
	}
}

func TestJFrogHandler_Health(t *testing.T) {
	tests := map[string]struct {
		jfrogStatus, registryStatus int
		healthy, jfrogUp, regUp     bool
	}{
		"all healthy":   {http.StatusOK, http.StatusOK, true, true, true},
		"jfrog down":    {http.StatusInternalServerError, http.StatusOK, false, false, true},
		"registry down": {http.StatusOK, http.StatusInternalServerError, false, true, false},
		"both down":     {http.StatusInternalServerError, http.StatusInternalServerError, false, false, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler, cleanup := newJFrogHealthHandler(tc.jfrogStatus, tc.registryStatus)
			defer cleanup()

			healthy, body := handler.Health(context.Background())
			resp, ok := body.(jfrogHealthResponse)
			require.True(t, ok)
			assert.Equal(t, tc.healthy, healthy)
			assert.Equal(t, tc.healthy, resp.Healthy)
			assert.Equal(t, tc.jfrogUp, resp.JFrog.Reachable)
			assert.Equal(t, tc.jfrogUp, resp.JFrog.Error == "")
			assert.Equal(t, tc.regUp, resp.Registry.Reachable)
			assert.Equal(t, tc.regUp, resp.Registry.Error == "")
		})
	}
}
