package producer

import (
	"encoding/json"
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/openlyinc/pointy"
	"github.com/rs/zerolog/log"
)

// See: https://github.com/edenhill/librdkafka/blob/master/CONFIGURATION.md
// To read a full description of the client library configuration

// NewProducer This is used by composition for all the producers
//   into this package.
// config Reference to the KafkaConfig struct that will be used to
//   define the client library configuration.
// Return a reference to the kafka.Producer, which is returned by
//   the client library used under the hood and nil when success,
//   else it returns a nil reference to the producer and an error
//   with the information of the situation.
func NewProducer(config *event.KafkaConfig) (*kafka.Producer, error) {
	var (
		err      error
		producer *kafka.Producer
	)
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers":        config.Bootstrap.Servers,
		"request.required.acks":    config.Request.Required.Acks,
		"message.send.max.retries": config.Message.Send.Max.Retries,
		"retry.backoff.ms":         config.Retry.Backoff.Ms,
	}
	if config.Sasl.Username != "" {
		_ = kafkaConfigMap.SetKey("sasl.username", config.Sasl.Username)
		_ = kafkaConfigMap.SetKey("sasl.password", config.Sasl.Password)
		_ = kafkaConfigMap.SetKey("sasl.mechanism", config.Sasl.Mechanism)
		_ = kafkaConfigMap.SetKey("security.protocol", config.Sasl.Protocol)
		_ = kafkaConfigMap.SetKey("ssl.ca.location", config.Capath)
	}
	if producer, err = kafka.NewProducer(kafkaConfigMap); err != nil {
		return nil, err
	}
	return producer, nil
}

// Produce a kafka message given a producer, topic, key, headers, and
//   the value with the message information. The message is serialized
//   as a json document.
// producer is the reference to a kafka.Producer (see NewProducer).
// topic is the name of the topic where the message will be published
// key is the key to be assigned to the message; this value is important
//   because it is used to distribute the load in kafka; Actually it is
//   checked that it is not an empty string, but if we assign always the
//   same value, we avoid the messages to be distributed properly between
//   the partitions. Internally it makes something like:
//   target_partition = hash(key) % number_of_partitions
// value it could be any structure; the message structures are defined
//   at pkg/event/message and are self-generated from the schema defined
//   at pkg/event/schema package.
// headers is a variadic argument that contain all the headers to be added
//   to the message.
// Return nil if the message is registered to be produced, which is not the
//   same as the message is added to the kafka topic queue. If some error
//   happens before register the message to be produced, an error is returned
//   with information about the situation.
func Produce(producer *kafka.Producer, topic string, key string, value interface{}, headers ...kafka.Header) error {
	var (
		err             error
		marshalledValue []byte
	)

	if producer == nil {
		return fmt.Errorf("producer cannot be nil")
	}
	if topic == "" {
		return fmt.Errorf("topic cannot be an empty string")
	}
	// key is used to distribute the load between the partitions
	if key == "" {
		return fmt.Errorf("key cannot be an empty string")
	}
	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	realTopic := event.TopicTranslationConfig.GetReal(topic)
	if realTopic == "" {
		return fmt.Errorf("Topic translation failed for topic: %s", topic)
	}
	log.Info().
		Str("Requested topic name", topic).
		Str("Topic name", realTopic).
		Msg("Topic mapping")

	// TODO Validate the value to serialize with the schema
	//      The method is not working as expected, it needs
	//      to fix it first

	if marshalledValue, err = json.Marshal(value); err != nil {
		return err
	}

	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     pointy.String(realTopic),
			Partition: kafka.PartitionAny,
		},
		Value: marshalledValue,
		Key:   []byte(key),
	}

	msg.Headers = append(msg.Headers, headers...)

	// logEventMessageInfo(msg, "Producing message")
	return producer.Produce(msg, nil)
}
