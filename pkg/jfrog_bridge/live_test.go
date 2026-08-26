package jfrog_bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// TestLocalPublish_SpringCore runs the full pipeline against the real JFrog
// Catalog Partners Artifactory using local testdata for the registry
// (JAR, POM, OSV). Real upload + evidence signing against JFrog.
//
// Prerequisites:
//   - JFROG_BRIDGE_CATALOG_TOKEN env var set
//   - lightwell-catalog.key present
//   - testdata fixtures in place (JAR, POM, OSV)
//
// Run:
//
//	set -a && source .env.catalog && set +a
//	go test ./pkg/jfrog_bridge/ -run TestLocalPublish_SpringCore -v
func TestLocalPublish_SpringCore(t *testing.T) {
	token := os.Getenv("JFROG_BRIDGE_CATALOG_TOKEN")
	if token == "" {
		t.Skip("JFROG_BRIDGE_CATALOG_TOKEN not set; skipping")
	}

	keyPath := "lightwell-catalog.key"
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Skip("lightwell-catalog.key not found; skipping")
	}
	keyData, err := os.ReadFile(keyPath)
	require.NoError(t, err)

	jarPath := testdataPath("registry", "org", "springframework",
		"spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.jar")
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		t.Skip("JAR fixture not found; skipping")
	}
	jarData, err := os.ReadFile(jarPath)
	require.NoError(t, err)

	pomData, err := os.ReadFile(testdataPath("registry", "org", "springframework",
		"spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.pom"))
	require.NoError(t, err)

	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".jar"):
			_, _ = w.Write(jarData)
		case strings.HasSuffix(r.URL.Path, ".pom"):
			_, _ = w.Write(pomData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer registryServer.Close()

	osvServer := httptest.NewServer(http.FileServer(http.Dir(testdataPath("osv"))))
	defer osvServer.Close()

	cfg := bridgeConfig{
		CatalogURL:      "https://jfscatalogpartners.jfrog.io",
		CatalogRepo:     "redhat-partner-maven-lightwell",
		CatalogToken:    token,
		RegistryURL:     config.Get().JFrogBridge.RegistryURL,
		SigningKeyPEM:   string(keyData),
		SigningKeyAlias: "redhat-partner-lightwell",
		MaxRetries:      2,
		RequestTimeout:  60,
	}

	registry := &httpRegistryClient{
		httpClient:  registryServer.Client(),
		registryURL: registryServer.URL,
		osvURL:      osvServer.URL,
		maxRetries:  0,
	}

	jfrog := newJFrogClient(cfg)
	evidence, err := newEvidenceCreator(cfg)
	require.NoError(t, err)

	metrics := newBridgeMetrics(prometheus.NewRegistry())
	handler := NewBridgeHandler(registry, jfrog, evidence, metrics)

	rem := Remediation{
		GroupID:     "org.springframework",
		ArtifactID:  "spring-core",
		Version:     "5.3.18.rhlw-00003",
		BaseVersion: "5.3.18",
		CVEsFixed:   []string{"CVE-2025-41249"},
	}

	t.Log("pipeline: fetch(local testdata) -> VEX -> upload(JFrog) -> sign -> verify")
	err = handler.processRemediation(context.Background(), rem)
	require.NoError(t, err)

	t.Log("pipeline completed")
	t.Log("verify at: https://jfscatalogpartners.jfrog.io/ui/repos/tree/General/redhat-partner-maven-lightwell/org/springframework/spring-core/5.3.18.rhlw-00003")
}

// TestLivePublish_SpringCore runs the full end-to-end pipeline: fetches
// JAR, POM, and OSV records from the real Lightwell registry on
// packages.redhat.com, then uploads to real JFrog with signed evidence.
//
// Prerequisites:
//   - JFROG_BRIDGE_CATALOG_TOKEN env var set
//   - lightwell-catalog.key present
//   - clients.lightwell.username and password set in config.yaml
//
// Run:
//
//	set -a && source .env.catalog && set +a
//	go test ./pkg/jfrog_bridge/ -run TestLivePublish_SpringCore -v
func TestLivePublish_SpringCore(t *testing.T) {
	token := os.Getenv("JFROG_BRIDGE_CATALOG_TOKEN")
	if token == "" {
		t.Skip("JFROG_BRIDGE_CATALOG_TOKEN not set; skipping")
	}

	keyPath := "lightwell-catalog.key"
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Skip("lightwell-catalog.key not found; skipping")
	}
	keyData, err := os.ReadFile(keyPath)
	require.NoError(t, err)

	lwCfg := config.Get().Clients.Lightwell
	if lwCfg.Username == "" || lwCfg.Password == "" {
		t.Skip("clients.lightwell credentials not set; skipping")
	}

	cfg := bridgeConfig{
		CatalogURL:       "https://jfscatalogpartners.jfrog.io",
		CatalogRepo:      "redhat-partner-maven-lightwell",
		CatalogToken:     token,
		RegistryURL:      config.Get().JFrogBridge.RegistryURL,
		RegistryOSVURL:   config.Get().JFrogBridge.RegistryOSVURL,
		SigningKeyPEM:    string(keyData),
		SigningKeyAlias:  "redhat-partner-lightwell",
		MaxRetries:       2,
		RequestTimeout:   60,
		RegistryUsername: lwCfg.Username,
		RegistryPassword: lwCfg.Password,
	}

	registry := newRegistryClient(cfg)
	jfrog := newJFrogClient(cfg)
	evidence, err := newEvidenceCreator(cfg)
	require.NoError(t, err)

	metrics := newBridgeMetrics(prometheus.NewRegistry())
	handler := NewBridgeHandler(registry, jfrog, evidence, metrics)

	rem := Remediation{
		GroupID:     "org.springframework",
		ArtifactID:  "spring-core",
		Version:     "5.3.18.rhlw-00003",
		BaseVersion: "5.3.18",
		CVEsFixed:   []string{"CVE-2025-41249"},
	}

	t.Log("pipeline: fetch(packages.redhat.com) -> VEX -> upload(JFrog) -> sign -> verify")
	err = handler.processRemediation(context.Background(), rem)
	require.NoError(t, err)

	t.Log("pipeline completed")
	t.Log("verify at: https://jfscatalogpartners.jfrog.io/ui/repos/tree/General/redhat-partner-maven-lightwell/org/springframework/spring-core/5.3.18.rhlw-00003")
}
