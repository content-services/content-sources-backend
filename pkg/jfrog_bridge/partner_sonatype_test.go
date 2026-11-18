// Some tests require local testdata fixtures (POM, OSV JSONs) under
// testdata/ that are not committed to git. They skip automatically
// when the files are absent.

package jfrog_bridge

import (
	"archive/zip"
	"bytes"
	"context"
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

func TestLoadSonatypeConfig(t *testing.T) {
	config.LoadedConfig.SonatypeBridge = config.SonatypeBridge{
		Enabled: true,
		S3: config.ObjectStore{
			URL:       "http://localhost:9002",
			Name:      "partner-bucket",
			Region:    "us-east-1",
			AccessKey: "partner-access-key",
			SecretKey: "partner-secret-key",
		},
		UsePathStyle:    true,
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

	cfg := loadSonatypeConfig()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, "http://localhost:9002", cfg.S3URL)
	assert.Equal(t, "partner-bucket", cfg.S3Bucket)
	assert.Equal(t, "us-east-1", cfg.S3Region)
	assert.Equal(t, "partner-access-key", cfg.S3AccessKey)
	assert.Equal(t, "partner-secret-key", cfg.S3SecretKey)
	assert.True(t, cfg.S3UsePathStyle)
	assert.Equal(t, "https://test.registry/java", cfg.RegistryURL)
	assert.Equal(t, "https://test.registry/osv", cfg.RegistryOSVURL)
	assert.Equal(t, "https://test.registry/vex", cfg.RegistryVEXURL)
	assert.Equal(t, "test-group", cfg.ConsumerGroupID)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 30, cfg.RequestTimeout)
	assert.Equal(t, "user", cfg.RegistryUsername)
	assert.Equal(t, "pass", cfg.RegistryPassword)
}

func TestLoadConfig_FlatSonatypeBlock(t *testing.T) {
	// The existing flat sonatype_bridge block drives the Sonatype partner:
	// without enabled: true the consumer stays off, and the consumer group
	// comes from sonatype_bridge.consumer_group_id.
	config.LoadedConfig.SonatypeBridge = config.SonatypeBridge{
		ConsumerGroupID: "content-sources-sonatype-bridge",
		S3: config.ObjectStore{
			Name:      "partner-bucket",
			Region:    "us-east-1",
			AccessKey: "ak",
			SecretKey: "sk",
		},
	}
	config.LoadedConfig.Loaded = true

	assert.False(t, sonatypePartner.enabled())
	assert.Equal(t, "content-sources-sonatype-bridge", sonatypePartner.consumerGroupID())

	config.LoadedConfig.SonatypeBridge.Enabled = true
	assert.True(t, sonatypePartner.enabled())
	handler, err := sonatypePartner.newHandler()
	require.NoError(t, err)
	assert.Equal(t, sonatypePartnerName, handler.Name())
}

func TestValidateS3(t *testing.T) {
	complete := sonatypeConfig{
		S3Bucket:    "partner-bucket",
		S3Region:    "us-east-1",
		S3AccessKey: "ak",
		S3SecretKey: "sk",
	}
	require.NoError(t, complete.validateS3())

	cases := map[string]func(c *sonatypeConfig){
		"sonatype_bridge.s3.name":       func(c *sonatypeConfig) { c.S3Bucket = "" },
		"sonatype_bridge.s3.region":     func(c *sonatypeConfig) { c.S3Region = "" },
		"sonatype_bridge.s3.access_key": func(c *sonatypeConfig) { c.S3AccessKey = "" },
		"sonatype_bridge.s3.secret_key": func(c *sonatypeConfig) { c.S3SecretKey = "" },
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			c := complete
			mutate(&c)
			err := c.validateS3()
			require.Error(t, err)
			assert.Contains(t, err.Error(), field)
		})
	}

	_, err := newS3Uploader(sonatypeConfig{})
	require.Error(t, err, "uploader must not be created without explicit partner credentials")
}

func TestSonatypePartner_MissingS3ConfigNotCreated(t *testing.T) {
	config.LoadedConfig.SonatypeBridge = config.SonatypeBridge{
		Enabled: true,
		S3:      config.ObjectStore{Name: "partner-bucket"},
	}
	config.LoadedConfig.Loaded = true

	_, err := sonatypePartner.newHandler()
	require.Error(t, err)
}

func TestSonatypeHandler_Name(t *testing.T) {
	assert.Equal(t, "sonatype", newSonatypeHandler(nil, nil).Name())
}

