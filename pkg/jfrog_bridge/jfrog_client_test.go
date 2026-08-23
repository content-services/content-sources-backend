package jfrog_bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	assert.Contains(t, capturedPath, "catalog.name=test:pkg")
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
