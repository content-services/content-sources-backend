package sonatype_bridge

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRegistry struct {
	jar, pom, sbom, asc, vex []byte
	osv                      []OSVRecord
	classifiers              map[string][]byte
	classifierErr            error
}

func (f *fakeRegistry) FetchJAR(_ context.Context, _, _, _ string) ([]byte, string, error) {
	sum := sha256.Sum256(f.jar)
	return f.jar, hex.EncodeToString(sum[:]), nil
}

func (f *fakeRegistry) FetchPOM(_ context.Context, _, _, _ string) ([]byte, error) {
	return f.pom, nil
}

func (f *fakeRegistry) FetchClassifier(_ context.Context, _, _, _, classifier string) ([]byte, error) {
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

	base := springKeyBase
	zipKey := base + ".zip"
	assert.Equal(t, []string{
		zipKey,
		base + ".bom.json.asc",
		base + ".vex.json",
		base + ".bom.json",
	}, uploader.order, "SBOM must be uploaded last")

	expected := map[string]uploadedObject{
		base + ".bom.json":     {testSBOMBody, "application/json"},
		base + ".bom.json.asc": {testASCBody, "application/pgp-signature"},
		base + ".vex.json":     {testVEXBody, "application/json"},
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

func TestProcessRemediation_BundleIncludesSourcesAndJavadoc(t *testing.T) {
	registry := newFakeRegistry()
	registry.classifiers = map[string][]byte{
		"sources": []byte("sources-bytes"),
		"javadoc": []byte("javadoc-bytes"),
	}
	uploader := newFakeS3Uploader()
	handler := NewBridgeHandler(registry, uploader, newBridgeMetrics(prometheus.NewRegistry()))

	require.NoError(t, handler.processRemediation(context.Background(), springRemediation()))

	assert.Equal(t, map[string][]byte{
		"spring-core-5.3.18.rhlw-00003.jar":         registry.jar,
		"spring-core-5.3.18.rhlw-00003.pom":         registry.pom,
		"spring-core-5.3.18.rhlw-00003-sources.jar": []byte("sources-bytes"),
		"spring-core-5.3.18.rhlw-00003-javadoc.jar": []byte("javadoc-bytes"),
	}, readZip(t, uploader.objects[springKeyBase+".zip"].data))
}

func TestProcessRemediation_ClassifierFetchError(t *testing.T) {
	registry := newFakeRegistry()
	registry.classifierErr = errors.New("HTTP 500")
	uploader := newFakeS3Uploader()
	handler := NewBridgeHandler(registry, uploader, newBridgeMetrics(prometheus.NewRegistry()))

	err := handler.processRemediation(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sources JAR")
	assert.Empty(t, uploader.objects)
}

func TestProcessRemediation_EmptyGroupOmitsNamespace(t *testing.T) {
	registry := newFakeRegistry()
	registry.sbom = []byte(`{"components":[{"name":"spring-core","version":"5.3.18.rhlw-00003","purl":"pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00003"}]}`)
	uploader := newFakeS3Uploader()
	handler := NewBridgeHandler(registry, uploader, newBridgeMetrics(prometheus.NewRegistry()))

	require.NoError(t, handler.processRemediation(context.Background(), springRemediation()))
	assert.Contains(t, uploader.objects, "packages/maven/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.zip")
}

func TestProcessRemediation_SBOMMismatchNotUploaded(t *testing.T) {
	tests := map[string]string{
		"name":    `{"components":[{"group":"org.springframework","name":"spring-beans","version":"5.3.18.rhlw-00003","purl":"pkg:maven/org.springframework/spring-beans@5.3.18.rhlw-00003"}]}`,
		"version": `{"components":[{"group":"org.springframework","name":"spring-core","version":"5.3.18.rhlw-00004","purl":"pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00004"}]}`,
		"group":   `{"components":[{"group":"org.other","name":"spring-core","version":"5.3.18.rhlw-00003","purl":"pkg:maven/org.other/spring-core@5.3.18.rhlw-00003"}]}`,
		"missing": `{"bomFormat":"CycloneDX"}`,
	}
	for name, sbom := range tests {
		t.Run(name, func(t *testing.T) {
			registry := newFakeRegistry()
			registry.sbom = []byte(sbom)
			uploader := newFakeS3Uploader()
			handler := NewBridgeHandler(registry, uploader, newBridgeMetrics(prometheus.NewRegistry()))

			err := handler.processRemediation(context.Background(), springRemediation())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "SBOM coordinates")
			assert.Empty(t, uploader.objects)
		})
	}
}

func TestProcessRemediation_S3UploadError(t *testing.T) {
	uploader := newFakeS3Uploader()
	uploader.uploadErr = errors.New("access denied")
	handler := NewBridgeHandler(newFakeRegistry(), uploader, newBridgeMetrics(prometheus.NewRegistry()))

	err := handler.processRemediation(context.Background(), springRemediation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload bundle")
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
	zip := "/" + testS3Bucket + "/maven2/" + springKeyBase + ".zip"
	assert.Contains(t, paths, zip)
	assert.Contains(t, paths, "/"+testS3Bucket+"/maven2/"+springKeyBase+".bom.json")
	// 4 PUTs + 1 HEAD (verify on the zip)
	assert.Len(t, *reqs, 5)
	last := (*reqs)[len(*reqs)-1]
	assert.Equal(t, "HEAD", last.method)
	assert.Equal(t, zip, last.path)
}

func TestProcessRemediation_NoPrefixKeysStartWithPackages(t *testing.T) {
	server, reqs, mu := newRecordingS3Server(200)
	defer server.Close()

	handler := NewBridgeHandler(newFakeRegistry(), newTestS3Uploader(server, ""), newBridgeMetrics(prometheus.NewRegistry()))
	require.NoError(t, handler.processRemediation(context.Background(), springRemediation()))

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, *reqs)
	for _, r := range *reqs {
		assert.True(t, strings.HasPrefix(r.path, "/"+testS3Bucket+"/packages/"), r.path)
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
