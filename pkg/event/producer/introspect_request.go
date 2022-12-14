package producer

import (
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event/adapter"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/content-services/content-sources-backend/pkg/event/schema"
	"github.com/labstack/echo/v4"
)

// Interface which define the producer for the IntrospectRequestMessage message
type IntrospectRequest interface {
	Produce(ctx echo.Context, msg *message.IntrospectRequestMessage) error
}

// Implementation for the specific producer
type IntrospectRequestProducer struct {
	producer *kafka.Producer
}

// NewIntrospectRequest Create the specific producer for IntrospectRequest message.
// producer is the reference to the kafka.Producer; see NewProducer(...) function.
// Return an IntrospectRequest interface and nil error if success, else nil interface
// and a filled error with the information about the situation.
func NewIntrospectRequest(producer *kafka.Producer) (IntrospectRequest, error) {
	if producer == nil {
		return nil, fmt.Errorf("producer cannot be nil")
	}
	output := &IntrospectRequestProducer{
		producer: producer,
	}
	return output, nil
}

// Produce Implement the specific method to produce an IntrospectRequestMessage.
// ctx Reference to the echo.Context to map the necessary headers.
// msg Reference to the IntrospectRequestMessage; it cannot be nil.
// Return nil if success, else an error filled with the information about the
// situation.
func (p *IntrospectRequestProducer) Produce(ctx echo.Context, msg *message.IntrospectRequestMessage) error {
	topic := schema.TopicIntrospect
	key := msg.Uuid
	headers, err := adapter.NewKafkaHeaders().FromEchoContext(ctx, message.HdrTypeIntrospect)
	if err != nil {
		return fmt.Errorf("Error adapting to kafka interface: %w", err)
	}
	if err = Produce(p.producer, topic, key, msg, headers...); err != nil {
		return err
	}
	return nil
}
