package kafka

import (
	"testing"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
)

func TestNewKafkaConsumer_NoBrokers(t *testing.T) {
	consumer, err := NewKafkaConsumer([]string{}, "test-group", sarama.NewConfig())
	assert.ErrorContains(t, err, "no brokers configured")
	assert.Nil(t, consumer)
}
