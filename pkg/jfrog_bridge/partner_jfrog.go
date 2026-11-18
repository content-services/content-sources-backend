package jfrog_bridge

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
)

const jfrogPartnerName = "jfrog"

var jfrogPartner = partner{
	name:            jfrogPartnerName,
	displayName:     "JFrog",
	enabled:         func() bool { return config.Get().JFrogBridge.Enabled },
	consumerGroupID: func() string { return config.Get().JFrogBridge.ConsumerGroupID },
	adminFeature:    func() config.Feature { return config.Get().Features.AdminJfrogUpload },
	newHandler: func() (PartnerHandler, error) {
		cfg := loadJFrogConfig()
		return newJFrogHandler(newRegistryClient(cfg.registryConfig), newJFrogClient(cfg), cfg.RegistryURL), nil
	},
}

// jfrogHandler delivers Maven artifacts to the JFrog Catalog Partners
// Artifactory and sets the catalog properties on the JAR.
type jfrogHandler struct {
	registry    RegistryClient
	jfrog       JFrogClient
	registryURL string
}

func newJFrogHandler(registry RegistryClient, jfrog JFrogClient, registryURL string) *jfrogHandler {
	return &jfrogHandler{registry: registry, jfrog: jfrog, registryURL: registryURL}
}

func (h *jfrogHandler) Name() string { return jfrogPartnerName }

// Process runs the full JFrog pipeline for a single remediation.
func (h *jfrogHandler) Process(ctx context.Context, rem Remediation) error {
	// 1a. Fetch JAR from registry
	jarData, jarSHA256, err := h.registry.FetchJAR(ctx, rem.GroupID, rem.ArtifactID, rem.Version)
	if err != nil {
		return fmt.Errorf("fetch JAR: %w", err)
	}

	// 1b. Verify JAR integrity against notification checksum
	jarFilename := fmt.Sprintf("%s-%s.jar", rem.ArtifactID, rem.Version)
	if err := verifyJARChecksum(jarFilename, jarSHA256, rem.Checksums); err != nil {
		return fmt.Errorf("checksum verification: %w", err)
	}

	pomData, err := h.registry.FetchPOM(ctx, rem.GroupID, rem.ArtifactID, rem.Version)
	if err != nil {
		return fmt.Errorf("fetch POM: %w", err)
	}

	// 2. Fetch OSV records
	osvRecords, err := h.registry.FetchOSVRecords(ctx, rem.GroupID, rem.ArtifactID, rem.BaseVersion)
	if err != nil {
		return fmt.Errorf("fetch OSV records: %w", err)
	}

	// 2b. Validate OSV records contain only public CVEs
	if err := ValidateOSVRecords(osvRecords); err != nil {
		return fmt.Errorf("content embargo check: %w", err)
	}

	// 3. Fetch the enriched SBOM and its RADAS signature from Pulp Maven.
	sbomData, sbomASC, err := h.registry.FetchSBOM(ctx, rem.GroupID, rem.ArtifactID, rem.Version)
	if err != nil {
		return fmt.Errorf("fetch SBOM: %w", err)
	}

	// 4. Fetch the VEX whose BOM-Link matches this SBOM.
	serial, err := sbomSerialNumber(sbomData)
	if err != nil {
		return fmt.Errorf("sbom serialNumber: %w", err)
	}
	vexDoc, err := h.registry.FetchVEX(ctx, rem.CVEsFixed, rem.GroupID, rem.ArtifactID, rem.Version, serial)
	if err != nil {
		return fmt.Errorf("fetch VEX: %w", err)
	}

	// 5. Upload bundle to JFrog
	gp := groupPath(rem.GroupID)
	deployBase := fmt.Sprintf("%s/%s/%s", gp, rem.ArtifactID, rem.Version)

	jarPath := fmt.Sprintf("%s/%s-%s.jar", deployBase, rem.ArtifactID, rem.Version)
	if err := h.jfrog.UploadFile(ctx, jarPath, jarData, "application/java-archive"); err != nil {
		return fmt.Errorf("upload JAR: %w", err)
	}

	pomPath := fmt.Sprintf("%s/%s-%s.pom", deployBase, rem.ArtifactID, rem.Version)
	if err := h.jfrog.UploadFile(ctx, pomPath, pomData, "application/xml"); err != nil {
		return fmt.Errorf("upload POM: %w", err)
	}

	sbomPath := fmt.Sprintf("%s/sbom/%s-%s.bom.json", deployBase, rem.ArtifactID, rem.Version)
	if err := h.jfrog.UploadFile(ctx, sbomPath, sbomData, "application/json"); err != nil {
		return fmt.Errorf("upload SBOM: %w", err)
	}
	if err := h.jfrog.UploadFile(ctx, sbomPath+".asc", sbomASC, "application/pgp-signature"); err != nil {
		return fmt.Errorf("upload SBOM signature: %w", err)
	}

	vexPath := fmt.Sprintf("%s/vex/%s-%s.bom.json", deployBase, rem.ArtifactID, rem.Version)
	if err := h.jfrog.UploadFile(ctx, vexPath, vexDoc.Data, "application/json"); err != nil {
		return fmt.Errorf("upload VEX: %w", err)
	}

	// 6. Upload maven-metadata.xml
	metadataXML := generateMavenMetadata(rem.GroupID, rem.ArtifactID, rem.Version)
	metadataPath := fmt.Sprintf("%s/%s/maven-metadata.xml", gp, rem.ArtifactID)
	if err := h.jfrog.UploadFile(ctx, metadataPath, metadataXML, "application/xml"); err != nil {
		return fmt.Errorf("upload maven-metadata.xml: %w", err)
	}

	// 7. Set catalog properties on JAR
	regURL := h.registryURL
	if regURL == "" {
		regURL = loadJFrogConfig().RegistryURL
	}
	props := map[string]string{
		"catalog.name":                   fmt.Sprintf("%s:%s", rem.GroupID, rem.ArtifactID),
		"catalog.version":                rem.Version,
		"catalog.compatible_with":        rem.BaseVersion, // comma-separated if multiple base versions match in the future
		"catalog.vendor_remote_repo_url": regURL,
		"license":                        "Apache-2.0",
	}
	if err := h.jfrog.SetProperties(ctx, jarPath, props); err != nil {
		return fmt.Errorf("set properties: %w", err)
	}

	// 8. Verify
	if err := h.jfrog.VerifyDelivery(ctx, jarPath); err != nil {
		return fmt.Errorf("verify delivery: %w", err)
	}

	return nil
}

