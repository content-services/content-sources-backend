package event

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/content-services/content-sources-backend/pkg/event/schema"
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

func getHeader(msg *kafka.Message, key string) (*kafka.Header, error) {
	if msg == nil {
		return nil, fmt.Errorf("msg is nil")
	}
	if key == "" {
		return nil, fmt.Errorf("key is empty")
	}
	for _, header := range msg.Headers {
		if header.Key == key {
			return &header, nil
		}
	}
	return nil, fmt.Errorf("could not find '%s' in message header", key)
}

func isValidEvent(event string) bool {
	switch event {
	case string(message.HdrTypeIntrospect):
		return true
	default:
		return false
	}
}

// TODO Convert in a method for schema.SchemaMap
func validateMessage(schemas schema.TopicSchemas, msg *kafka.Message) error {
	var (
		err   error
		event *kafka.Header
		sm    schema.SchemaMap
		s     *schema.Schema
	)
	if len(schemas) == 0 {
		return fmt.Errorf("schemas is empty")
	}
	if msg == nil {
		return fmt.Errorf("msg cannot be nil")
	}
	// TODO Check to be removed when refactor to 1 topic - 1 schema
	if event, err = getHeader(msg, string(message.HdrType)); err != nil {
		return fmt.Errorf("header '%s' not found: %s", string(message.HdrType), err.Error())
	}
	if !isValidEvent(string(event.Value)) {
		return fmt.Errorf("event not valid: %v", event)
	}
	if msg.TopicPartition.Topic == nil {
		return fmt.Errorf("topic cannot be nil")
	}
	topic := *msg.TopicPartition.Topic
	if sm = schemas.GetSchemaMap(topic); sm == nil {
		return fmt.Errorf("topic '%s' not found in schema mapping", topic)
	}
	if s = sm.GetSchema(string(event.Value)); s == nil {
		return fmt.Errorf("schema '%s' not found in schema mapping", string(event.Value))
	}

	return s.ValidateBytes(msg.Value)
}

func NewConsumerEventLoop(consumer *kafka.Consumer, handler Eventable) func() {
	var (
		err     error
		msg     *kafka.Message
		schemas schema.TopicSchemas
		sigchan chan os.Signal
	)
	if schemas, err = schema.LoadSchemas(); err != nil {
		panic(err)
	}

	return func() {
		log.Logger.Info().Msg("Setting up signals SIGINT and SIGTERM")
		sigchan = make(chan os.Signal, 1)
		signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)

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
					case sig := <-sigchan:
						log.Logger.Info().Msgf("Caught signal %v: terminating\n", sig)
						return
					default:
					}

					continue
				}
				break
			}

			if err = processConsumedMessage(schemas, msg, handler); err != nil {
				logEventMessageError(msg, err)
				continue
			}
		}
	}
}

func processConsumedMessage(schemas schema.TopicSchemas, msg *kafka.Message, handler Eventable) error {
	var err error

	if schemas == nil || msg == nil || handler == nil {
		return fmt.Errorf("schemas, msg or handler is nil")
	}
	if msg.TopicPartition.Topic == nil {
		return fmt.Errorf("Topic cannot be nil")
	}

	// TODO In the future remove the usage of this 'TopicTranslationConfig' global variable
	// Map the real topic to the internal topic as they could differ
	internalTopic := TopicTranslationConfig.GetInternal(*msg.TopicPartition.Topic)
	if internalTopic == "" {
		return fmt.Errorf("Topic maping not found for: %s", *msg.TopicPartition.Topic)
	}
	log.Info().
		Str("Topic name", *msg.TopicPartition.Topic).
		Str("Requested topic name", internalTopic).
		Msg("Topic mapping")
	*msg.TopicPartition.Topic = internalTopic
	logEventMessageInfo(msg, "Consuming message")

	if err = validateMessage(schemas, msg); err != nil {
		return err
	}

	// Dispatch message
	if err = handler.OnMessage(msg); err != nil {
		return err
	}
	return nil
}
