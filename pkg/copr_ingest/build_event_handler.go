package copr_ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/IBM/sarama"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	BuildEndEventType       = "copr.build.end"
	maxRepositoryNameLength = 255 // length limit for repository_configurations name column
)

var errInvalidMessage = errors.New("invalid message")

type BuildEventHandler struct {
	taskClient          client.TaskClient
	metrics             *instrumentation.Metrics
	enqueueRetryBackoff time.Duration
}

func NewBuildEventHandler(taskClient client.TaskClient, metrics *instrumentation.Metrics) *BuildEventHandler {
	return &BuildEventHandler{taskClient: taskClient, metrics: metrics, enqueueRetryBackoff: enqueueRetryBackoff}
}

// Setup is called at the start of a new consumer group session.
func (h *BuildEventHandler) Setup(_ sarama.ConsumerGroupSession) error { return nil }

// Cleanup is called at the end of a consumer group session.
func (h *BuildEventHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

// ConsumeClaim processes messages from a single partition.
func (h *BuildEventHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			if !h.consumeMessage(session.Context(), msg) {
				// The session ended while retrying, the message stays unmarked and is consumed again
				return nil
			}
			session.MarkMessage(msg, "")
		case <-session.Context().Done():
			return nil
		}
	}
}

// consumeMessage returns false if the session ended before handling, leaving the message unmarked for redelivery.
func (h *BuildEventHandler) consumeMessage(ctx context.Context, msg *sarama.ConsumerMessage) bool {
	ctx, logger := withMessageLogger(ctx, msg)

	err := h.handleCloudEvent(ctx, msg)
	if err == nil {
		return true
	}
	if ctx.Err() != nil {
		logger.Info().Err(err).Msg("session ended while handling message, it will be consumed again")
		return false
	}
	if errors.Is(err, errInvalidMessage) {
		h.metrics.RecordCoprBuildEndIngest(instrumentation.CoprBuildEndIngestInvalid)
		logger.Warn().Err(err).Msg("skipping invalid message")
	} else {
		h.metrics.RecordCoprBuildEndIngest(instrumentation.CoprBuildEndIngestFailed)
		logger.Error().Err(err).Msg("failed to handle message, skipping")
	}
	return true
}

// withMessageLogger returns ctx and the per-message logger stored in it (a WithContext copy), so later UpdateContext fields stay isolated to this message.
func withMessageLogger(ctx context.Context, msg *sarama.ConsumerMessage) (context.Context, *zerolog.Logger) {
	ctx = log.With().Str("topic", msg.Topic).Int32("partition", msg.Partition).Int64("offset", msg.Offset).Logger().WithContext(ctx)
	return ctx, zerolog.Ctx(ctx)
}

// handleCloudEvent decodes the CloudEvent in the Kafka message and handles the build end data.
func (h *BuildEventHandler) handleCloudEvent(ctx context.Context, msg *sarama.ConsumerMessage) error {
	ce, err := parseEnvelope(msg.Value)
	if err != nil {
		return err
	}

	// Copr sends all event types to the same topic, so other types are expected and skipped without counting them
	if ce.Type() != BuildEndEventType {
		zerolog.Ctx(ctx).Debug().Str("event_type", ce.Type()).Msg("skipping event type")
		return nil
	}

	// request_id is the snapshot task's request ID
	zerolog.Ctx(ctx).UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Str(config.RequestIdLoggingKey, ce.ID()).Str("event_type", ce.Type())
	})
	buildEnd, err := parseBuildEndPayload(ce)
	if err != nil {
		return err
	}
	return h.handleBuildEnd(ctx, ce.ID(), buildEnd)
}

// parseEnvelope decodes and validates a structured mode CloudEvent from the record value.
func parseEnvelope(data []byte) (cloudevents.Event, error) {
	var ce cloudevents.Event
	if err := json.Unmarshal(data, &ce); err != nil {
		return cloudevents.Event{}, fmt.Errorf("%w: cannot decode cloudevent: %w", errInvalidMessage, err)
	}
	if err := ce.Validate(); err != nil {
		return cloudevents.Event{}, fmt.Errorf("%w: invalid cloudevent: %w", errInvalidMessage, err)
	}
	return ce, nil
}

// parseBuildEndPayload decodes the Copr build end data carried by the CloudEvent.
func parseBuildEndPayload(ce cloudevents.Event) (buildEndData, error) {
	// DataAs returns nil without decoding anything when the event has no data
	if len(ce.Data()) == 0 {
		return buildEndData{}, fmt.Errorf("%w: invalid build end data: missing data", errInvalidMessage)
	}
	var data buildEndData
	if err := ce.DataAs(&data); err != nil {
		return buildEndData{}, fmt.Errorf("%w: cannot decode build end data: %w", errInvalidMessage, err)
	}
	return data, nil
}

