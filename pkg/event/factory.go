package event

import (
	"github.com/content-services/content-sources-backend/pkg/config"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/rs/zerolog/log"
)

// Adapted from: https://github.com/RedHatInsights/playbook-dispatcher/blob/master/internal/response-consumer/main.go#L21

func Start(config *config.Configuration, handler Eventable) {
	var (
		err      error
		consumer *kafka.Consumer
	)

	if consumer, err = NewConsumer(config); err != nil {
		log.Logger.Panic().Msg("[Start] error creating consumer")
		return
	}
	defer consumer.Close()

	start := NewConsumerEventLoop(consumer, handler)
	start()
}
