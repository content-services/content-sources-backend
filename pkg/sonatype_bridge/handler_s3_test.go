package sonatype_bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRegistry struct {
	jar, pom, sbom, asc, vex []byte
	osv                      []OSVRecord
}

func (f *fakeRegistry) FetchJAR(_ context.Context, _, _, _ string) ([]byte, string, error) {
	sum := sha256.Sum256(f.jar)
	return f.jar, hex.EncodeToString(sum[:]), nil
}

func (f *fakeRegistry) FetchPOM(_ context.Context, _, _, _ string) ([]byte, error) {
	return f.pom, nil
}

func (f *fakeRegistry) FetchSBOM(_ context.Context, _, _, _ string) ([]byte, []byte, error) {
	return f.sbom, f.asc, nil
}

func (f *fakeRegistry) FetchOSVRecords(_ context.Context, _, _, _ string) ([]OSVRecord, error) {
	return f.osv, nil
}

func (f *fakeRegistry) FetchVEX(_ context.Context, _ []string) ([]byte, error) {
	return f.vex, nil
}

func (f *fakeRegistry) Ping(_ context.Context) error { return nil }

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

func (f *fakeS3Uploader) UploadFile(_ context.Context, key string, data []byte, contentType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.uploadErr != nil {
		return f.uploadErr
	}
	f.objects[key] = uploadedObject{data: data, contentType: contentType}
	f.order = append(f.order, key)
	return nil
}

func (f *fakeS3Uploader) VerifyDelivery(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verified = append(f.verified, key)
	return f.verifyErr
}

func (f *fakeS3Uploader) Ping(_ context.Context) error { return nil }

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{
		jar:  []byte("jar-bytes"),
		pom:  []byte("<project/>"),
		sbom: testSBOMBody,
		asc:  testASCBody,
		vex:  testVEXBody,
		osv:  []OSVRecord{{CVEID: "CVE-2025-41249"}},
	}
}

func TestProcessRemediation_UploadsBundleToS3(t *testing.T) {
	registry := newFakeRegistry()
	uploader := newFakeS3Uploader()
	handler := NewBridgeHandler(registry, uploader, newBridgeMetrics(prometheus.NewRegistry()))

	require.NoError(t, handler.processRemediation(context.Background(), springRemediation()))

	base := "org/springframework/spring-core/5.3.18.rhlw-00003"
	expected := map[string]uploadedObject{
		base + "/spring-core-5.3.18.rhlw-00003.jar":               {registry.jar, "application/java-archive"},
		base + "/spring-core-5.3.18.rhlw-00003.pom":               {registry.pom, "application/xml"},
		base + "/sbom/spring-core-5.3.18.rhlw-00003.bom.json":     {testSBOMBody, "application/json"},
		base + "/sbom/spring-core-5.3.18.rhlw-00003.bom.json.asc": {testASCBody, "application/pgp-signature"},
		base + "/vex/spring-core-5.3.18.rhlw-00003.bom.json":      {testVEXBody, "application/json"},
	}
	for key, want := range expected {
		got, ok := uploader.objects[key]
		require.True(t, ok, "expected %s to be uploaded", key)
		assert.Equal(t, want.data, got.data, key)
		assert.Equal(t, want.contentType, got.contentType, key)
	}

	meta, ok := uploader.objects["org/springframework/spring-core/maven-metadata.xml"]
	require.True(t, ok, "maven-metadata.xml should be uploaded")
	assert.Equal(t, "application/xml", meta.contentType)
	assert.Contains(t, string(meta.data), "<version>5.3.18.rhlw-00003</version>")

	assert.Len(t, uploader.objects, 6)
	assert.Equal(t, []string{base + "/spring-core-5.3.18.rhlw-00003.jar"}, uploader.verified)
}

func TestProcessRemediation_S3UploadError(t *testing.T) {
	uploader := newFakeS3Uploader()
	uploader.uploadErr = errors.New("access denied")
	handler := NewBridgeHandler(newFakeRegistry(), uploader, newBridgeMetrics(prometheus.NewRegistry()))

	err := handler.processRemediation(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload JAR")
	assert.Empty(t, uploader.verified)
}

func TestProcessRemediation_S3VerifyError(t *testing.T) {
	uploader := newFakeS3Uploader()
	uploader.verifyErr = errors.New("not found")
	handler := NewBridgeHandler(newFakeRegistry(), uploader, newBridgeMetrics(prometheus.NewRegistry()))

	err := handler.processRemediation(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify delivery")
}

func TestProcessRemediation_EmbargoedOSVNotUploaded(t *testing.T) {
	registry := newFakeRegistry()
	registry.osv = []OSVRecord{{CVEID: "LTWL-0001"}}
	uploader := newFakeS3Uploader()
	handler := NewBridgeHandler(registry, uploader, newBridgeMetrics(prometheus.NewRegistry()))

	err := handler.processRemediation(context.Background(), springRemediation())
	require.Error(t, err)
	var embargoErr *EmbargoError
	assert.True(t, errors.As(err, &embargoErr))
	assert.Empty(t, uploader.objects, "nothing may reach the partner bucket on embargo")
}

func TestProcessRemediation_ChecksumMismatchNotUploaded(t *testing.T) {
	uploader := newFakeS3Uploader()
	handler := NewBridgeHandler(newFakeRegistry(), uploader, newBridgeMetrics(prometheus.NewRegistry()))

	rem := springRemediation()
	rem.Checksums = map[string]string{"spring-core-5.3.18.rhlw-00003.jar": "deadbeef"}
	err := handler.processRemediation(context.Background(), rem)
	require.Error(t, err)
	var checksumErr *ChecksumMismatchError
	assert.True(t, errors.As(err, &checksumErr))
	assert.Empty(t, uploader.objects)
}

func TestProcessRemediation_WithFilePrefixOverHTTP(t *testing.T) {
	server, reqs, mu := newRecordingS3Server(200)
	defer server.Close()

	handler := NewBridgeHandler(newFakeRegistry(), newTestS3Uploader(server, "maven2"), newBridgeMetrics(prometheus.NewRegistry()))
	require.NoError(t, handler.processRemediation(context.Background(), springRemediation()))

	mu.Lock()
	defer mu.Unlock()
	paths := map[string]string{}
	for _, r := range *reqs {
		paths[r.path] = r.method
	}
	jar := "/" + testS3Bucket + "/maven2/org/springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.jar"
	assert.Contains(t, paths, jar)
	assert.Contains(t, paths, "/"+testS3Bucket+"/maven2/org/springframework/spring-core/maven-metadata.xml")
	// 6 PUTs + 1 HEAD (verify on the JAR)
	assert.Len(t, *reqs, 7)
	last := (*reqs)[len(*reqs)-1]
	assert.Equal(t, "HEAD", last.method)
	assert.Equal(t, jar, last.path)
}
