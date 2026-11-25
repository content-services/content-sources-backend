package jfrog_bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/IBM/sarama"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

const (
	testVEXTimestamp     = "2026-07-15T14:02:27+00:00"
	testVEXDeliveredName = "CVE-2025-41249-2026-07-15T14:02:27Z.bom.json"
	testSBOMUUID         = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
)

var (
	testSBOMBody = []byte(`{"bomFormat":"CycloneDX","specVersion":"1.7","serialNumber":"urn:uuid:` + testSBOMUUID + `","metadata":{"timestamp":"2026-11-04T17:26:03Z"},"components":[{"type":"library","group":"org.springframework","name":"spring-core","version":"5.3.18.rhlw-00003","purl":"pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00003?repository_url=https://packages.redhat.com/lightwell/java/remediated/&type=jar"}]}`)
	testASCBody  = []byte("-----BEGIN PGP SIGNATURE-----\ntest\n")
	testVEXBody  = []byte(`{"bomFormat":"CycloneDX","specVersion":"1.7","vulnerabilities":[{"affects":[{"ref":"urn:cdx:` + testSBOMUUID + `/1#pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00003"}]}]}`)
)

const springPayload = `{"specversion":"1.0","type":"com.redhat.console.lightwelll.lightwell-advisory-created","eventtype":"java-remediated","data":[{"metadata":{},"payload":{"package_name":"org.springframework:spring-core","releases":[{"release_names":[{"name":"5.3.18.rhlw-00003"}],"related_cve":[{"cve":"CVE-2025-41249","severity":"important"}]}]}}]}`

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
		case strings.HasSuffix(r.URL.Path, "-sources.jar"), strings.HasSuffix(r.URL.Path, "-javadoc.jar"):
			http.NotFound(w, r)
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
// name the bridge selects from PULP_MANIFEST. The multi-CVE entry is listed
// to prove a one-CVE release does not fetch it.
func pipelineOSVHandler() http.Handler {
	dir := testdataPath("osv", "java", "remediated")
	files := http.FileServer(http.Dir(dir))
	perCVE := "CVE-2025-41249-" + testVEXTimestamp + ".bom.json"
	multi := testVEXTimestamp + ".bom.json"
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
		if strings.Contains(r.URL.Path, testVEXTimestamp) {
			if !strings.HasPrefix(path.Base(r.URL.Path), "CVE-2025-41249-") {
				http.Error(w, "unexpected VEX request", http.StatusTeapot)
				return
			}
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

func claimWith(payloads ...string) *mockConsumerGroupClaim {
	ch := make(chan *sarama.ConsumerMessage, len(payloads))
	for _, p := range payloads {
		ch <- &sarama.ConsumerMessage{Value: []byte(p)}
	}
	close(ch)
	return &mockConsumerGroupClaim{ch: ch}
}

// fakePartner is a PartnerHandler that records calls and returns a fixed error.
type fakePartner struct {
	name    string
	err     error
	mu      sync.Mutex
	calls   []Remediation
	healthy bool
}

func (f *fakePartner) Name() string { return f.name }

func (f *fakePartner) Process(_ context.Context, rem Remediation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, rem)
	return f.err
}

func (f *fakePartner) Status() map[string]interface{} {
	return map[string]interface{}{"partner": f.name}
}

func (f *fakePartner) Health(context.Context) (bool, interface{}) {
	return f.healthy, map[string]interface{}{"healthy": f.healthy}
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

func newTestBridgeHandler(partner PartnerHandler, reg *prometheus.Registry) *BridgeHandler {
	if reg == nil {
		reg = prometheus.NewRegistry()
	}
	return NewBridgeHandler(partner, newBridgeMetrics(reg))
}

// counterValue returns the value of a content_sources_bridge_* counter for a partner.
func counterValue(t *testing.T, reg *prometheus.Registry, name, partnerName string) float64 {
	t.Helper()
	gathered, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range gathered {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == partnerLabel && l.GetValue() == partnerName {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// echoContextWithBridge returns an echo context with the partner's bridge
// handler set, as checkPartnerAccessible does for routed requests.
func echoContextWithBridge(c echo.Context, bh *BridgeHandler) echo.Context {
	c.Set(bridgeContextKey, bh)
	return c
}

type fakeRegistry struct {
	jar, pom, sbom, asc []byte
	vex                 VEXDocument
	vexSerial           string
	vexCVEs             []string
	osv                 []OSVRecord
	classifiers         map[string][]byte
	classifierErr       error
	classifierCalls     []string
}

func (f *fakeRegistry) FetchJAR(_ context.Context, _, _, _ string) ([]byte, string, error) {
	sum := sha256.Sum256(f.jar)
	return f.jar, hex.EncodeToString(sum[:]), nil
}

func (f *fakeRegistry) FetchPOM(_ context.Context, _, _, _ string) ([]byte, error) {
	return f.pom, nil
}

func (f *fakeRegistry) FetchClassifier(_ context.Context, _, _, _, classifier string) ([]byte, error) {
	f.classifierCalls = append(f.classifierCalls, classifier)
	if f.classifierErr != nil {
		return nil, f.classifierErr
	}
	return f.classifiers[classifier], nil
}

func (f *fakeRegistry) FetchSBOM(_ context.Context, _, _, _ string) ([]byte, []byte, error) {
	return f.sbom, f.asc, nil
}

func (f *fakeRegistry) FetchOSVRecords(_ context.Context, _, _, _ string) ([]OSVRecord, error) {
	return f.osv, nil
}

func (f *fakeRegistry) FetchVEX(_ context.Context, cveIDs []string, _, _, _, serialNumber string) (VEXDocument, error) {
	f.vexSerial = serialNumber
	f.vexCVEs = cveIDs
	return f.vex, nil
}

func (f *fakeRegistry) Ping(_ context.Context) error { return nil }

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{
		jar:  []byte("jar-bytes"),
		pom:  []byte("<project/>"),
		sbom: testSBOMBody,
		asc:  testASCBody,
		vex: VEXDocument{
			vexCandidate: vexCandidate{
				Filename:  "CVE-2025-41249-" + testVEXTimestamp + ".bom.json",
				Timestamp: testVEXTimestamp,
				CVEID:     "CVE-2025-41249",
			},
			Data: testVEXBody,
		},
		osv: []OSVRecord{{CVEID: "CVE-2025-41249"}},
	}
}

type uploadedObject struct {
	data        []byte
	contentType string
}

type fakeS3Uploader struct {
	mu        sync.Mutex
	objects   map[string]uploadedObject
	order     []string
	verified  []string
	uploadErr error
	verifyErr error
}

func newFakeS3Uploader() *fakeS3Uploader {
	return &fakeS3Uploader{objects: map[string]uploadedObject{}}
}

// UploadFile mimics the partner bucket's no-overwrite policy: identical
// content is accepted as already delivered, different content conflicts.
func (f *fakeS3Uploader) UploadFile(_ context.Context, key string, data []byte, contentType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.uploadErr != nil {
		return f.uploadErr
	}
	f.order = append(f.order, key)
	if existing, ok := f.objects[key]; ok {
		if !bytes.Equal(existing.data, data) {
			return &ObjectConflictError{Key: key}
		}
		return nil
	}
	f.objects[key] = uploadedObject{data: data, contentType: contentType}
	return nil
}

func (f *fakeS3Uploader) UploadRootFile(ctx context.Context, key string, data []byte, contentType string) error {
	return f.UploadFile(ctx, key, data, contentType)
}

func (f *fakeS3Uploader) VerifyDelivery(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verified = append(f.verified, key)
	return f.verifyErr
}

func (f *fakeS3Uploader) Ping(_ context.Context) error { return nil }

// fakeJFrogClient records uploads, property updates, and delivery checks.
type fakeJFrogClient struct {
	mu         sync.Mutex
	objects    map[string]uploadedObject
	order      []string
	properties map[string]map[string]string
	verified   []string
	uploadErr  error
}

func newFakeJFrogClient() *fakeJFrogClient {
	return &fakeJFrogClient{objects: map[string]uploadedObject{}, properties: map[string]map[string]string{}}
}

func (f *fakeJFrogClient) UploadFile(_ context.Context, repoPath string, data []byte, contentType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.uploadErr != nil {
		return f.uploadErr
	}
	f.order = append(f.order, repoPath)
	f.objects[repoPath] = uploadedObject{data: data, contentType: contentType}
	return nil
}

func (f *fakeJFrogClient) SetProperties(_ context.Context, repoPath string, props map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.properties[repoPath] = props
	return nil
}

func (f *fakeJFrogClient) VerifyDelivery(_ context.Context, repoPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verified = append(f.verified, repoPath)
	return nil
}

func (f *fakeJFrogClient) Ping(_ context.Context) error { return nil }
