package message

import (
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

type EventHeaderKey string

const (
	HdrType                 EventHeaderKey = "Type"
	HdrXRhIdentity          EventHeaderKey = "X-Rh-Identity"
	HdrXRhInsightsRequestId EventHeaderKey = "X-Rh-Insights-Request-Id"
)

type Header map[EventHeaderKey]string

const (
	HdrTypeIntrospect = "Introspect"
)

func (h *Header) Set(key EventHeaderKey, value string) {
	var m map[EventHeaderKey]string = *h
	m[key] = value
}

func (h *Header) Get(key EventHeaderKey) (string, error) {
	var m map[EventHeaderKey]string = *h
	if value, ok := m[key]; ok {
		return value, nil
	}
	return "", fmt.Errorf("key '%s' not found", string(key))
}

func (h *Header) GetAll() []kafka.Header {
	var m map[EventHeaderKey]string = *h
	var output []kafka.Header
	for key, value := range m {
		output = append(output, kafka.Header{
			Key:   string(key),
			Value: []byte(value),
		})
	}
	return output
}
