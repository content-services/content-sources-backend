package event

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
)

type Eventable interface {
	OnMessage(msg *kafka.Message) error
}