func TestSonatypeHandler_Status(t *testing.T) {
	config.LoadedConfig.SonatypeBridge = config.SonatypeBridge{
		Enabled: true,
		S3: config.ObjectStore{
			URL:       "http://s3.example",
			Name:      "partner-bucket",
			Region:    "us-east-1",
			AccessKey: "partner-access-key",
			SecretKey: "partner-secret-key",
		},
		RegistryURL:    "https://test.registry",
		RegistryOSVURL: "https://test.osv",
		RegistryVEXURL: "https://test.vex",
	}
	config.LoadedConfig.Loaded = true

	status := newSonatypeHandler(nil, nil).Status()
	assert.Equal(t, map[string]interface{}{
		"enabled":          true,
		"s3_url":           "http://s3.example",
		"s3_bucket":        "partner-bucket",
		"s3_region":        "us-east-1",
		"registry_url":     "https://test.registry",
		"registry_osv_url": "https://test.osv",
		"registry_vex_url": "https://test.vex",
	}, status, "status must not expose credentials")
}

func TestSonatypeHandler_Health(t *testing.T) {
	tests := map[string]struct {
		s3Status, registryStatus int
		healthy, s3Up, regUp     bool
	}{
		"all healthy":   {http.StatusOK, http.StatusOK, true, true, true},
		"s3 down":       {http.StatusInternalServerError, http.StatusOK, false, false, true},
		"registry down": {http.StatusOK, http.StatusInternalServerError, false, true, false},
		"both down":     {http.StatusInternalServerError, http.StatusInternalServerError, false, false, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.s3Status)
			}))
			defer s3Server.Close()
			registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.registryStatus)
			}))
			defer registryServer.Close()

			registry := &httpRegistryClient{httpClient: registryServer.Client(), osvURL: registryServer.URL}
			handler := newSonatypeHandler(registry, newTestS3Uploader(s3Server))

			healthy, body := handler.Health(context.Background())
			resp, ok := body.(sonatypeHealthResponse)
			require.True(t, ok)
			assert.Equal(t, tc.healthy, healthy)
			assert.Equal(t, tc.healthy, resp.Healthy)
			assert.Equal(t, tc.s3Up, resp.S3.Reachable)
			assert.Equal(t, tc.s3Up, resp.S3.Error == "")
			assert.Equal(t, tc.regUp, resp.Registry.Reachable)
			assert.Equal(t, tc.regUp, resp.Registry.Error == "")
		})
	}
}

func TestSonatypeHandler_DedupesCVEsBeforeFetchVEX(t *testing.T) {
	registry := newFakeRegistry()
	handler := newSonatypeHandler(registry, newFakeS3Uploader())

	rem := springRemediation()
	rem.CVEsFixed = []string{"CVE-2025-41249", "CVE-2025-41249"}
	require.NoError(t, handler.Process(context.Background(), rem))
	assert.Equal(t, []string{"CVE-2025-41249"}, registry.vexCVEs)
}

func TestSonatypeHandler_InvalidVEXTimestampNotUploaded(t *testing.T) {
	registry := newFakeRegistry()
	registry.vex.Timestamp = "not-a-date"
	uploader := newFakeS3Uploader()
	handler := newSonatypeHandler(registry, uploader)

	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse VEX timestamp")
	assert.Empty(t, uploader.objects)
}

func TestSonatypeVEXName(t *testing.T) {
	name, err := sonatypeVEXName(VEXDocument{vexCandidate: vexCandidate{CVEID: "CVE-2025-41249", Timestamp: testVEXTimestamp}})
	require.NoError(t, err)
	assert.Equal(t, testVEXDeliveredName, name)

	name, err = sonatypeVEXName(VEXDocument{vexCandidate: vexCandidate{Timestamp: testVEXTimestamp}})
	require.NoError(t, err)
	assert.Equal(t, "2026-07-15T14:02:27Z.bom.json", name)
}

func TestDedupeStrings(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, dedupeStrings([]string{"a", "b", "a"}))
	assert.Empty(t, dedupeStrings(nil))
}

