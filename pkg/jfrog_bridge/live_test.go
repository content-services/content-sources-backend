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

// TestLivePublish_SpringCore runs the full pipeline against the real JFrog
// Catalog Partners Artifactory. It uses local testdata for the registry
// (JAR, POM, OSV) but performs a real upload and evidence signing.
//
// Prerequisites:
//   - JFROG_BRIDGE_CATALOG_TOKEN env var set
//   - pkg/jfrog_bridge/lightwell-catalog.key present
//   - testdata fixtures in place
//
// Run:
//
//	set -a && source .env.catalog && set +a
//	go test ./pkg/jfrog_bridge/ -run TestLivePublish_SpringCore -v
func TestLivePublish_SpringCore(t *testing.T) {
	token := os.Getenv("JFROG_BRIDGE_CATALOG_TOKEN")
	if token == "" {
		t.Skip("JFROG_BRIDGE_CATALOG_TOKEN not set; skipping live test")
	}

	keyPath := "lightwell-catalog.key"
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Skip("lightwell-catalog.key not found; skipping live test")
	}
	keyData, err := os.ReadFile(keyPath)
	require.NoError(t, err)

	jarPath := testdataPath("registry", "org", "springframework",
		"spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.jar")
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		t.Skip("JAR fixture not found; skipping live test")
	}
	jarData, err := os.ReadFile(jarPath)
	require.NoError(t, err)

	pomData, err := os.ReadFile(testdataPath("registry", "org", "springframework",
		"spring-core", "5.3.18.rhlw-00003", "spring-core-5.3.18.rhlw-00003.pom"))
	require.NoError(t, err)

	// Local registry server serves JAR and POM from testdata
	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".jar"):
			w.Write(jarData)
		case strings.HasSuffix(r.URL.Path, ".pom"):
			w.Write(pomData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer registryServer.Close()

	// Local OSV server serves from testdata/osv/
	osvServer := httptest.NewServer(http.FileServer(http.Dir(testdataPath("osv"))))
	defer osvServer.Close()

	// Real JFrog client
	cfg := bridgeConfig{
		CatalogURL:  "https://jfscatalogpartners.jfrog.io",
		CatalogRepo: "redhat-partner-maven-lightwell",
		CatalogToken: token,
		RegistryURL:  config.Get().JFrogBridge.RegistryURL,
		SigningKeyPEM: string(keyData),
		SigningKeyAlias: "redhat-partner-lightwell",
		MaxRetries:   2,
		RequestTimeout: 60,
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
		ArtifactID: "spring-core",
		Version:    "5.3.18.rhlw-00003",
		BaseVersion: "5.3.18",
		CVEsFixed:  []string{"CVE-2025-41249"},
	}

	t.Log("starting live pipeline: fetch(local) -> VEX -> upload(JFrog) -> sign -> verify")
	err = handler.processRemediation(context.Background(), rem)
	require.NoError(t, err)

	t.Log("pipeline completed successfully")
	t.Log("verify at: https://jfscatalogpartners.jfrog.io/ui/repos/tree/General/redhat-partner-maven-lightwell/org/springframework/spring-core/5.3.18.rhlw-00003")
}
