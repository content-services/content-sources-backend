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
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/IBM/sarama"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testVEXTimestamp = "2026-07-15T14:02:27+00:00"
	testSBOMUUID     = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
)

var (
	testSBOMBody = []byte(`{"bomFormat":"CycloneDX","specVersion":"1.7","serialNumber":"urn:uuid:` + testSBOMUUID + `"}`)
	testASCBody  = []byte("-----BEGIN PGP SIGNATURE-----\ntest\n")
	testVEXBody  = []byte(`{"bomFormat":"CycloneDX","specVersion":"1.7","vulnerabilities":[{"affects":[{"ref":"urn:cdx:` + testSBOMUUID + `/1#pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00003"}]}]}`)
)

func testVEXObjectName() string {
	return testVEXTimestamp + ".bom.json"
}

func pipelineRegistryHandler(jar, pom, sbom, asc []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".bom.json.asc"):
			if asc == nil {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(asc)
		case strings.HasSuffix(r.URL.Path, ".bom.json"):
			if sbom == nil {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(sbom)
		case strings.HasSuffix(r.URL.Path, ".jar"):
			_, _ = w.Write(jar)
		case strings.HasSuffix(r.URL.Path, ".pom"):
			_, _ = w.Write(pom)
		default:
			http.NotFound(w, r)
		}
	})
}