func TestSonatypeHandler_SpringCore(t *testing.T) {
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
	s3Puts := make(map[string]int)
	capturedBodies := make(map[string][]byte)

	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			capturedBodies[r.URL.Path] = body
			s3Puts[r.URL.Path]++
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer s3Server.Close()

	registry := &httpRegistryClient{
		httpClient:  registryServer.Client(),
		registryURL: registryServer.URL,
		osvURL:      osvServer.URL,
		vexURL:      osvServer.URL,
		maxRetries:  0,
	}
	uploader := newTestS3Uploader(s3Server)

	handler := newSonatypeHandler(registry, uploader)

	rem := Remediation{
		GroupID:     "org.springframework",
		ArtifactID:  "spring-core",
		Version:     "5.3.18.rhlw-00003",
		BaseVersion: "5.3.18",
		CVEsFixed:   []string{"CVE-2025-41249"},
	}

	err = handler.Process(context.Background(), rem)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()

	base := "/" + testS3Bucket + "/packages/maven/org.springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003"
	zipKey := base + ".zip"
	require.Equal(t, 1, s3Puts[zipKey], "bundle should be uploaded")
	assert.Equal(t, map[string][]byte{
		"spring-core-5.3.18.rhlw-00003.jar": jarData,
		"spring-core-5.3.18.rhlw-00003.pom": pomData,
	}, readZip(t, capturedBodies[zipKey]))

	sbomKey := base + ".bom.json"
	vexKey := "/" + testS3Bucket + "/vex/" + testVEXDeliveredName
	assert.Equal(t, 1, s3Puts[sbomKey], "SBOM should be uploaded")
	assert.Equal(t, 1, s3Puts[sbomKey+".asc"], "SBOM signature should be uploaded")
	assert.Equal(t, 1, s3Puts[vexKey], "VEX should be uploaded")
	require.True(t, bytes.Equal(testVEXBody, capturedBodies[vexKey]), "VEX uploaded to S3 must be byte-identical to what the registry served")
	require.True(t, bytes.Equal(testSBOMBody, capturedBodies[sbomKey]), "SBOM uploaded to S3 must be byte-identical to what the registry served")
	require.True(t, bytes.Equal(testASCBody, capturedBodies[sbomKey+".asc"]), "SBOM signature uploaded to S3 must be byte-identical to what the registry served")
	assert.Len(t, s3Puts, 4)
}

func TestSonatypeHandler_UploadFailure(t *testing.T) {
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)
	pomData := readSpringPOM(t)

	registryServer := httptest.NewServer(pipelineRegistryHandler(pomData, pomData, testSBOMBody, testASCBody))
	defer registryServer.Close()
	osvServer := httptest.NewServer(pipelineOSVHandler())
	defer osvServer.Close()
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s3Server.Close()

	handler := newSonatypePipelineHandler(registryServer, osvServer, s3Server)
	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload")
}

func TestSonatypeHandler_SBOMFetchFailure(t *testing.T) {
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)
	pomData := readSpringPOM(t)

	var puts int
	registryServer := httptest.NewServer(pipelineRegistryHandler(pomData, pomData, nil, testASCBody))
	defer registryServer.Close()
	osvServer := httptest.NewServer(pipelineOSVHandler())
	defer osvServer.Close()
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts++
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer s3Server.Close()

	handler := newSonatypePipelineHandler(registryServer, osvServer, s3Server)
	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch SBOM")
	assert.Zero(t, puts)
}

func TestSonatypeHandler_SBOMSignatureFailure(t *testing.T) {
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)
	pomData := readSpringPOM(t)

	registryServer := httptest.NewServer(pipelineRegistryHandler(pomData, pomData, testSBOMBody, nil))
	defer registryServer.Close()
	osvServer := httptest.NewServer(pipelineOSVHandler())
	defer osvServer.Close()
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer s3Server.Close()

	handler := newSonatypePipelineHandler(registryServer, osvServer, s3Server)
	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature")
}

func TestSonatypeHandler_VEXFetchFailure(t *testing.T) {
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
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer s3Server.Close()

	handler := newSonatypePipelineHandlerVEX(registryServer, osvServer, vexServer, s3Server)
	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch VEX")
}

func newSonatypePipelineHandler(registryServer, osvServer, s3Server *httptest.Server) *sonatypeHandler {
	return newSonatypePipelineHandlerVEX(registryServer, osvServer, osvServer, s3Server)
}

func newSonatypePipelineHandlerVEX(registryServer, osvServer, vexServer, s3Server *httptest.Server) *sonatypeHandler {
	registry := &httpRegistryClient{
		httpClient:  registryServer.Client(),
		registryURL: registryServer.URL,
		osvURL:      osvServer.URL,
		vexURL:      vexServer.URL,
		maxRetries:  0,
	}
	return newSonatypeHandler(registry, newTestS3Uploader(s3Server))
}

