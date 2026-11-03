package copr_ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var errInvalidMessage = errors.New("invalid message")

type BuildEndHandler struct {
	taskClient          client.TaskClient
	metrics             *instrumentation.Metrics
	enqueueRetryBackoff time.Duration
}

func NewBuildEndHandler(taskClient client.TaskClient, metrics *instrumentation.Metrics) *BuildEndHandler {
	return &BuildEndHandler{taskClient: taskClient, metrics: metrics, enqueueRetryBackoff: enqueueRetryBackoff}
}

// Setup is called at the start of a new consumer group session.
func (h *BuildEndHandler) Setup(_ sarama.ConsumerGroupSession) error { return nil }

// Cleanup is called at the end of a consumer group session.
func (h *BuildEndHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

// ConsumeClaim processes messages from a single partition.
func (h *BuildEndHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
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
func (h *BuildEndHandler) consumeMessage(ctx context.Context, msg *sarama.ConsumerMessage) bool {
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
		h.metrics.RecordCoprBuildEndResult(instrumentation.CoprBuildEndResultInvalid)
		logger.Warn().Err(err).Msg("skipping invalid message")
	} else {
		h.metrics.RecordCoprBuildEndResult(instrumentation.CoprBuildEndResultFailed)
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
func (h *BuildEndHandler) handleCloudEvent(ctx context.Context, msg *sarama.ConsumerMessage) error {
	ce, err := parseEnvelope(msg.Value)
	if err != nil {
		return err
	}
	h.metrics.RecordCoprEventDelay(ce.Time())

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
