package jfrog_bridge

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessRemediation_SpringCore(t *testing.T) {
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

	// Registry server: serves JAR, POM from testdata
	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".jar") {
			w.Write(jarData)
		} else if strings.HasSuffix(r.URL.Path, ".pom") {
			w.Write(pomData)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer registryServer.Close()

	// OSV server: serves from testdata/osv/
	osvServer := httptest.NewServer(http.FileServer(http.Dir(testdataPath("osv"))))
	defer osvServer.Close()

	// JFrog server: records all requests
	var mu sync.Mutex
	jfrogPuts := make(map[string]int)
	var jfrogPosts []string
	var capturedProperties string

	testKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case http.MethodPut:
			if strings.Contains(r.URL.RawQuery, "properties=") {
				capturedProperties = r.URL.RawQuery
			}
			jfrogPuts[r.URL.Path]++
			w.WriteHeader(http.StatusCreated)

		case http.MethodPost:
			jfrogPosts = append(jfrogPosts, r.URL.Path)
			if strings.Contains(r.URL.Path, "prepare") {
				resp := prepareResponse{
					DSSEPayload: base64.StdEncoding.EncodeToString([]byte("test-payload")),
					PAE:         "test-pae",
					PostURL:     "/evidence/api/v1/evidence/deploy",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			} else {
				resp := struct {
					Verified bool   `json:"verified"`
					ID       string `json:"id"`
				}{Verified: true, ID: "evd-123"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			}

		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		}
	}))
	defer jfrogServer.Close()

	// Build clients
	registry := &httpRegistryClient{
		httpClient:  registryServer.Client(),
		registryURL: registryServer.URL,
		osvURL:      osvServer.URL,
		maxRetries:  0,
	}

	jfrog := &httpJFrogClient{
		httpClient:  jfrogServer.Client(),
		catalogURL:  jfrogServer.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
		maxRetries:  0,
	}

	evidence := &evidenceCreator{
		httpClient:  jfrogServer.Client(),
		catalogURL:  jfrogServer.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
		privateKey:  testKey,
		keyAlias:    "redhat-partner-lightwell",
		maxRetries:  0,
	}

	metrics := newBridgeMetrics(prometheus.NewRegistry())
	handler := NewBridgeHandler(registry, jfrog, evidence, metrics)

	rem := Remediation{
		GroupID:     "org.springframework",
		ArtifactID: "spring-core",
		Version:    "5.3.18.rhlw-00003",
		BaseVersion: "5.3.18",
		CVEsFixed:  []string{"CVE-2025-41249"},
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

	// Assert: CycloneDX VEX uploaded
	cdxKey := "/artifactory/test-repo/org/springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.cdx.vex.json"
	assert.Equal(t, 1, jfrogPuts[cdxKey], "CycloneDX VEX should be uploaded")

	// Assert: maven-metadata.xml uploaded
	metaKey := "/artifactory/test-repo/org/springframework/spring-core/maven-metadata.xml"
	assert.Equal(t, 1, jfrogPuts[metaKey], "maven-metadata.xml should be uploaded")

	// Assert: Properties set
	assert.Contains(t, capturedProperties, "catalog.name=org.springframework:spring-core")
	assert.Contains(t, capturedProperties, "catalog.compatible_with=5.3.18")

	// Assert: Evidence prepared and deployed
	assert.Contains(t, jfrogPosts, "/evidence/api/v1/evidence/prepare")
	assert.Contains(t, jfrogPosts, "/evidence/api/v1/evidence/deploy")
}

func TestGAVDedup(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := newBridgeMetrics(reg)
	handler := NewBridgeHandler(nil, nil, nil, metrics)

	rem := Remediation{
		GroupID:     "org.test",
		ArtifactID: "test",
		Version:    "1.0.rhlw-00001",
	}

	gav := gavKey(rem)

	_, loaded := handler.processed.Load(gav)
	assert.False(t, loaded)

	handler.processed.Store(gav, true)

	_, loaded = handler.processed.Load(gav)
	assert.True(t, loaded)
}