func TestSonatypeConsumeClaim_ChecksumMismatchCommitsOffset(t *testing.T) {
	// A notification checksum that does not match the fetched JAR is
	// non-retryable: nothing is uploaded and the offset is committed.
	uploader := newFakeS3Uploader()
	reg := prometheus.NewRegistry()
	handler := newTestBridgeHandler(newSonatypeHandler(newFakeRegistry(), uploader), reg)

	payload := `{"specversion":"1.0","type":"com.redhat.console.lightwelll.lightwell-advisory-created","eventtype":"java-remediated","data":[{"metadata":{},"payload":{"package_name":"org.springframework:spring-core","releases":[{"release_names":[{"name":"5.3.18.rhlw-00003"}],"related_cve":[{"cve":"CVE-2025-41249","severity":"important"}],"artifact_checksums":{"spring-core-5.3.18.rhlw-00003.jar":"0000000000000000000000000000000000000000000000000000000000000000"}}]}}]}`

	session := &mockConsumerGroupSession{ctx: context.Background()}
	require.NoError(t, handler.ConsumeClaim(session, claimWith(payload)))

	assert.Len(t, session.marked, 1, "offset should be committed for non-retryable checksum error")
	assert.Empty(t, uploader.objects)
	assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_checksum_rejections_total", sonatypePartnerName))
	assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_messages_failed_total", sonatypePartnerName))
}

func TestSonatypeConsumeClaim_ObjectConflictCommitsOffset(t *testing.T) {
	uploader := newFakeS3Uploader()
	uploader.objects["vex/"+testVEXDeliveredName] = uploadedObject{data: []byte("other")}

	reg := prometheus.NewRegistry()
	handler := newTestBridgeHandler(newSonatypeHandler(newFakeRegistry(), uploader), reg)

	session := &mockConsumerGroupSession{ctx: context.Background()}
	require.NoError(t, handler.ConsumeClaim(session, claimWith(springPayload)))
	assert.Len(t, session.marked, 1, "offset should be committed for non-retryable object conflict")

	assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_conflict_rejections_total", sonatypePartnerName))
	assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_messages_failed_total", sonatypePartnerName))
}

func TestSonatypeHandler_UploadsBundleToS3(t *testing.T) {
	registry := newFakeRegistry()
	uploader := newFakeS3Uploader()
	handler := newSonatypeHandler(registry, uploader)

	require.NoError(t, handler.Process(context.Background(), springRemediation()))

	base := springKeyBase
	zipKey := base + ".zip"
	vexKey := "vex/" + testVEXDeliveredName
	assert.Equal(t, []string{
		zipKey,
		base + ".bom.json.asc",
		vexKey,
		base + ".bom.json",
	}, uploader.order, "SBOM must be uploaded last")

	expected := map[string]uploadedObject{
		base + ".bom.json":     {testSBOMBody, "application/json"},
		base + ".bom.json.asc": {testASCBody, "application/pgp-signature"},
		vexKey:                 {testVEXBody, "application/json"},
	}
	for key, want := range expected {
		got, ok := uploader.objects[key]
		require.True(t, ok, "expected %s to be uploaded", key)
		assert.Equal(t, want.data, got.data, key)
		assert.Equal(t, want.contentType, got.contentType, key)
	}

	bundle := uploader.objects[zipKey]
	assert.Equal(t, "application/zip", bundle.contentType)
	assert.Equal(t, map[string][]byte{
		"spring-core-5.3.18.rhlw-00003.jar": registry.jar,
		"spring-core-5.3.18.rhlw-00003.pom": registry.pom,
	}, readZip(t, bundle.data), "bundle holds JAR and POM only; SBOM/VEX stay outside")

	assert.Equal(t, []string{zipKey}, uploader.verified)
}

func TestSonatypeHandler_BundleIncludesSourcesAndJavadoc(t *testing.T) {
	registry := newFakeRegistry()
	registry.classifiers = map[string][]byte{
		"sources": []byte("sources-bytes"),
		"javadoc": []byte("javadoc-bytes"),
	}
	uploader := newFakeS3Uploader()
	handler := newSonatypeHandler(registry, uploader)

	require.NoError(t, handler.Process(context.Background(), springRemediation()))

	assert.Equal(t, map[string][]byte{
		"spring-core-5.3.18.rhlw-00003.jar":         registry.jar,
		"spring-core-5.3.18.rhlw-00003.pom":         registry.pom,
		"spring-core-5.3.18.rhlw-00003-sources.jar": []byte("sources-bytes"),
		"spring-core-5.3.18.rhlw-00003-javadoc.jar": []byte("javadoc-bytes"),
	}, readZip(t, uploader.objects[springKeyBase+".zip"].data))
}

