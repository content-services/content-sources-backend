package event

import (
	"context"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/rs/zerolog/log"
)

// Adapted from: https://github.com/RedHatInsights/playbook-dispatcher/blob/master/internal/response-consumer/main.go#L21

// Start initiate a kafka run loop consumer given the
// configuration and the event handler for the received
// messages.
// config a reference to an initialized KafkaConfig. It cannot be nil.
// handler is the event handler which receive the read messages.
func Start(ctx context.Context, config *KafkaConfig, handler Eventable, m *m.Metrics) {
	var (
		err      error
		consumer *kafka.Consumer
	)

	if consumer, err = NewConsumer(config); err != nil {
		log.Logger.Panic().Msgf("error creating consumer: %s", err.Error())
		return
	}
	defer consumer.Close()

	start := NewConsumerEventLoop(ctx, consumer, handler, m)
	start()
}
