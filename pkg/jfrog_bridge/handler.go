package jfrog_bridge

import (
	"context"
	"encoding/json"
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

// BridgeHandler implements sarama.ConsumerGroupHandler for one partner. It
// parses and gates each message, then hands every remediation to the
// partner's handler.
type BridgeHandler struct {
	partner   PartnerHandler
	metrics   *partnerMetrics
	processed *boundedSet
}

// NewBridgeHandler creates a BridgeHandler that delivers to the given partner.
func NewBridgeHandler(partner PartnerHandler, metrics *bridgeMetrics) *BridgeHandler {
	return &BridgeHandler{
		partner:   partner,
		metrics:   metrics.forPartner(partner.Name()),
		processed: newBoundedSet(maxProcessedGAVs),
	}
}

// Setup is called at the start of a new consumer group session.
func (h *BridgeHandler) Setup(_ sarama.ConsumerGroupSession) error { return nil }

// Cleanup is called at the end of a consumer group session.
func (h *BridgeHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

// ConsumeClaim processes messages from a single partition.
func (h *BridgeHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	partnerName := h.partner.Name()
	for msg := range claim.Messages() {
		h.metrics.messagesReceived.Inc()

		remediations, shouldCommit, err := h.filterAndParse(msg.Value)
		if err != nil {
			log.Warn().Err(err).Str("partner", partnerName).Msg("message rejected")
			if shouldCommit {
				session.MarkMessage(msg, "")
			}
			continue
		}

		allSucceeded := true
		for _, rem := range remediations {
			gav := gavKey(rem)
			if h.processed.Contains(gav) {
				log.Debug().Str("partner", partnerName).Str("gav", gav).Msg("skipping already-processed GAV")
				continue
			}

			if err := ValidateNotificationCVEs(rem); err != nil {
				log.Error().Err(err).Str("partner", partnerName).Str("gav", gav).Msg("embargo check failed")
				h.metrics.embargoRejections.Inc()
				h.metrics.messagesFailed.Inc()
				continue
			}

			if err := h.process(session.Context(), rem); err != nil {
				var embargoErr *EmbargoError
				if errors.As(err, &embargoErr) {
					log.Error().Err(err).Str("partner", partnerName).Str("gav", gav).Msg("embargo check failed (content)")
					h.metrics.embargoRejections.Inc()
					h.metrics.messagesFailed.Inc()
					continue
				}
				var checksumErr *ChecksumMismatchError
				if errors.As(err, &checksumErr) {
					log.Error().Err(err).Str("partner", partnerName).Str("gav", gav).Msg("JAR checksum mismatch")
					h.metrics.checksumRejections.Inc()
					h.metrics.messagesFailed.Inc()
					continue
				}
				var conflictErr *ObjectConflictError
				if errors.As(err, &conflictErr) {
					log.Error().Err(err).Str("partner", partnerName).Str("gav", gav).Msg("partner object conflict")
					h.metrics.conflictRejections.Inc()
					h.metrics.messagesFailed.Inc()
					continue
				}
				log.Error().Err(err).Str("partner", partnerName).Str("gav", gav).Msg("pipeline failed")
				h.metrics.messagesFailed.Inc()
				allSucceeded = false
				continue
			}

			h.processed.Add(gav)
			h.metrics.messagesProcessed.Inc()
			log.Info().Str("partner", partnerName).Str("gav", gav).Msg("pipeline completed")
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

// process runs the partner pipeline for a single remediation.
func (h *BridgeHandler) process(ctx context.Context, rem Remediation) error {
	start := time.Now()
	defer func() {
		h.metrics.pipelineDuration.Observe(time.Since(start).Seconds())
	}()

	log.Info().Str("partner", h.partner.Name()).Str("gav", gavKey(rem)).Msg("processing remediation")
	return h.partner.Process(ctx, rem)
}

func gavKey(rem Remediation) string {
	return fmt.Sprintf("%s:%s:%s", rem.GroupID, rem.ArtifactID, rem.Version)
}

func sbomSerialNumber(data []byte) (string, error) {
	var doc struct {
		SerialNumber string `json:"serialNumber"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("parse SBOM: %w", err)
	}
	if doc.SerialNumber == "" {
		return "", fmt.Errorf("SBOM serialNumber is empty")
	}
	return doc.SerialNumber, nil
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