func TestSonatypeHandler_ClassifierFetchError(t *testing.T) {
	registry := newFakeRegistry()
	registry.classifierErr = errors.New("HTTP 500")
	uploader := newFakeS3Uploader()
	handler := newSonatypeHandler(registry, uploader)

	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sources JAR")
	assert.Empty(t, uploader.objects)
}

func TestSonatypeHandler_EmptyGroupOmitsNamespace(t *testing.T) {
	registry := newFakeRegistry()
	registry.sbom = []byte(`{"serialNumber":"urn:uuid:` + testSBOMUUID + `","components":[{"name":"spring-core","version":"5.3.18.rhlw-00003","purl":"pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00003"}]}`)
	uploader := newFakeS3Uploader()
	handler := newSonatypeHandler(registry, uploader)

	require.NoError(t, handler.Process(context.Background(), springRemediation()))
	assert.Contains(t, uploader.objects, "packages/maven/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.zip")
}

func TestSonatypeHandler_SBOMMismatchNotUploaded(t *testing.T) {
	serial := `"serialNumber":"urn:uuid:` + testSBOMUUID + `",`
	tests := map[string]string{
		"name":    `{` + serial + `"components":[{"group":"org.springframework","name":"spring-beans","version":"5.3.18.rhlw-00003","purl":"pkg:maven/org.springframework/spring-beans@5.3.18.rhlw-00003"}]}`,
		"version": `{` + serial + `"components":[{"group":"org.springframework","name":"spring-core","version":"5.3.18.rhlw-00004","purl":"pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00004"}]}`,
		"group":   `{` + serial + `"components":[{"group":"org.other","name":"spring-core","version":"5.3.18.rhlw-00003","purl":"pkg:maven/org.other/spring-core@5.3.18.rhlw-00003"}]}`,
		"missing": `{` + serial + `"bomFormat":"CycloneDX"}`,
	}
	for name, sbom := range tests {
		t.Run(name, func(t *testing.T) {
			registry := newFakeRegistry()
			registry.sbom = []byte(sbom)
			uploader := newFakeS3Uploader()
			handler := newSonatypeHandler(registry, uploader)

			err := handler.Process(context.Background(), springRemediation())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "SBOM coordinates")
			assert.Empty(t, uploader.objects)
		})
	}
}

func TestSonatypeHandler_S3UploadError(t *testing.T) {
	uploader := newFakeS3Uploader()
	uploader.uploadErr = errors.New("access denied")
	handler := newSonatypeHandler(newFakeRegistry(), uploader)

	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload bundle")
	assert.Empty(t, uploader.verified)
}

func TestSonatypeHandler_S3VerifyError(t *testing.T) {
	uploader := newFakeS3Uploader()
	uploader.verifyErr = errors.New("not found")
	handler := newSonatypeHandler(newFakeRegistry(), uploader)

	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify delivery")
}

func TestSonatypeHandler_EmbargoedOSVNotUploaded(t *testing.T) {
	registry := newFakeRegistry()
	registry.osv = []OSVRecord{{CVEID: "LTWL-0001"}}
	uploader := newFakeS3Uploader()
	handler := newSonatypeHandler(registry, uploader)

	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	var embargoErr *EmbargoError
	assert.True(t, errors.As(err, &embargoErr))
	assert.Empty(t, uploader.objects, "nothing may reach the partner bucket on embargo")
}

func TestSonatypeHandler_ChecksumMismatchNotUploaded(t *testing.T) {
	uploader := newFakeS3Uploader()
	handler := newSonatypeHandler(newFakeRegistry(), uploader)

	rem := springRemediation()
	rem.Checksums = map[string]string{"spring-core-5.3.18.rhlw-00003.jar": "deadbeef"}
	err := handler.Process(context.Background(), rem)
	require.Error(t, err)
	var checksumErr *ChecksumMismatchError
	assert.True(t, errors.As(err, &checksumErr))
	assert.Empty(t, uploader.objects)
}

