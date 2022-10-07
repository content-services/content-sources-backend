package event

import (
	"encoding/json"
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/openlyinc/pointy"
	"github.com/rs/zerolog/log"
)

// https://github.com/edenhill/librdkafka/blob/master/CONFIGURATION.md

func NewProducer(config *KafkaConfig) (*kafka.Producer, error) {
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

// TODO Add Producible interface and add this function as a method
// TODO Add Consumible interface and add Consume function as a method
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

	if marshalledValue, err = json.Marshal(value); err != nil {
		return err
	}

	realTopic := TopicTranslationConfig.GetReal(topic)
	if realTopic == "" {
		return fmt.Errorf("Topic translation failed for topic: %s", topic)
	}
	log.Info().
		Str("Requested topic name", topic).
		Str("Topic name", realTopic).
		Msg("Topic mapping")

	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     pointy.String(realTopic),
			Partition: kafka.PartitionAny,
		},
		Value: marshalledValue,
		Key:   []byte(key),
	}

	msg.Headers = append(msg.Headers, headers...)

	logEventMessageInfo(msg, "Producing message")
	return producer.Produce(msg, nil)
}
