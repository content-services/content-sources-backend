package sonatype_bridge

import (
	"context"
	"crypto/md5" //nolint:gosec // ETag comparison in tests
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testS3Bucket = "test-bucket"

// newTestS3Uploader returns an uploader that talks to a fake S3 endpoint
// using path-style addressing (/<bucket>/<key>).
func newTestS3Uploader(server *httptest.Server, prefix string) *awsS3Uploader {
	client := s3.New(s3.Options{
		Region:                     "us-east-1",
		Credentials:                credentials.NewStaticCredentialsProvider("test-ak", "test-sk", ""),
		BaseEndpoint:               aws.String(server.URL),
		UsePathStyle:               true,
		HTTPClient:                 server.Client(),
		Retryer:                    aws.NopRetryer{},
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
	})
	return &awsS3Uploader{
		client: client,
		bucket: testS3Bucket,
		prefix: strings.Trim(prefix, "/"),
	}
}

type capturedRequest struct {
	method      string
	path        string
	contentType string
	auth        string
	body        []byte
}

func newRecordingS3Server(status int) (*httptest.Server, *[]capturedRequest, *sync.Mutex) {
	var mu sync.Mutex
	var reqs []capturedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		reqs = append(reqs, capturedRequest{
			method:      r.Method,
			path:        r.URL.Path,
			contentType: r.Header.Get("Content-Type"),
			auth:        r.Header.Get("Authorization"),
			body:        body,
		})
		mu.Unlock()
		w.WriteHeader(status)
	}))
	return server, &reqs, &mu
}

func TestS3Uploader_ObjectKey(t *testing.T) {
	cases := []struct {
		prefix string
		key    string
		want   string
	}{
		{"", "org/test/a.jar", "org/test/a.jar"},
		{"", "/org/test/a.jar", "org/test/a.jar"},
		{"maven2", "org/test/a.jar", "maven2/org/test/a.jar"},
		{"/maven2/", "org/test/a.jar", "maven2/org/test/a.jar"},
		{"a/b", "org/test/a.jar", "a/b/org/test/a.jar"},
	}
	for _, tc := range cases {
		u := &awsS3Uploader{prefix: strings.Trim(tc.prefix, "/")}
		assert.Equal(t, tc.want, u.objectKey(tc.key), "prefix=%q key=%q", tc.prefix, tc.key)
	}
}

func TestS3Uploader_UploadFile(t *testing.T) {
	server, reqs, mu := newRecordingS3Server(http.StatusOK)
	defer server.Close()

	u := newTestS3Uploader(server, "maven2")
	err := u.UploadFile(context.Background(), "org/test/1.0/test-1.0.jar", []byte("jar-bytes"), "application/java-archive")
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, *reqs, 1)
	r := (*reqs)[0]
	assert.Equal(t, http.MethodPut, r.method)
	assert.Equal(t, "/"+testS3Bucket+"/maven2/org/test/1.0/test-1.0.jar", r.path)
	assert.Equal(t, "application/java-archive", r.contentType)
	assert.Equal(t, []byte("jar-bytes"), r.body)
}

func TestS3Uploader_UploadFileRetries(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u := newTestS3Uploader(server, "")
	u.maxRetries = 1
	require.NoError(t, u.UploadFile(context.Background(), "a.pom", []byte("pom"), "application/xml"))
	assert.Equal(t, 2, attempts)
}

func TestS3Uploader_UploadFileError(t *testing.T) {
	server, _, _ := newRecordingS3Server(http.StatusForbidden)
	defer server.Close()

	u := newTestS3Uploader(server, "")
	err := u.UploadFile(context.Background(), "org/test/a.jar", []byte("x"), "application/java-archive")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload org/test/a.jar")
}

func TestS3Uploader_VerifyDelivery(t *testing.T) {
	server, reqs, mu := newRecordingS3Server(http.StatusOK)
	defer server.Close()

	u := newTestS3Uploader(server, "maven2")
	require.NoError(t, u.VerifyDelivery(context.Background(), "org/test/a.jar"))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, *reqs, 1)
	assert.Equal(t, http.MethodHead, (*reqs)[0].method)
	assert.Equal(t, "/"+testS3Bucket+"/maven2/org/test/a.jar", (*reqs)[0].path)
}

func TestS3Uploader_VerifyDeliveryNotFound(t *testing.T) {
	server, _, _ := newRecordingS3Server(http.StatusNotFound)
	defer server.Close()

	u := newTestS3Uploader(server, "")
	err := u.VerifyDelivery(context.Background(), "org/test/a.jar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify delivery")
}

func TestS3Uploader_Ping(t *testing.T) {
	server, reqs, mu := newRecordingS3Server(http.StatusOK)
	defer server.Close()

	u := newTestS3Uploader(server, "")
	require.NoError(t, u.Ping(context.Background()))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, *reqs, 1)
	assert.Equal(t, http.MethodHead, (*reqs)[0].method)
	assert.Equal(t, "/"+testS3Bucket, (*reqs)[0].path)
}

