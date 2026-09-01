package jfrog_bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/rs/zerolog/log"
)

const (
	allowedEventType = "java-remediated"
)

// BridgeHandler implements sarama.ConsumerGroupHandler and orchestrates
// the full pipeline for each remediation message.
type BridgeHandler struct {
	registry    RegistryClient
	jfrog       JFrogClient
	evidence    EvidenceCreator
	metrics     *bridgeMetrics
	registryURL string
	// GAV dedup: records only after successful processing
	processed sync.Map
}

// NewBridgeHandler creates a BridgeHandler with the given clients.
func NewBridgeHandler(registry RegistryClient, jfrog JFrogClient, evidence EvidenceCreator, metrics *bridgeMetrics) *BridgeHandler {
	return &BridgeHandler{
		registry: registry,
		jfrog:    jfrog,
		evidence: evidence,
		metrics:  metrics,
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
			if _, loaded := h.processed.Load(gav); loaded {
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
				log.Error().Err(err).Str("gav", gav).Msg("pipeline failed")
				h.metrics.messagesFailed.Inc()
				allSucceeded = false
				continue
			}

			h.processed.Store(gav, true)
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
	// so every message is a lightwell advisory. Only filter by event_type to
	// gate ecosystems (java-remediated only until embargoes clear).
	var env struct {
		EventType string `json:"event_type"`
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

	// 1. Fetch JAR + POM from registry
	jarData, jarSHA256, err := h.registry.FetchJAR(ctx, rem.GroupID, rem.ArtifactID, rem.Version)
	if err != nil {
		return fmt.Errorf("fetch JAR: %w", err)
	}

	pomData, err := h.registry.FetchPOM(ctx, rem.GroupID, rem.ArtifactID, rem.Version)
	if err != nil {
		return fmt.Errorf("fetch POM: %w", err)
	}

	// 2. Fetch OSV records
	osvRecords, err := h.registry.FetchOSVRecords(ctx, rem.BaseVersion)
	if err != nil {
		return fmt.Errorf("fetch OSV records: %w", err)
	}

	// 2b. Validate OSV records contain only public CVEs
	if err := ValidateOSVRecords(osvRecords); err != nil {
		return fmt.Errorf("content embargo check: %w", err)
	}

	// 3. Generate CycloneDX VEX
	cdxData, err := GenerateCycloneDXVEX(rem.GroupID, rem.ArtifactID, rem.Version, rem.BaseVersion, osvRecords)
	if err != nil {
		return fmt.Errorf("generate CycloneDX VEX: %w", err)
	}

	// 4. Generate OpenVEX predicate (for Evidence, not uploaded as file)
	openVEXData, err := GenerateOpenVEXPredicate(rem.GroupID, rem.ArtifactID, rem.Version, osvRecords)
	if err != nil {
		return fmt.Errorf("generate OpenVEX predicate: %w", err)
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

	cdxPath := fmt.Sprintf("%s/%s-%s.cdx.vex.json", deployBase, rem.ArtifactID, rem.Version)
	if err := h.jfrog.UploadFile(ctx, cdxPath, cdxData, "application/json"); err != nil {
		return fmt.Errorf("upload CycloneDX VEX: %w", err)
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
		regURL = loadConfig().RegistryURL
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

	// 8. Sign and deploy Evidence
	if err := h.evidence.CreateAndDeploy(ctx, openVEXData, jarPath, jarSHA256); err != nil {
		return fmt.Errorf("evidence: %w", err)
	}

	// 9. Verify
	if err := h.jfrog.VerifyDelivery(ctx, jarPath); err != nil {
		return fmt.Errorf("verify delivery: %w", err)
	}

	return nil
}

func gavKey(rem Remediation) string {
	return fmt.Sprintf("%s:%s:%s", rem.GroupID, rem.ArtifactID, rem.Version)
}

func generateMavenMetadata(groupID, artifactID, version string) []byte {
	ts := time.Now().UTC().Format("20060102150405")
	return []byte(fmt.Sprintf(
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"+
			"<metadata>\n"+
			"  <groupId>%s</groupId>\n"+
			"  <artifactId>%s</artifactId>\n"+
			"  <versioning>\n"+
			"    <latest>%s</latest>\n"+
			"    <release>%s</release>\n"+
			"    <versions><version>%s</version></versions>\n"+
			"    <lastUpdated>%s</lastUpdated>\n"+
			"  </versioning>\n"+
			"</metadata>\n",
		groupID, artifactID, version, version, version, ts))
}

