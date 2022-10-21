package event

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/content-services/content-sources-backend/pkg/event/schema"
	"github.com/openlyinc/pointy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

func TestLogEventMessageInfo(t *testing.T) {
	oldLogger := log.Logger
	defer func() { log.Logger = oldLogger }()
	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf)

	buf.Reset()
	logEventMessageInfo(nil, "")
	assert.Equal(t, "", buf.String())

	buf.Reset()
	logEventMessageInfo(
		&kafka.Message{
			Headers: []kafka.Header{
				{
					Key:   string(message.HdrType),
					Value: []byte(message.HdrTypeIntrospect),
				},
			},
			TopicPartition: kafka.TopicPartition{
				Topic: pointy.String(schema.TopicIntrospect),
			},
			Value: []byte(`{"uuid":"5e759032-5124-11ed-a029-482ae3863d30","url":"https://example.test"}`),
		}, "")
	assert.Equal(t, "", buf.String())

	buf.Reset()
	logEventMessageInfo(nil, "Any additional message")
	assert.Equal(t, "", buf.String())

	buf.Reset()
	logEventMessageInfo(
		&kafka.Message{
			Headers: []kafka.Header{
				{
					Key:   string(message.HdrType),
					Value: []byte(message.HdrTypeIntrospect),
				},
			},
			TopicPartition: kafka.TopicPartition{
				Topic: pointy.String(schema.TopicIntrospect),
			},
			Value: []byte(`{"uuid":"c810053e-512b-11ed-9d9c-482ae3863d30","url":"https://example.test"}`),
		},
		"Some message",
	)
	assert.Equal(t, "{\"level\":\"info\",\"Topic\":\"platform.content-sources.introspect\",\"Key\":\"\",\"Headers\":\"{Type: Introspect}\",\"message\":\"Some message\"}\n", buf.String())
}

func TestLogEventMessageError(t *testing.T) {
	oldLogger := log.Logger
	defer func() { log.Logger = oldLogger }()
	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf)

	logEventMessageError(nil, nil)
	assert.Equal(t, "", buf.String())

	buf.Reset()
	logEventMessageError(
		&kafka.Message{
			Headers: []kafka.Header{
				{
					Key:   string(message.HdrType),
					Value: []byte(message.HdrTypeIntrospect),
				},
			},
			TopicPartition: kafka.TopicPartition{
				Topic: pointy.String(schema.TopicIntrospect),
			},
			// TODO Complete schema definition for more accurate validation
			Value: []byte(`{"uuid":"","url":""}`),
		}, nil)
	assert.Equal(t, "", buf.String())

	buf.Reset()
	logEventMessageError(nil, fmt.Errorf("Any error message"))
	assert.Equal(t, "", buf.String())

	buf.Reset()
	logEventMessageError(
		&kafka.Message{
			Headers: []kafka.Header{
				{
					Key:   string(message.HdrType),
					Value: []byte(message.HdrTypeIntrospect),
				},
			},
			TopicPartition: kafka.TopicPartition{
				Topic: pointy.String(schema.TopicIntrospect),
			},
			// TODO Complete schema definition for more accurate validation
			Value: []byte(`{"uuid":"","url":""}`),
		},
		fmt.Errorf("Any error message"),
	)
	assert.Equal(t, "{\"level\":\"error\",\"message\":\"error processing event message: headers=[Type=\\\"Introspect\\\"]; payload={\\\"uuid\\\":\\\"\\\",\\\"url\\\":\\\"\\\"}: Any error message\"}\n", buf.String())
}

func TestGetHeaderString(t *testing.T) {
	// func getHeaderString(headers []kafka.Header) string {
	// 	var output []string = make([]string, len(headers))
	// 	for i, header := range headers {
	// 		output[i] = fmt.Sprintf("%s: %s", header.Key, string(header.Value))
	// 	}
	// 	return fmt.Sprintf("{%s}", strings.Join(output, "\n"))
	// }
	var result string

	headers := [][]kafka.Header{
		{},
		{
			kafka.Header{
				Key:   "Header1",
				Value: []byte("Value1"),
			},
		},
		{
			kafka.Header{
				Key:   "Header1",
				Value: []byte("Value1"),
			},
			kafka.Header{
				Key:   "Header2",
				Value: []byte("Value2"),
			},
		},
	}

	result = getHeaderString(headers[0])
	assert.Equal(t, "{}", result)

	result = getHeaderString(headers[1])
	assert.Equal(t, "{Header1: Value1}", result)

	result = getHeaderString(headers[2])
	assert.Equal(t, "{Header1: Value1, Header2: Value2}", result)
}
