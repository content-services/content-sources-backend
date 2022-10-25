package adapter

import (
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/random"
)

// KafkaHeaders is the adapter interface to translate to kafka.Header slice
//   which is used to compose a kafka message.
type KafkaHeaders interface {
	// FromEchoContext translate from an echo.Context to []kafka.Header
	// ctx is the echo.Context from an http handler.
	// event is an additional type to identify exactly the schema which match
	//   with the kafka message.
	// Return headers a slice of kafka.Header and nil error when success, else
	//   an error reference filled and an empty slice of kafka.Header.
	FromEchoContext(ctx echo.Context, event string) (headers []kafka.Header, err error)
}

// KafkaAdapter represent a specific implementation from the KafkaHeaders adapter interface.
type KafkaAdapter struct{}

// NewKafkaHeaders create KafkaAdapter and return the KafkaHeaders interface.
// Return KafkaHeaders interface.
func NewKafkaHeaders() KafkaHeaders {
	return KafkaAdapter{}
}

// FIXME Code duplicated from pkg/handler but if it is included a cycle dependency happens
//       Find a better solution than duplicate it
func getEchoHeader(ctx echo.Context, key string, defvalues []string) []string {
	if val, ok := ctx.Request().Header[key]; ok {
		return val
	}
	return defvalues
}

func (a KafkaAdapter) FromEchoContext(ctx echo.Context, event string) (headers []kafka.Header, err error) {
	if ctx == nil {
		return []kafka.Header{}, fmt.Errorf("ctx cannot be nil")
	}
	if event == "" {
		return []kafka.Header{}, fmt.Errorf("event cannot be an empty string")
	}
	var (
		headerKey string
	)

	headerKey = string(message.HdrXRhIdentity)
	xrhIdentity := getEchoHeader(ctx, headerKey, []string{})
	if len(xrhIdentity) == 0 {
		return []kafka.Header{}, fmt.Errorf("expected a value for '%s' http header", headerKey)
	}

	headerKey = string(message.HdrXRhInsightsRequestId)
	xrhInsightsRequestId := getEchoHeader(ctx, headerKey, []string{random.String(32)})

	// Fill headers
	headers = []kafka.Header{
		{
			Key:   string(message.HdrType),
			Value: []byte(message.HdrTypeIntrospect),
		},
		{
			Key:   string(message.HdrXRhIdentity),
			Value: []byte(xrhIdentity[0]),
		},
		{
			Key:   string(message.HdrXRhInsightsRequestId),
			Value: []byte(xrhInsightsRequestId[0]),
		},
	}

	return headers, nil
}
