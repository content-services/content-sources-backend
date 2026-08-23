package jfrog_bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJFrogClient_UploadFile(t *testing.T) {
	var mu sync.Mutex
	received := make(map[string]string)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received[r.URL.Path] = r.Method
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := &httpJFrogClient{
		httpClient:  server.Client(),
		catalogURL:  server.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
		maxRetries:  0,
	}

	err := client.UploadFile(context.Background(),
		"org/springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.jar",
		[]byte("fake-jar"), "application/java-archive")
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "PUT",
		received["/artifactory/test-repo/org/springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.jar"])
}

func TestJFrogClient_SetProperties(t *testing.T) {
	var capturedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RequestURI()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := &httpJFrogClient{
		httpClient:  server.Client(),
		catalogURL:  server.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
	}

	err := client.SetProperties(context.Background(),
		"path/to/file.jar",
		map[string]string{"catalog.name": "test:pkg"})
	require.NoError(t, err)
	assert.Contains(t, capturedPath, "catalog.name=test%3Apkg")
}

func TestJFrogClient_VerifyDelivery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"path": "test"}`))
	}))
	defer server.Close()

	client := &httpJFrogClient{
		httpClient:  server.Client(),
		catalogURL:  server.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
	}

	err := client.VerifyDelivery(context.Background(), "path/to/file.jar")
	require.NoError(t, err)
}

func TestJFrogClient_Ping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	client := &httpJFrogClient{
		httpClient:  server.Client(),
		catalogURL:  server.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
	}

	err := client.Ping(context.Background())
	require.NoError(t, err)
}

func TestJFrogClient_UploadFile_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &httpJFrogClient{
		httpClient:  server.Client(),
		catalogURL:  server.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
		maxRetries:  0,
	}

	err := client.UploadFile(context.Background(), "path/to/file.jar", []byte("data"), "application/java-archive")
	require.Error(t, err)
}

func TestJFrogClient_AuthHeader(t *testing.T) {
	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := &httpJFrogClient{
		httpClient:  server.Client(),
		catalogURL:  server.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
		maxRetries:  0,
	}

	err := client.UploadFile(context.Background(), "path/to/file.jar", []byte("data"), "application/java-archive")
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-token", capturedAuth)
}

func TestJFrogClient_SetProperties_SpecialChars(t *testing.T) {
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := &httpJFrogClient{
		httpClient:  server.Client(),
		catalogURL:  server.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
	}

	props := map[string]string{
		"catalog.vendor_remote_repo_url": "https://packages.redhat.com/lightwell/java/remediated",
	}

	err := client.SetProperties(context.Background(), "path/to/file.jar", props)
	require.NoError(t, err)

	decoded, err := url.QueryUnescape(capturedQuery)
	require.NoError(t, err)
	assert.Contains(t, decoded, "catalog.vendor_remote_repo_url=https://packages.redhat.com/lightwell/java/remediated")
}
