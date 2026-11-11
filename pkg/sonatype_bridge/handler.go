package sonatype_bridge

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/rs/zerolog/log"
)

const (
	allowedEventType = "java-remediated"
)

const maxProcessedGAVs = 10000

// BridgeHandler implements sarama.ConsumerGroupHandler and orchestrates
// the full pipeline for each remediation message.
type BridgeHandler struct {
	registry  RegistryClient
	s3        S3Uploader
	metrics   *bridgeMetrics
	processed *boundedSet
}

// NewBridgeHandler creates a BridgeHandler with the given clients.
func NewBridgeHandler(registry RegistryClient, s3 S3Uploader, metrics *bridgeMetrics) *BridgeHandler {
	return &BridgeHandler{
		registry:  registry,
		s3:        s3,
		metrics:   metrics,
		processed: newBoundedSet(maxProcessedGAVs),
	}
}

// Setup is called at the start of a new consumer group session.
func (h *BridgeHandler) Setup(_ sarama.ConsumerGroupSession) error { return nil }

// Cleanup is called at the end of a consumer group session.
func (h *BridgeHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

// ConsumeClaim processes messages from a single partition.
func (h *BridgeHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		h.metrics.messagesReceived.Inc()

		remediations, shouldCommit, err := h.filterAndParse(msg.Value)
		if err != nil {
			log.Warn().Err(err).Msg("message rejected")
			if shouldCommit {
				session.MarkMessage(msg, "")
			}
			continue
		}

		allSucceeded := true
		for _, rem := range remediations {
			gav := gavKey(rem)
			if h.processed.Contains(gav) {
				log.Debug().Str("gav", gav).Msg("skipping already-processed GAV")
				continue
			}

			if err := ValidateNotificationCVEs(rem); err != nil {
				log.Error().Err(err).Str("gav", gav).Msg("embargo check failed")
				h.metrics.embargoRejections.Inc()
				h.metrics.messagesFailed.Inc()
				continue
			}

			if err := h.processRemediation(session.Context(), rem); err != nil {
				var embargoErr *EmbargoError
				if errors.As(err, &embargoErr) {
					log.Error().Err(err).Str("gav", gav).Msg("embargo check failed (content)")
					h.metrics.embargoRejections.Inc()
					h.metrics.messagesFailed.Inc()
					continue
				}
				var checksumErr *ChecksumMismatchError
				if errors.As(err, &checksumErr) {
					log.Error().Err(err).Str("gav", gav).Msg("JAR checksum mismatch")
					h.metrics.checksumRejections.Inc()
					h.metrics.messagesFailed.Inc()
					continue
				}
				log.Error().Err(err).Str("gav", gav).Msg("pipeline failed")
				h.metrics.messagesFailed.Inc()
				allSucceeded = false
				continue
			}

			h.processed.Add(gav)
			h.metrics.messagesProcessed.Inc()
			log.Info().Str("gav", gav).Msg("pipeline completed")
		}

		if allSucceeded {
			session.MarkMessage(msg, "")
		}
	}
	return nil
}