func (h *jfrogHandler) Status() map[string]interface{} {
	cfg := config.Get().JFrogBridge
	return map[string]interface{}{
		"enabled":          cfg.Enabled,
		"catalog_url":      cfg.CatalogURL,
		"catalog_repo":     cfg.CatalogRepo,
		"registry_url":     cfg.RegistryURL,
		"registry_osv_url": cfg.RegistryOSVURL,
		"registry_vex_url": cfg.RegistryVEXURL,
	}
}

type jfrogHealthResponse struct {
	Healthy  bool          `json:"healthy"`
	JFrog    serviceHealth `json:"jfrog"`
	Registry serviceHealth `json:"registry"`
}

// Health probes JFrog Artifactory and the Lightwell registry.
func (h *jfrogHandler) Health(ctx context.Context) (bool, interface{}) {
	jfrogErr := h.jfrog.Ping(ctx)
	registryErr := h.registry.Ping(ctx)

	healthy := jfrogErr == nil && registryErr == nil
	return healthy, jfrogHealthResponse{
		Healthy:  healthy,
		JFrog:    newServiceHealth(jfrogErr),
		Registry: newServiceHealth(registryErr),
	}
}

type mavenMetadata struct {
	XMLName    xml.Name        `xml:"metadata"`
	GroupID    string          `xml:"groupId"`
	ArtifactID string          `xml:"artifactId"`
	Versioning mavenVersioning `xml:"versioning"`
}

type mavenVersioning struct {
	Latest      string           `xml:"latest"`
	Release     string           `xml:"release"`
	Versions    mavenVersionList `xml:"versions"`
	LastUpdated string           `xml:"lastUpdated"`
}

type mavenVersionList struct {
	Version []string `xml:"version"`
}

func generateMavenMetadata(groupID, artifactID, version string) []byte {
	ts := time.Now().UTC().Format("20060102150405")
	meta := mavenMetadata{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Versioning: mavenVersioning{
			Latest:  version,
			Release: version,
			Versions: mavenVersionList{
				Version: []string{version},
			},
			LastUpdated: ts,
		},
	}
	data, err := xml.MarshalIndent(meta, "", "  ")
	if err != nil {
		return []byte{}
	}
	return append([]byte(xml.Header), data...)
}
