package producer

import (
	"net/http"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIntrospectRequest(t *testing.T) {
	var (
		introspectRequest IntrospectRequest
		err               error
	)
	event.TopicTranslationConfig = event.NewTopicTranslationWithDefaults()

	producer, err := NewProducer(helperGetKafkaConfig())
	require.NoError(t, err)
	require.NotNil(t, producer)
	defer producer.Close()

	// When producer is nil
	introspectRequest, err = NewIntrospectRequest(nil)
	require.Error(t, err)
	assert.Nil(t, introspectRequest)
	assert.Equal(t, "producer cannot be nil", err.Error())

	// Success result
	introspectRequest, err = NewIntrospectRequest(producer)
	require.NoError(t, err)
	assert.NotNil(t, introspectRequest)
}

func TestIntrospectRequestProduce(t *testing.T) {
	event.TopicTranslationConfig = event.NewTopicTranslationWithDefaults()

	producer, err := NewProducer(helperGetKafkaConfig())
	require.NoError(t, err)
	require.NotNil(t, producer)
	defer producer.Close()

	introspectRequestProducer, err := NewIntrospectRequest(producer)
	require.NoError(t, err)
	require.NotNil(t, introspectRequestProducer)

	// Error when adapting to kafka interface
	err = introspectRequestProducer.Produce(
		nil,
		&message.IntrospectRequestMessage{},
	)
	require.Error(t, err)
	assert.Equal(t, "Error adapting to kafka interface: ctx cannot be nil", err.Error())

	// Error when producing the message
	err = introspectRequestProducer.Produce(
		echo.New().NewContext(
			&http.Request{
				Header: http.Header{
					// ./scripts/header.sh 999999
					string(message.HdrXRhIdentity): {"eyJpZGVudGl0eSI6eyJ0eXBlIjoiQXNzb2NpYXRlIiwiYWNjb3VudF9udW1iZXIiOiIiLCJpbnRlcm5hbCI6eyJvcmdfaWQiOiI5OTk5OTkifX19Cg=="},
				},
			},
			&echo.Response{},
		),
		&message.IntrospectRequestMessage{},
	)
	require.Error(t, err)
	assert.Equal(t, err.Error(), "key cannot be an empty string")

	// // Success scenario
	var uuidTest uuid.UUID
	uuidTest, err = uuid.NewV4()
	require.NoError(t, err)
	err = introspectRequestProducer.Produce(
		echo.New().NewContext(
			&http.Request{
				Header: http.Header{
					// ./scripts/header.sh 999999
					string(message.HdrXRhIdentity): {"eyJpZGVudGl0eSI6eyJ0eXBlIjoiQXNzb2NpYXRlIiwiYWNjb3VudF9udW1iZXIiOiIiLCJpbnRlcm5hbCI6eyJvcmdfaWQiOiI5OTk5OTkifX19Cg=="},
				},
			},
			&echo.Response{},
		),
		&message.IntrospectRequestMessage{
			Uuid: uuidTest.String(),
			Url:  "https://example.test",
		},
	)
	require.NoError(t, err)
}
