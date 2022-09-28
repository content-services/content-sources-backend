package event

import (
	"encoding/json"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/openlyinc/pointy"
)

// https://github.com/edenhill/librdkafka/blob/master/CONFIGURATION.md

func NewProducer(config *config.Configuration) (*kafka.Producer, error) {
	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers":        config.Kafka.Bootstrap.Servers,
		"request.required.acks":    config.Kafka.Request.Required.Acks,
		"message.send.max.retries": config.Kafka.Message.Send.Max.Retries,
		"retry.backoff.ms":         config.Kafka.Retry.Backoff.Ms,
	}
	if config.Kafka.Sasl.Username != "" {
		_ = kafkaConfigMap.SetKey("sasl.username", config.Kafka.Sasl.Username)
		_ = kafkaConfigMap.SetKey("sasl.password", config.Kafka.Sasl.Password)
		_ = kafkaConfigMap.SetKey("sasl.mechanism", config.Kafka.Sasl.Mechanism)
		_ = kafkaConfigMap.SetKey("security.protocol", config.Kafka.Sasl.Protocol)
		_ = kafkaConfigMap.SetKey("ssl.ca.location", config.Kafka.Capath)
	}
	producer, err := kafka.NewProducer(kafkaConfigMap)
	if err != nil {
		return nil, err
	}

	return producer, nil
}

// TODO Add Producible interface and add this function as a method
// TODO Add Consumible intarface and add Consume function as a method
func Produce(producer *kafka.Producer, topic string, key string, value interface{}, headers ...kafka.Header) error {
	marshalledValue, err := json.Marshal(value)
	if err != nil {
		return err
	}

	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     pointy.String(topic),
			Partition: kafka.PartitionAny,
		},
		Value: marshalledValue,
		Key:   []byte(key),
	}

	msg.Headers = append(msg.Headers, headers...)

	return producer.Produce(msg, nil)
}