func TestSonatypeHandler_S3PathsOverHTTP(t *testing.T) {
	server, reqs, mu := newRecordingS3Server(200)
	defer server.Close()

	handler := newSonatypeHandler(newFakeRegistry(), newTestS3Uploader(server))
	require.NoError(t, handler.Process(context.Background(), springRemediation()))

	mu.Lock()
	defer mu.Unlock()
	paths := map[string]string{}
	for _, r := range *reqs {
		paths[r.path] = r.method
	}
	zip := "/" + testS3Bucket + "/" + springKeyBase + ".zip"
	assert.Contains(t, paths, zip)
	assert.Contains(t, paths, "/"+testS3Bucket+"/"+springKeyBase+".bom.json")
	assert.Contains(t, paths, "/"+testS3Bucket+"/vex/"+testVEXDeliveredName)
	// 4 PUTs + 1 HEAD (verify on the zip)
	assert.Len(t, *reqs, 5)
	last := (*reqs)[len(*reqs)-1]
	assert.Equal(t, "HEAD", last.method)
	assert.Equal(t, zip, last.path)
}

func TestSonatypeHandler_RerunSkipsIdenticalObjects(t *testing.T) {
	uploader := newFakeS3Uploader()
	handler := newSonatypeHandler(newFakeRegistry(), uploader)

	require.NoError(t, handler.Process(context.Background(), springRemediation()))
	require.NoError(t, handler.Process(context.Background(), springRemediation()),
		"a retry must succeed when every object already exists with identical content")
	assert.Len(t, uploader.objects, 4)
	assert.Len(t, uploader.verified, 2)
}

func TestSonatypeHandler_VEXConflictStopsDelivery(t *testing.T) {
	uploader := newFakeS3Uploader()
	vexKey := "vex/" + testVEXDeliveredName
	uploader.objects[vexKey] = uploadedObject{data: []byte(`{"different":true}`), contentType: "application/json"}
	handler := newSonatypeHandler(newFakeRegistry(), uploader)

	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	var conflictErr *ObjectConflictError
	require.True(t, errors.As(err, &conflictErr))
	assert.Equal(t, vexKey, conflictErr.Key)
	assert.NotContains(t, uploader.objects, springKeyBase+".bom.json", "SBOM marks completion and must not be uploaded")
	assert.Empty(t, uploader.verified)
}

func TestSonatypeHandler_UploadsBundledVEX(t *testing.T) {
	registry := newFakeRegistry()
	registry.vex = VEXDocument{
		vexCandidate: vexCandidate{Filename: testVEXTimestamp + ".bom.json", Timestamp: testVEXTimestamp},
		Data:         []byte("bundled"),
	}
	uploader := newFakeS3Uploader()
	handler := newSonatypeHandler(registry, uploader)

	rem := springRemediation()
	rem.CVEsFixed = []string{"CVE-2025-41249", "CVE-2023-20860"}
	require.NoError(t, handler.Process(context.Background(), rem))
	assert.Equal(t, []byte("bundled"), uploader.objects["vex/2026-07-15T14:02:27Z.bom.json"].data)
	assert.Equal(t, springKeyBase+".bom.json", uploader.order[len(uploader.order)-1])
}

func TestSonatypeHandler_PassesSBOMSerialToFetchVEX(t *testing.T) {
	registry := newFakeRegistry()
	handler := newSonatypeHandler(registry, newFakeS3Uploader())

	require.NoError(t, handler.Process(context.Background(), springRemediation()))
	assert.Equal(t, "urn:uuid:"+testSBOMUUID, registry.vexSerial)
}

func TestSonatypeHandler_SBOMWithoutSerialNotUploaded(t *testing.T) {
	registry := newFakeRegistry()
	registry.sbom = []byte(`{"components":[{"group":"org.springframework","name":"spring-core","version":"5.3.18.rhlw-00003","purl":"pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00003"}]}`)
	uploader := newFakeS3Uploader()
	handler := newSonatypeHandler(registry, uploader)

	err := handler.Process(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "serialNumber")
	assert.Empty(t, uploader.objects)
}

func TestSonatypeHandler_NoPrefixKeysStartWithPackages(t *testing.T) {
	server, reqs, mu := newRecordingS3Server(200)
	defer server.Close()

	handler := newSonatypeHandler(newFakeRegistry(), newTestS3Uploader(server))
	require.NoError(t, handler.Process(context.Background(), springRemediation()))

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, *reqs)
	for _, r := range *reqs {
		assert.True(t, strings.HasPrefix(r.path, "/"+testS3Bucket+"/packages/") ||
			strings.HasPrefix(r.path, "/"+testS3Bucket+"/vex/"), r.path)
	}
}

const springKeyBase = "packages/maven/org.springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003"

func readZip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		b, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		out[f.Name] = b
	}
	return out
}
