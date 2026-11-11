//go:build integration

package jfrog_bridge

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// TestLocalPublish_SpringCore runs the full pipeline against the real JFrog
// Catalog Partners Artifactory using local testdata for the registry
// (JAR, POM, OSV) plus a test SBOM, signature, and VEX document.
//
// Prerequisites:
//   - JFROG_BRIDGE_CATALOG_TOKEN env var set
//   - testdata fixtures in place (JAR, POM, OSV)
//
// Run:
//
//	set -a && source .env.catalog && set +a
//	go test -tags integration ./pkg/jfrog_bridge/ -run TestLocalPublish_SpringCore -v
func TestLocalPublish_SpringCore(t *testing.T) {
	token := os.Getenv("JFROG_BRIDGE_CATALOG_TOKEN")
	if token == "" {
		t.Skip("JFROG_BRIDGE_CATALOG_TOKEN not set; skipping")
	}

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

	registryServer := httptest.NewServer(pipelineRegistryHandler(jarData, pomData, testSBOMBody, testASCBody))
	defer registryServer.Close()

	osvServer := httptest.NewServer(pipelineOSVHandler())
	defer osvServer.Close()

	cfg := bridgeConfig{
		CatalogURL:     "https://jfscatalogpartners.jfrog.io",
		CatalogRepo:    "redhat-partner-maven-lightwell",
		CatalogToken:   token,
		MaxRetries:     2,
		RequestTimeout: 60,
	}

	registry := &httpRegistryClient{
		httpClient:  registryServer.Client(),
		registryURL: registryServer.URL,
		osvURL:      osvServer.URL,
		vexURL:      osvServer.URL,
		maxRetries:  0,
	}

	jfrog := newJFrogClient(cfg)
	metrics := newBridgeMetrics(prometheus.NewRegistry())
	handler := NewBridgeHandler(registry, jfrog, metrics)

	rem := Remediation{
		GroupID:     "org.springframework",
		ArtifactID:  "spring-core",
		Version:     "5.3.18.rhlw-00003",
		BaseVersion: "5.3.18",
		CVEsFixed:   []string{"CVE-2025-41249"},
	}

	t.Log("pipeline: fetch(local testdata) -> SBOM+VEX -> upload(JFrog) -> verify")
	err = handler.processRemediation(context.Background(), rem)
	require.NoError(t, err)

	t.Log("pipeline completed")
	t.Log("verify at: https://jfscatalogpartners.jfrog.io/ui/repos/tree/General/redhat-partner-maven-lightwell/org/springframework/spring-core/5.3.18.rhlw-00003")
}

// TestLivePublish_SpringCore runs the full end-to-end pipeline: fetches
// JAR, POM, SBOM, SBOM signature, OSV records, and VEX from the real
// Lightwell registry on packages.redhat.com, then uploads to real JFrog.
//
// Prerequisites:
//   - JFROG_BRIDGE_CATALOG_TOKEN env var set
//   - clients.lightwell.username and password set in config.yaml
//   - the remediated Maven repository contains the SBOM and its .asc
//   - the OSV file repository contains the matching multi-CVE VEX
//
// Run:
//
//	set -a && source .env.catalog && set +a
//	go test -tags integration ./pkg/jfrog_bridge/ -run TestLivePublish_SpringCore -v
func TestLivePublish_SpringCore(t *testing.T) {
	token := os.Getenv("JFROG_BRIDGE_CATALOG_TOKEN")
	if token == "" {
		t.Skip("JFROG_BRIDGE_CATALOG_TOKEN not set; skipping")
	}

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
		RegistryVEXURL:   config.Get().JFrogBridge.RegistryVEXURL,
		MaxRetries:       2,
		RequestTimeout:   60,
		RegistryUsername: lwCfg.Username,
		RegistryPassword: lwCfg.Password,
	}

	registry := newRegistryClient(cfg)
	jfrog := newJFrogClient(cfg)
	metrics := newBridgeMetrics(prometheus.NewRegistry())
	handler := NewBridgeHandler(registry, jfrog, metrics)

	rem := Remediation{
		GroupID:     "org.springframework",
		ArtifactID:  "spring-core",
		Version:     "5.3.18.rhlw-00003",
		BaseVersion: "5.3.18",
		CVEsFixed:   []string{"CVE-2025-41249"},
	}

	t.Log("pipeline: fetch(packages.redhat.com) -> SBOM+VEX -> upload(JFrog) -> verify")
	err := handler.processRemediation(context.Background(), rem)
	require.NoError(t, err)

	t.Log("pipeline completed")
	t.Log("verify at: https://jfscatalogpartners.jfrog.io/ui/repos/tree/General/redhat-partner-maven-lightwell/org/springframework/spring-core/5.3.18.rhlw-00003")
}