// TODO: add the partner org ID and Pulp domain fields once Copr sends them, until then both come from config
type buildEndData struct {
	PublicationHref string `json:"publication_href"` // the snapshot task resolves the Copr Pulp repository from it
	Chroot          string `json:"chroot"`
	Build           int64  `json:"build"`
	Owner           string `json:"owner"`
	Copr            string `json:"copr"`
	Status          *int   `json:"status"`
}

type validBuildEnd struct {
	PublicationHref string
	Build           int64
	Owner           string
	Copr            string
	Distro          string
	Version         string
	Arch            string
}

// parseValidBuildEnd checks that a successful build end has the fields needed for the snapshot task and a well-formed chroot.
func parseValidBuildEnd(d buildEndData) (validBuildEnd, error) {
	var missing []string
	if d.PublicationHref == "" {
		missing = append(missing, "publication_href")
	}
	if d.Chroot == "" {
		missing = append(missing, "chroot")
	}
	if d.Build <= 0 {
		missing = append(missing, "build")
	}
	if d.Owner == "" {
		missing = append(missing, "owner")
	}
	if d.Copr == "" {
		missing = append(missing, "copr")
	}
	if len(missing) > 0 {
		return validBuildEnd{}, fmt.Errorf("%w: invalid build end data: missing %s", errInvalidMessage, strings.Join(missing, ", "))
	}
	distro, version, arch, err := parseChroot(d.Chroot)
	if err != nil {
		return validBuildEnd{}, err
	}
	return validBuildEnd{
		PublicationHref: d.PublicationHref,
		Build:           d.Build,
		Owner:           d.Owner,
		Copr:            d.Copr,
		Distro:          distro,
		Version:         version,
		Arch:            arch,
	}, nil
}

// handleBuildEnd rejects malformed build ends, skips failed builds and unsupported chroots, and enqueues a snapshot task for the rest.
func (h *BuildEventHandler) handleBuildEnd(ctx context.Context, eventID string, data buildEndData) error {
	logger := zerolog.Ctx(ctx)
	logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Int64("copr_build_id", data.Build).Str("chroot", data.Chroot)
	})
	if data.Status == nil {
		return fmt.Errorf("%w: invalid build end data: missing status", errInvalidMessage)
	}
	if *data.Status != 1 { // Copr build status: 1 succeeded, 0 failed
		logger.Debug().Int("status", *data.Status).Msg("skipping unsuccessful build")
		return nil
	}
	build, err := parseValidBuildEnd(data)
	if err != nil {
		return err
	}
	if !supportedChroot(build.Distro, build.Version, build.Arch) {
		logger.Debug().Msg("skipping unsupported chroot")
		return nil
	}
	payload, err := toSnapshotTaskPayload(build)
	if err != nil {
		return err
	}
	if err := h.enqueue(ctx, eventID, payload); err != nil {
		return err
	}
	h.metrics.RecordCoprBuildEndIngest(instrumentation.CoprBuildEndIngestEnqueued)
	return nil
}

// toSnapshotTaskPayload maps a validated build to a snapshot task payload.
func toSnapshotTaskPayload(build validBuildEnd) (payloads.CreatePartnerSnapshotPayload, error) {
	projectID := build.Owner + "/" + build.Copr
	// The owner is part of the name, since repository names are unique per org and project names are only unique per owner
	name := fmt.Sprintf("%s for %s %s %s", projectID, config.CoprChrootDistros[build.Distro], build.Version, build.Arch) // e.g., "@nvidia/cuda for RHEL 9 aarch64"
	if len(name) > maxRepositoryNameLength {
		return payloads.CreatePartnerSnapshotPayload{}, fmt.Errorf("%w: repository name %q is longer than %d characters",
			errInvalidMessage, name, maxRepositoryNameLength)
	}
	return payloads.CreatePartnerSnapshotPayload{
		PublicationHref:   build.PublicationHref,
		RepositoryName:    name,
		RepositoryVersion: build.Version,
		RepositoryArch:    build.Arch,
		CoprProjectID:     projectID,
		CoprBuildID:       strconv.FormatInt(build.Build, 10),
	}, nil
}

// parseChroot splits "<distro>-<version>-<arch>", taking version and arch from the end.
func parseChroot(chroot string) (distro, version, arch string, err error) {
	parts := strings.Split(chroot, "-")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("%w: invalid chroot %q, expected <distro>-<version>-<arch>", errInvalidMessage, chroot)
	}
	if slices.Contains(parts, "") {
		return "", "", "", fmt.Errorf("%w: invalid chroot %q, expected <distro>-<version>-<arch>", errInvalidMessage, chroot)
	}
	n := len(parts)
	return strings.Join(parts[:n-2], "-"), parts[n-2], parts[n-1], nil
}

// supportedChroot reports whether distro, version, and arch are allowed by Content Sources.
func supportedChroot(distro, version, arch string) bool {
	if _, ok := config.CoprChrootDistros[distro]; !ok {
		return false
	}
	validVersion, _ := config.ValidDistributionVersionLabels([]string{version})
	return validVersion && version != config.ANY_VERSION && config.ValidArchLabel(arch) && arch != config.ANY_ARCH
}
