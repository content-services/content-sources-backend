package event

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event/schema"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/rs/zerolog/log"
)

// https://github.com/edenhill/librdkafka/blob/master/CONFIGURATION.md
// https://docs.confluent.io/platform/current/clients/consumer.html#ak-consumer-configuration

// NewConsumer create a new consumer based on the configuration
// supplied.
// config Provide the necessary configuration to create the consumer.
func NewConsumer(config *KafkaConfig) (*kafka.Consumer, error) {
	var (
		consumer *kafka.Consumer
		err      error
	)

	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers":        config.Bootstrap.Servers,
		"group.id":                 config.Group.Id,
		"auto.offset.reset":        config.Auto.Offset.Reset,
		"auto.commit.interval.ms":  config.Auto.Commit.Interval.Ms,
		"go.logs.channel.enable":   false,
		"allow.auto.create.topics": true,
		// NOTE This could be useful when launching locally
		// "socket.timeout.ms":                  60000,
		// "socket.connection.setup.timeout.ms": 3000,
		// "session.timeout.ms":                 6000,
	}

	if config.Sasl.Username != "" {
		_ = kafkaConfigMap.SetKey("sasl.username", config.Sasl.Username)
		_ = kafkaConfigMap.SetKey("sasl.password", config.Sasl.Password)
		_ = kafkaConfigMap.SetKey("sasl.mechanism", config.Sasl.Mechanism)
		_ = kafkaConfigMap.SetKey("security.protocol", config.Sasl.Protocol)
		_ = kafkaConfigMap.SetKey("ssl.ca.location", config.Capath)
	}

	if consumer, err = kafka.NewConsumer(kafkaConfigMap); err != nil {
		return nil, err
	}

	if err = consumer.SubscribeTopics(config.Topics, nil); err != nil {
		return nil, err
	}
	log.Info().Msgf("Consumer subscribed to topics: %s", strings.Join(config.Topics, ","))

	return consumer, nil
}

// NewConsumerEventLoop creates a consumer event loop, which is awaiting for
// new kafka messages and process them by the specified handler.
//
// consumer is an initialized kafka.Consumer. It cannot be nil.
// handler is the event handler which will dispatch the received messages.
// It cannot be nil.
//
// Return a function that represent the event loop or a panic if a failure
// happens.
func NewConsumerEventLoop(ctx context.Context, consumer *kafka.Consumer, handler Eventable, metrics *m.Metrics) func() {
	var (
		err     error
		msg     *kafka.Message
		schemas schema.TopicSchemas
	)
	if consumer == nil {
		panic(fmt.Errorf("consumer cannot be nil"))
	}
	if handler == nil {
		panic(fmt.Errorf("handler cannot be nil"))
	}
	if metrics == nil {
		panic(fmt.Errorf("metrics cannot be nil"))
	}
	if schemas, err = schema.LoadSchemas(); err != nil {
		panic(err)
	}

	return func() {
		log.Logger.Info().Msg("Consumer loop awaiting to consume messages")
		for {
			// Message wait loop
			for {
				msg, err = consumer.ReadMessage(1 * time.Second)

				if err != nil {
					val, ok := err.(kafka.Error)
					if !ok || val.Code() != kafka.ErrTimedOut {
						log.Logger.Error().Msgf("error awaiting to read a message: %s", err.Error())
					}

					select {
					case <-ctx.Done():
						log.Logger.Info().Msgf("Context done for NewConsumerEventLoop")
						return
					default:
					}

					continue
				}
				break
			}

			if err = processConsumedMessage(schemas, msg, handler, *metrics); err != nil {
				logEventMessageError(msg, err)
				continue
			}
		}
	}
}

func processConsumedMessage(schemas schema.TopicSchemas, msg *kafka.Message, handler Eventable, metrics m.Metrics) error {
	var err error
	if schemas == nil || msg == nil || handler == nil {
		metrics.RecordMessageResult(false)
		return fmt.Errorf("schemas, msg or handler is nil")
	}
	metrics.RecordMessageLatency(msg.Timestamp)
	if msg.TopicPartition.Topic == nil {
		metrics.RecordMessageResult(false)
		return fmt.Errorf("Topic cannot be nil")
	}

	internalTopic := TopicTranslationConfig.GetInternal(*msg.TopicPartition.Topic)
	if internalTopic == "" {
		metrics.RecordMessageResult(false)
		return fmt.Errorf("Topic maping not found for: %s", *msg.TopicPartition.Topic)
	}
	log.Info().
		Str("Topic name", *msg.TopicPartition.Topic).
		Str("Requested topic name", internalTopic).
		Msg("Topic mapping")
	*msg.TopicPartition.Topic = internalTopic
	logEventMessageInfo(msg, "Consuming message")

	if err = schemas.ValidateMessage(msg); err != nil {
		metrics.RecordMessageResult(false)
		return err
	}

	// Dispatch message
	if err = handler.OnMessage(msg); err != nil {
		metrics.RecordMessageResult(false)
		return err
	}
	metrics.RecordMessageResult(true)
	return nil
}
