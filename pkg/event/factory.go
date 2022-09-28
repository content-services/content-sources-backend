package event

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/rs/zerolog/log"
)

// Adapted from: https://github.com/RedHatInsights/playbook-dispatcher/blob/master/internal/response-consumer/main.go#L21

func Start(config *KafkaConfig, handler Eventable) {
	var (
		err      error
		consumer *kafka.Consumer
	)

	if consumer, err = NewConsumer(config); err != nil {
		log.Logger.Panic().Msgf("error creating consumer: %s", err.Error())
		return
	}
	defer consumer.Close()

	start := NewConsumerEventLoop(consumer, handler)
	start()
}
