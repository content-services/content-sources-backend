package event

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
)

// Eventable represent the interface for any kafka message handler.
type Eventable interface {
	// Process a pure kafka.Message structure.
	// Return nil if it was processed with success, else error.
	OnMessage(msg *kafka.Message) error
}