func TestNewS3Uploader_IgnoresAmbientAWSEnv(t *testing.T) {
	server, reqs, mu := newRecordingS3Server(http.StatusOK)
	defer server.Close()

	// Ambient AWS settings must not redirect the partner client.
	t.Setenv("AWS_ENDPOINT_URL", "http://127.0.0.1:1")
	t.Setenv("AWS_ENDPOINT_URL_S3", "http://127.0.0.1:1")
	t.Setenv("AWS_ACCESS_KEY_ID", "ambient-ak")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-sk")
	t.Setenv("AWS_REGION", "eu-west-1")

	u, err := newS3Uploader(bridgeConfig{
		S3URL:          server.URL,
		S3Bucket:       "partner-bucket",
		S3Region:       "us-east-1",
		S3AccessKey:    "partner-ak",
		S3SecretKey:    "partner-sk",
		S3UsePathStyle: true,
		RequestTimeout: 5,
	})
	require.NoError(t, err)
	require.NoError(t, u.Ping(context.Background()))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, *reqs, 1)
	assert.Equal(t, "/partner-bucket", (*reqs)[0].path)
	assert.Contains(t, (*reqs)[0].auth, "Credential=partner-ak/")
	assert.Contains(t, (*reqs)[0].auth, "/us-east-1/s3/")
	assert.NotContains(t, (*reqs)[0].auth, "ambient-ak")
}

func TestS3Uploader_UploadFileIsConditional(t *testing.T) {
	var mu sync.Mutex
	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		headers = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u := newTestS3Uploader(server, "")
	data := []byte("payload")
	require.NoError(t, u.UploadFile(context.Background(), "a.json", data, "application/json"))

	sum := sha256.Sum256(data)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "*", headers.Get("If-None-Match"))
	assert.Equal(t, base64.StdEncoding.EncodeToString(sum[:]), headers.Get("X-Amz-Checksum-Sha256"))
}

func TestS3Uploader_UploadRootFileIgnoresPrefix(t *testing.T) {
	server, reqs, mu := newRecordingS3Server(http.StatusOK)
	defer server.Close()

	u := newTestS3Uploader(server, "maven2")
	require.NoError(t, u.UploadRootFile(context.Background(), "vex/CVE-2025-1-2026-07-15.bom.json", []byte("{}"), "application/json"))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, *reqs, 1)
	assert.Equal(t, http.MethodPut, (*reqs)[0].method)
	assert.Equal(t, "/"+testS3Bucket+"/vex/CVE-2025-1-2026-07-15.bom.json", (*reqs)[0].path)
}

// newExistingObjectServer answers PUT with 412 and HEAD with the given headers.
func newExistingObjectServer(headHeaders map[string]string) (*httptest.Server, *[]string, *sync.Mutex) {
	var mu sync.Mutex
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method)
		mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = w.Write([]byte(`<Error><Code>PreconditionFailed</Code><Message>At least one of the pre-conditions you specified did not hold</Message></Error>`))
		case http.MethodHead:
			for k, v := range headHeaders {
				w.Header().Set(k, v)
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	return server, &methods, &mu
}

func TestS3Uploader_ExistingIdenticalChecksumSkips(t *testing.T) {
	data := []byte("payload")
	sum := sha256.Sum256(data)
	server, methods, mu := newExistingObjectServer(map[string]string{
		"x-amz-checksum-sha256": base64.StdEncoding.EncodeToString(sum[:]),
	})
	defer server.Close()

	u := newTestS3Uploader(server, "")
	u.maxRetries = 2
	require.NoError(t, u.UploadFile(context.Background(), "a.json", data, "application/json"))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{http.MethodPut, http.MethodHead}, *methods, "412 must not be retried")
}

func TestS3Uploader_ExistingDifferentChecksumConflicts(t *testing.T) {
	other := sha256.Sum256([]byte("other"))
	server, _, _ := newExistingObjectServer(map[string]string{
		"x-amz-checksum-sha256": base64.StdEncoding.EncodeToString(other[:]),
	})
	defer server.Close()

	u := newTestS3Uploader(server, "maven2")
	err := u.UploadFile(context.Background(), "a.json", []byte("payload"), "application/json")
	require.Error(t, err)
	var conflictErr *ObjectConflictError
	require.True(t, errors.As(err, &conflictErr))
	assert.Equal(t, "maven2/a.json", conflictErr.Key)
}

func TestS3Uploader_ExistingMatchingETagSkips(t *testing.T) {
	data := []byte("payload")
	sum := md5.Sum(data) //nolint:gosec // test
	server, _, _ := newExistingObjectServer(map[string]string{
		"ETag": `"` + hex.EncodeToString(sum[:]) + `"`,
	})
	defer server.Close()

	u := newTestS3Uploader(server, "")
	require.NoError(t, u.UploadFile(context.Background(), "a.json", data, "application/json"))
}

func TestS3Uploader_ExistingUnverifiableConflicts(t *testing.T) {
	server, _, _ := newExistingObjectServer(map[string]string{
		"ETag": `"d41d8cd98f00b204e9800998ecf8427e-2"`,
	})
	defer server.Close()

	u := newTestS3Uploader(server, "")
	err := u.UploadFile(context.Background(), "a.json", []byte("payload"), "application/json")
	var conflictErr *ObjectConflictError
	require.True(t, errors.As(err, &conflictErr))
}

func TestS3Uploader_ConditionalConflictRetried(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		if attempts == 1 {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`<Error><Code>ConditionalRequestConflict</Code></Error>`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u := newTestS3Uploader(server, "")
	u.maxRetries = 1
	require.NoError(t, u.UploadFile(context.Background(), "a.json", []byte("x"), "application/json"))
	assert.Equal(t, 2, attempts)
}
