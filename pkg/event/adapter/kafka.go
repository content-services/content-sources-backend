package adapter

import (
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/random"
)

type KafkaHeaders interface {
	FromEchoContext(ctx echo.Context, event string) (headers []kafka.Header, err error)
	getEchoHeader(ctx echo.Context, key string, defvalues []string) []string
}

type KafkaAdapter struct{}

func NewKafkaHeaders() KafkaHeaders {
	return KafkaAdapter{}
}

// FIXME Code duplicated from pkg/handler but if it is included a cycle dependency happens
//       Find a better solution than duplicate it
func (a KafkaAdapter) getEchoHeader(ctx echo.Context, key string, defvalues []string) []string {
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
	xrhIdentity := a.getEchoHeader(ctx, headerKey, []string{})
	if len(xrhIdentity) == 0 {
		return []kafka.Header{}, fmt.Errorf("expected a value for '%s' http header", headerKey)
	}

	headerKey = string(message.HdrXRhInsightsRequestId)
	xrhInsightsRequestId := a.getEchoHeader(ctx, headerKey, []string{random.String(32)})

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
