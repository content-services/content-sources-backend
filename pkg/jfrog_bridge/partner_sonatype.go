package jfrog_bridge

import (
	"context"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
)

const (
	sonatypePartnerName = "sonatype"
	vexDir              = "vex"
)

// optionalClassifiers are classified JARs added to the bundle when published.
var optionalClassifiers = []string{"sources", "javadoc"}

var sonatypePartner = partner{
	name:            sonatypePartnerName,
	displayName:     "Sonatype",
	enabled:         func() bool { return config.Get().SonatypeBridge.Enabled },
	consumerGroupID: func() string { return config.Get().SonatypeBridge.ConsumerGroupID },
	adminFeature:    func() config.Feature { return config.Get().Features.AdminSonatypeUpload },
	newHandler: func() (PartnerHandler, error) {
		cfg := loadSonatypeConfig()
		uploader, err := newS3Uploader(cfg)
		if err != nil {
			return nil, err
		}
		return newSonatypeHandler(newRegistryClient(cfg.registryConfig), uploader), nil
	},
}

// sonatypeHandler delivers a flat zip bundle, the SBOM, its signature, and
// the VEX document to the partner-owned S3 bucket.
type sonatypeHandler struct {
	registry RegistryClient
	s3       S3Uploader
}

func newSonatypeHandler(registry RegistryClient, s3 S3Uploader) *sonatypeHandler {
	return &sonatypeHandler{registry: registry, s3: s3}
}

func (h *sonatypeHandler) Name() string { return sonatypePartnerName }

// Process runs the full Sonatype pipeline for a single remediation.
func (h *sonatypeHandler) Process(ctx context.Context, rem Remediation) error {
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

	bundleFiles := []bundleFile{
		{Name: jarFilename, Data: jarData},
		{Name: fmt.Sprintf("%s-%s.pom", rem.ArtifactID, rem.Version), Data: pomData},
	}
	for _, classifier := range optionalClassifiers {
		data, err := h.registry.FetchClassifier(ctx, rem.GroupID, rem.ArtifactID, rem.Version, classifier)
		if err != nil {
			return fmt.Errorf("fetch %s JAR: %w", classifier, err)
		}
		if data == nil {
			continue
		}
		bundleFiles = append(bundleFiles, bundleFile{
			Name: fmt.Sprintf("%s-%s-%s.jar", rem.ArtifactID, rem.Version, classifier),
			Data: data,
		})
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

	// 4. Fetch the VEX whose BOM-Link matches this SBOM and name it for delivery.
	serial, err := sbomSerialNumber(sbomData)
	if err != nil {
		return fmt.Errorf("sbom serialNumber: %w", err)
	}
	vexDoc, err := h.registry.FetchVEX(ctx, dedupeStrings(rem.CVEsFixed), rem.GroupID, rem.ArtifactID, rem.Version, serial)
	if err != nil {
		return fmt.Errorf("fetch VEX: %w", err)
	}
	vexName, err := sonatypeVEXName(vexDoc)
	if err != nil {
		return fmt.Errorf("fetch VEX %s: %w", vexDoc.Filename, err)
	}

	// 5. Derive the bucket layout from the SBOM and check it describes this artifact.
	coords, err := parseBOMCoordinates(sbomData, rem)
	if err != nil {
		return fmt.Errorf("SBOM coordinates: %w", err)
	}
	if err := validateCoordinates(coords, rem); err != nil {
		return fmt.Errorf("SBOM coordinates: %w", err)
	}

	// 6. Build the flat zip bundle (JAR, POM, optional sources/javadoc).
	zipData, err := buildBundleZip(bundleFiles)
	if err != nil {
		return fmt.Errorf("build bundle: %w", err)
	}

	// 7. Upload to the partner S3 bucket. The VEX document goes under the
	// bucket-root vex/ directory. The SBOM goes last so its presence marks
	// a complete delivery.
	keyBase := fmt.Sprintf("%s/%s-%s", bundleBasePath(coords), coords.Name, coords.Version)
	zipPath := keyBase + ".zip"
	sbomPath := keyBase + ".bom.json"

	if err := h.s3.UploadFile(ctx, zipPath, zipData, "application/zip"); err != nil {
		return fmt.Errorf("upload bundle: %w", err)
	}
	if err := h.s3.UploadFile(ctx, sbomPath+".asc", sbomASC, "application/pgp-signature"); err != nil {
		return fmt.Errorf("upload SBOM signature: %w", err)
	}
	if err := h.s3.UploadRootFile(ctx, vexDir+"/"+vexName, vexDoc.Data, "application/json"); err != nil {
		return fmt.Errorf("upload VEX %s: %w", vexName, err)
	}
	if err := h.s3.UploadFile(ctx, sbomPath, sbomData, "application/json"); err != nil {
		return fmt.Errorf("upload SBOM: %w", err)
	}

	// 8. Verify
	if err := h.s3.VerifyDelivery(ctx, zipPath); err != nil {
		return fmt.Errorf("verify delivery: %w", err)
	}

	return nil
}

func (h *sonatypeHandler) Status() map[string]interface{} {
	cfg := config.Get().SonatypeBridge
	// Never include S3 credentials in this response.
	return map[string]interface{}{
		"enabled":          cfg.Enabled,
		"s3_url":           cfg.S3.URL,
		"s3_bucket":        cfg.S3.Name,
		"s3_region":        cfg.S3.Region,
		"registry_url":     cfg.RegistryURL,
		"registry_osv_url": cfg.RegistryOSVURL,
		"registry_vex_url": cfg.RegistryVEXURL,
	}
}

type sonatypeHealthResponse struct {
	Healthy  bool          `json:"healthy"`
	S3       serviceHealth `json:"s3"`
	Registry serviceHealth `json:"registry"`
}

// Health probes the partner S3 bucket and the Lightwell registry.
func (h *sonatypeHandler) Health(ctx context.Context) (bool, interface{}) {
	s3Err := h.s3.Ping(ctx)
	registryErr := h.registry.Ping(ctx)

	healthy := s3Err == nil && registryErr == nil
	return healthy, sonatypeHealthResponse{
		Healthy:  healthy,
		S3:       newServiceHealth(s3Err),
		Registry: newServiceHealth(registryErr),
	}
}

// sonatypeVEXName returns the delivered VEX filename: "{cveID}-{utc}.bom.json"
// for a single-CVE document, "{utc}.bom.json" for a multi-CVE one.
func sonatypeVEXName(doc VEXDocument) (string, error) {
	if doc.CVEID == "" {
		return bundledVEXObjectName(doc.Timestamp)
	}
	return vexObjectName(doc.CVEID, doc.Timestamp)
}

// vexObjectName builds the delivered single-CVE VEX filename
// "{cveID}-{full-UTC-timestamp}.bom.json" from the ISO-8601 build timestamp,
// normalized to UTC.
func vexObjectName(cveID, timestamp string) (string, error) {
	utcTimestamp, err := vexTimestamp(timestamp)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s.bom.json", cveID, utcTimestamp), nil
}

// bundledVEXObjectName builds the delivered multi-CVE VEX filename
// "{full-UTC-timestamp}.bom.json" from the ISO-8601 build timestamp, normalized to UTC.
func bundledVEXObjectName(timestamp string) (string, error) {
	utcTimestamp, err := vexTimestamp(timestamp)
	if err != nil {
		return "", err
	}
	return utcTimestamp + ".bom.json", nil
}

func vexTimestamp(timestamp string) (string, error) {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return "", fmt.Errorf("parse VEX timestamp %q: %w", timestamp, err)
	}
	return t.UTC().Format(time.RFC3339Nano), nil
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
