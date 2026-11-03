package copr_ingest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/IBM/sarama"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testEventSource = "urn:content-sources:test"

func TestParseEnvelope_InvalidJSON(t *testing.T) {
	_, err := parseEnvelope([]byte(`{"specversion": "1.0",`))
	assert.ErrorIs(t, err, errInvalidMessage)
	assert.ErrorContains(t, err, "cannot decode cloudevent")
}

func TestParseBuildEndPayload_MissingData(t *testing.T) {
	ce := cloudevents.NewEvent()
	ce.SetID(testEventID)
	ce.SetSource(testEventSource)
	ce.SetType(BuildEndEventType)

	_, err := parseBuildEndPayload(ce)
	assert.ErrorIs(t, err, errInvalidMessage)
	assert.ErrorContains(t, err, "invalid build end data: missing data")
}

func TestBuildEndHandler_HandleCloudEvent_UnrelatedEventTypeSkipped(t *testing.T) {
	h, taskClient := newTestHandler(t)
	ce := cloudevents.NewEvent()
	ce.SetID(testEventID)
	ce.SetSource(testEventSource)
	ce.SetType("copr.build.start")
	require.NoError(t, ce.SetData(cloudevents.ApplicationJSON, map[string]any{"build": "not-a-number"}))
	value, err := json.Marshal(ce)
	require.NoError(t, err)
	msg := &sarama.ConsumerMessage{Value: value}

	err = h.handleCloudEvent(context.Background(), msg)
	assert.NoError(t, err)
	taskClient.AssertNumberOfCalls(t, "Enqueue", 0)
}