func (h *BridgeHandler) filterAndParse(data []byte) ([]Remediation, bool, error) {
	// The bridge consumes a dedicated topic (platform.lightwell.advisory-created)
	// carrying CloudEvents messages. The ecosystem gate is the CloudEvents
	// "eventtype" extension (java-remediated only until embargoes clear).
	var env struct {
		EventType string `json:"eventtype"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, true, fmt.Errorf("invalid message: %w", err)
	}
	if env.EventType != "" && env.EventType != allowedEventType {
		return nil, true, fmt.Errorf("event_type %q is not %q", env.EventType, allowedEventType)
	}

	remediations, err := ParseRemediations(data)
	if err != nil {
		return nil, true, fmt.Errorf("parse: %w", err)
	}
	return remediations, false, nil
}

// processRemediation runs the full pipeline for a single remediation.
func (h *BridgeHandler) processRemediation(ctx context.Context, rem Remediation) error {
	start := time.Now()
	defer func() {
		h.metrics.pipelineDuration.Observe(time.Since(start).Seconds())
	}()

	gav := gavKey(rem)
	log.Info().Str("gav", gav).Msg("processing remediation")

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

	// 4. Fetch the multi-CVE VEX from the VEX file distribution.
	vexData, err := h.registry.FetchVEX(ctx, rem.CVEsFixed)
	if err != nil {
		return fmt.Errorf("fetch VEX: %w", err)
	}

	// 5. Upload bundle to the partner S3 bucket
	gp := groupPath(rem.GroupID)
	deployBase := fmt.Sprintf("%s/%s/%s", gp, rem.ArtifactID, rem.Version)

	jarPath := fmt.Sprintf("%s/%s-%s.jar", deployBase, rem.ArtifactID, rem.Version)
	if err := h.s3.UploadFile(ctx, jarPath, jarData, "application/java-archive"); err != nil {
		return fmt.Errorf("upload JAR: %w", err)
	}

	pomPath := fmt.Sprintf("%s/%s-%s.pom", deployBase, rem.ArtifactID, rem.Version)
	if err := h.s3.UploadFile(ctx, pomPath, pomData, "application/xml"); err != nil {
		return fmt.Errorf("upload POM: %w", err)
	}

	sbomPath := fmt.Sprintf("%s/sbom/%s-%s.bom.json", deployBase, rem.ArtifactID, rem.Version)
	if err := h.s3.UploadFile(ctx, sbomPath, sbomData, "application/json"); err != nil {
		return fmt.Errorf("upload SBOM: %w", err)
	}
	if err := h.s3.UploadFile(ctx, sbomPath+".asc", sbomASC, "application/pgp-signature"); err != nil {
		return fmt.Errorf("upload SBOM signature: %w", err)
	}

	vexPath := fmt.Sprintf("%s/vex/%s-%s.bom.json", deployBase, rem.ArtifactID, rem.Version)
	if err := h.s3.UploadFile(ctx, vexPath, vexData, "application/json"); err != nil {
		return fmt.Errorf("upload VEX: %w", err)
	}

	// 6. Upload maven-metadata.xml
	metadataXML := generateMavenMetadata(rem.GroupID, rem.ArtifactID, rem.Version)
	metadataPath := fmt.Sprintf("%s/%s/maven-metadata.xml", gp, rem.ArtifactID)
	if err := h.s3.UploadFile(ctx, metadataPath, metadataXML, "application/xml"); err != nil {
		return fmt.Errorf("upload maven-metadata.xml: %w", err)
	}

	// 7. Verify
	if err := h.s3.VerifyDelivery(ctx, jarPath); err != nil {
		return fmt.Errorf("verify delivery: %w", err)
	}

	return nil
}

func gavKey(rem Remediation) string {
	return fmt.Sprintf("%s:%s:%s", rem.GroupID, rem.ArtifactID, rem.Version)
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

// verifyJARChecksum compares the fetched JAR's SHA-256 against the checksum
// carried in the Kafka notification. If the checksums map is nil or empty
// (older producers), verification is skipped for backward compatibility.
func verifyJARChecksum(filename, actualSHA256 string, checksums map[string]string) error {
	if len(checksums) == 0 {
		log.Warn().Str("file", filename).Msg("no checksums in notification, skipping verification")
		return nil
	}
	expected, ok := checksums[filename]
	if !ok {
		keys := make([]string, 0, len(checksums))
		for k := range checksums {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return &ChecksumMismatchError{
			msg: fmt.Sprintf("JAR %s absent from checksums map (available: %s)",
				filename, strings.Join(keys, ", ")),
		}
	}
	if actualSHA256 != expected {
		return &ChecksumMismatchError{
			msg: fmt.Sprintf("JAR checksum mismatch for %s: expected %s, got %s", filename, expected, actualSHA256),
		}
	}
	return nil
}