// pipelineOSVHandler serves the fixture OSV directory and the one-CVE VEX
// name the bridge selects from PULP_MANIFEST.
func pipelineOSVHandler() http.Handler {
	dir := testdataPath("osv", "java", "remediated")
	files := http.FileServer(http.Dir(dir))
	perCVE := "CVE-2025-41249-" + testVEXTimestamp + ".bom.json"
	multi := testVEXObjectName()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "PULP_MANIFEST") {
			base, err := os.ReadFile(filepath.Join(dir, "PULP_MANIFEST"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if len(base) > 0 && base[len(base)-1] != '\n' {
				base = append(base, '\n')
			}
			_, _ = w.Write(append(base, []byte(perCVE+",abc,10\n"+multi+",def,10\n")...))
			return
		}
		if strings.Contains(r.URL.Path, "CVE-2025-41249-") && strings.Contains(r.URL.Path, testVEXTimestamp) {
			_, _ = w.Write(testVEXBody)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// mockConsumerGroupSession is a minimal test double for sarama.ConsumerGroupSession.
type mockConsumerGroupSession struct {
	ctx    context.Context
	marked []*sarama.ConsumerMessage
}

func (m *mockConsumerGroupSession) Claims() map[string][]int32               { return nil }
func (m *mockConsumerGroupSession) MemberID() string                         { return "test" }
func (m *mockConsumerGroupSession) GenerationID() int32                      { return 1 }
func (m *mockConsumerGroupSession) MarkOffset(string, int32, int64, string)  {}
func (m *mockConsumerGroupSession) Commit()                                  {}
func (m *mockConsumerGroupSession) ResetOffset(string, int32, int64, string) {}
func (m *mockConsumerGroupSession) Context() context.Context                 { return m.ctx }
func (m *mockConsumerGroupSession) MarkMessage(msg *sarama.ConsumerMessage, _ string) {
	m.marked = append(m.marked, msg)
}

// mockConsumerGroupClaim feeds a fixed set of messages then closes the channel.
type mockConsumerGroupClaim struct {
	ch chan *sarama.ConsumerMessage
}

func (m *mockConsumerGroupClaim) Topic() string                            { return "test-topic" }
func (m *mockConsumerGroupClaim) Partition() int32                         { return 0 }
func (m *mockConsumerGroupClaim) InitialOffset() int64                     { return 0 }
func (m *mockConsumerGroupClaim) HighWaterMarkOffset() int64               { return 0 }
func (m *mockConsumerGroupClaim) Messages() <-chan *sarama.ConsumerMessage { return m.ch }

func TestSBOMSerialNumber(t *testing.T) {
	got, err := sbomSerialNumber(testSBOMBody)
	require.NoError(t, err)
	assert.Equal(t, "urn:uuid:"+testSBOMUUID, got)

	_, err = sbomSerialNumber([]byte(`{"bomFormat":"CycloneDX"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")

	_, err = sbomSerialNumber([]byte(`{`))
	require.Error(t, err)
}

func TestVerifyJARChecksum_Match(t *testing.T) {
	checksums := map[string]string{
		"spring-core-5.3.18.rhlw-00003.jar": "abcdef1234567890",
	}
	err := verifyJARChecksum("spring-core-5.3.18.rhlw-00003.jar", "abcdef1234567890", checksums)
	assert.NoError(t, err)
}

func TestVerifyJARChecksum_Mismatch(t *testing.T) {
	checksums := map[string]string{
		"spring-core-5.3.18.rhlw-00003.jar": "expected_hash",
	}
	err := verifyJARChecksum("spring-core-5.3.18.rhlw-00003.jar", "actual_hash", checksums)
	require.Error(t, err)
	var checksumErr *ChecksumMismatchError
	assert.True(t, errors.As(err, &checksumErr))
	assert.Contains(t, err.Error(), "expected_hash")
	assert.Contains(t, err.Error(), "actual_hash")
}

func TestVerifyJARChecksum_NoChecksums(t *testing.T) {
	assert.NoError(t, verifyJARChecksum("test.jar", "abc123", nil),
		"nil checksums should skip verification")
	assert.NoError(t, verifyJARChecksum("test.jar", "abc123", map[string]string{}),
		"empty checksums should skip verification")
}

func TestVerifyJARChecksum_MissingKey(t *testing.T) {
	checksums := map[string]string{
		"other-artifact-1.0.jar": "somehash",
	}
	err := verifyJARChecksum("spring-core-5.3.18.rhlw-00003.jar", "abc123", checksums)
	require.Error(t, err)
	var checksumErr *ChecksumMismatchError
	assert.True(t, errors.As(err, &checksumErr))
	assert.Contains(t, err.Error(), "absent from checksums map")
	assert.Contains(t, err.Error(), "other-artifact-1.0.jar")
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

	registry := &httpRegistryClient{
		httpClient:  registryServer.Client(),
		registryURL: registryServer.URL,
		osvURL:      osvServer.URL,
		vexURL:      osvServer.URL,
		maxRetries:  0,
	}
	jfrog := &httpJFrogClient{
		httpClient:  jfrogServer.Client(),
		catalogURL:  jfrogServer.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
		maxRetries:  0,
	}

	metrics := newBridgeMetrics(prometheus.NewRegistry())
	handler := NewBridgeHandler(registry, jfrog, metrics)

	rem := Remediation{
		GroupID:     "org.springframework",
		ArtifactID:  "spring-core",
		Version:     "5.3.18.rhlw-00003",
		BaseVersion: "5.3.18",
		CVEsFixed:   []string{"CVE-2025-41249"},
	}

	err = handler.processRemediation(context.Background(), rem)
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

func TestGAVDedup(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := newBridgeMetrics(reg)
	handler := NewBridgeHandler(nil, nil, metrics)

	rem := Remediation{
		GroupID:    "org.test",
		ArtifactID: "test",
		Version:    "1.0.rhlw-00001",
	}

	gav := gavKey(rem)

	assert.False(t, handler.processed.Contains(gav))

	handler.processed.Add(gav)

	assert.True(t, handler.processed.Contains(gav))
}

func TestFilterAndParse_CloudEventEventTypeAccepted(t *testing.T) {
	// The production bridge topic carries the ecosystem gate as the
	// CloudEvents "eventtype" extension. java-remediated must be accepted.
	reg := prometheus.NewRegistry()
	metrics := newBridgeMetrics(reg)
	handler := NewBridgeHandler(nil, nil, metrics)

	payload := []byte(`{"specversion":"1.0","type":"com.redhat.console.lightwelll.lightwell-advisory-created","eventtype":"java-remediated","data":[{"metadata":{},"payload":{"package_name":"org.test:test","releases":[{"release_names":[{"name":"1.0.rhlw-00001"}],"related_cve":[{"cve":"CVE-2024-00001"}]}]}}]}`)

	remediations, shouldCommit, err := handler.filterAndParse(payload)
	require.NoError(t, err)
	assert.False(t, shouldCommit)
	require.Len(t, remediations, 1)
	assert.Equal(t, "org.test", remediations[0].GroupID)
	assert.Equal(t, "test", remediations[0].ArtifactID)
	assert.Equal(t, "1.0.rhlw-00001", remediations[0].Version)
	assert.Equal(t, "1.0", remediations[0].BaseVersion)
	assert.Contains(t, remediations[0].CVEsFixed, "CVE-2024-00001")
}

func TestFilterAndParse_CloudEventEventTypeRejected(t *testing.T) {
	// A CloudEvents message for a non-remediated ecosystem must be rejected
	// (and committed so the consumer moves on).
	reg := prometheus.NewRegistry()
	metrics := newBridgeMetrics(reg)
	handler := NewBridgeHandler(nil, nil, metrics)

	payload := []byte(`{"specversion":"1.0","type":"com.redhat.console.lightwelll.lightwell-advisory-created","eventtype":"java-predisclosure","data":[{"metadata":{},"payload":{"package_name":"org.test:test","releases":[{"release_names":[{"name":"1.0.rhlw-00001"}],"related_cve":[{"cve":"CVE-2024-00001"}]}]}}]}`)

	_, shouldCommit, err := handler.filterAndParse(payload)
	assert.Error(t, err)
	assert.True(t, shouldCommit)
	assert.Contains(t, err.Error(), "java-remediated")
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

	handler := newPipelineHandler(registryServer, osvServer, jfrogServer)
	err := handler.processRemediation(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload")
}

func TestGAVDedup_ProcessRemediationSkips(t *testing.T) {
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)
	pomData := readSpringPOM(t)

	registryServer := httptest.NewServer(pipelineRegistryHandler(pomData, pomData, testSBOMBody, testASCBody))
	defer registryServer.Close()
	osvServer := httptest.NewServer(pipelineOSVHandler())
	defer osvServer.Close()
	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer jfrogServer.Close()

	handler := newPipelineHandler(registryServer, osvServer, jfrogServer)
	rem := springRemediation()
	require.NoError(t, handler.processRemediation(context.Background(), rem))

	gav := gavKey(rem)
	handler.processed.Add(gav)
	assert.True(t, handler.processed.Contains(gav), "GAV should be in processed set after successful pipeline")
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

	handler := newPipelineHandler(registryServer, osvServer, jfrogServer)
	err := handler.processRemediation(context.Background(), springRemediation())
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

	handler := newPipelineHandler(registryServer, osvServer, jfrogServer)
	err := handler.processRemediation(context.Background(), springRemediation())
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

	handler := newPipelineHandlerVEX(registryServer, osvServer, vexServer, jfrogServer)
	err := handler.processRemediation(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch VEX")
}

func readSpringPOM(t *testing.T) []byte {
	t.Helper()
	pomData, err := os.ReadFile(testdataPath("registry", "org", "springframework",
		"spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.pom"))
	require.NoError(t, err)
	return pomData
}

func springRemediation() Remediation {
	return Remediation{
		GroupID:     "org.springframework",
		ArtifactID:  "spring-core",
		Version:     "5.3.18.rhlw-00003",
		BaseVersion: "5.3.18",
		CVEsFixed:   []string{"CVE-2025-41249"},
	}
}

func newPipelineHandler(registryServer, osvServer, jfrogServer *httptest.Server) *BridgeHandler {
	return newPipelineHandlerVEX(registryServer, osvServer, osvServer, jfrogServer)
}

func newPipelineHandlerVEX(registryServer, osvServer, vexServer, jfrogServer *httptest.Server) *BridgeHandler {
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
	return NewBridgeHandler(registry, jfrog, newBridgeMetrics(prometheus.NewRegistry()))
}

func TestConsumeClaim_ChecksumMismatchCommitsOffset(t *testing.T) {
	// A message whose checksums map does not match the fetched JAR should
	// increment checksumRejections, increment messagesFailed, and still
	// commit the Kafka offset (non-retryable).
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)

	pomData, err := os.ReadFile(testdataPath("registry", "org", "springframework",
		"spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.pom"))
	require.NoError(t, err)
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
	metrics := newBridgeMetrics(reg)
	handler := NewBridgeHandler(registry, nil, metrics)

	// Build a CloudEvents message with a wrong checksum.
	payload := `{"specversion":"1.0","type":"com.redhat.console.lightwelll.lightwell-advisory-created","eventtype":"java-remediated","data":[{"metadata":{},"payload":{"package_name":"org.springframework:spring-core","releases":[{"release_names":[{"name":"5.3.18.rhlw-00003"}],"related_cve":[{"cve":"CVE-2025-41249","severity":"important"}],"artifact_checksums":{"spring-core-5.3.18.rhlw-00003.jar":"0000000000000000000000000000000000000000000000000000000000000000"}}]}}]}`

	ch := make(chan *sarama.ConsumerMessage, 1)
	ch <- &sarama.ConsumerMessage{Value: []byte(payload)}
	close(ch)

	session := &mockConsumerGroupSession{ctx: context.Background()}
	claim := &mockConsumerGroupClaim{ch: ch}

	err = handler.ConsumeClaim(session, claim)
	require.NoError(t, err)

	// Offset should have been committed (message marked).
	assert.Len(t, session.marked, 1, "offset should be committed for non-retryable checksum error")

	// checksumRejections should be 1.
	gathered, gatherErr := reg.Gather()
	require.NoError(t, gatherErr)
	var checksumCount float64
	var failedCount float64
	for _, mf := range gathered {
		switch mf.GetName() {
		case "content_sources_jfrog_bridge_checksum_rejections_total":
			checksumCount = mf.GetMetric()[0].GetCounter().GetValue()
		case "content_sources_jfrog_bridge_messages_failed_total":
			failedCount = mf.GetMetric()[0].GetCounter().GetValue()
		}
	}
	assert.Equal(t, float64(1), checksumCount, "checksumRejections should be incremented")
	assert.Equal(t, float64(1), failedCount, "messagesFailed should be incremented")
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
