package producer

import (
	"fmt"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/event/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewProducer(t *testing.T) {
	var (
		err    error
		result *kafka.Producer
	)

	type TestCase struct {
		Name     string
		Given    *event.KafkaConfig
		Expected error
	}
	var testCases []TestCase = []TestCase{
		{
			Name:     "force error by nil config",
			Given:    nil,
			Expected: fmt.Errorf("config cannot be nil"),
		},
		{
			Name: "force error when NewProducer fails",
			Given: &event.KafkaConfig{
				Bootstrap: struct{ Servers string }{
					Servers: "localhost:9092",
				},
				Request: struct {
					Timeout  struct{ Ms int }
					Required struct{ Acks int }
				}{
					Required: struct{ Acks int }{
						Acks: -1,
					},
				},
				Message: struct {
					Send struct{ Max struct{ Retries int } }
				}{
					Send: struct{ Max struct{ Retries int } }{
						Max: struct{ Retries int }{
							Retries: 3,
						},
					},
				},
			},
			Expected: fmt.Errorf("Configuration property \"retry.backoff.ms\" value 0 is outside allowed range 1..300000\n"),
		},
		{
			Name: "force error when creating producer by wrong sasl mechanism",
			Given: &event.KafkaConfig{
				Bootstrap: struct{ Servers string }{
					Servers: "localhost:9092",
				},
				Request: struct {
					Timeout  struct{ Ms int }
					Required struct{ Acks int }
				}{
					Required: struct{ Acks int }{
						Acks: -1,
					},
				},
				Message: struct {
					Send struct{ Max struct{ Retries int } }
				}{
					Send: struct{ Max struct{ Retries int } }{
						Max: struct{ Retries int }{
							Retries: 3,
						},
					},
				},
				Retry: struct{ Backoff struct{ Ms int } }{
					Backoff: struct{ Ms int }{
						Ms: 300,
					},
				},
				Sasl: struct {
					Username  string
					Password  string
					Mechanism string
					Protocol  string
				}{
					Username:  "myuser",
					Password:  "mypassword",
					Mechanism: "SCRAM-SHA-512-BAD_MECHANISM",
					Protocol:  "sasl_plaintext",
				},
			},
			Expected: fmt.Errorf("Unsupported hash function: SCRAM-SHA-512-BAD_MECHANISM (try SCRAM-SHA-512)"),
		},
		{
			Name: "Success result",
			Given: &event.KafkaConfig{
				Bootstrap: struct{ Servers string }{
					Servers: "localhost:9092",
				},
				Request: struct {
					Timeout  struct{ Ms int }
					Required struct{ Acks int }
				}{
					Required: struct{ Acks int }{
						Acks: -1,
					},
				},
				Message: struct {
					Send struct{ Max struct{ Retries int } }
				}{
					Send: struct{ Max struct{ Retries int } }{
						Max: struct{ Retries int }{
							Retries: 3,
						},
					},
				},
				Retry: struct{ Backoff struct{ Ms int } }{
					Backoff: struct{ Ms int }{
						Ms: 300,
					},
				},
			},
			Expected: nil,
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		result, err = NewProducer(testCase.Given)
		if testCase.Expected != nil {
			require.Error(t, err)
			assert.Nil(t, result)
			assert.Equal(t, testCase.Expected.Error(), err.Error())
		} else {
			assert.NoError(t, err)
			assert.NotNil(t, result)
		}
	}
}

type MockProducer struct {
	kafka.Producer
	mock.Mock
}

func (m *MockProducer) Produce(msg *kafka.Message, deliveryChan chan kafka.Event) error {
	args := m.Called(msg, deliveryChan)
	return args.Error(0)
}

func TestProduce(t *testing.T) {
	var (
		err      error
		producer *kafka.Producer
	)

	cfg := helperGetKafkaConfig()
	producer, err = NewProducer(cfg)
	require.NoError(t, err)
	require.NotNil(t, producer)
	defer producer.Close()

	// Initialize the topic translation
	event.TopicTranslationConfig = event.NewTopicTranslationWithDefaults()

	type TestCaseGiven struct {
		Producer *kafka.Producer
		Topic    string
		Key      string
		Value    interface{}
		Headers  []kafka.Header
	}
	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected error
	}

	var testCases = []TestCase{
		{
			Name:     "force error when producer is nil",
			Given:    TestCaseGiven{},
			Expected: fmt.Errorf("producer cannot be nil"),
		},
		{
			Name: "force error when topic is empty",
			Given: TestCaseGiven{
				Producer: producer,
			},
			Expected: fmt.Errorf("topic cannot be an empty string"),
		},
		{
			Name: "force error when key is empty",
			Given: TestCaseGiven{
				Producer: producer,
				Topic:    "SomeTopic",
			},
			Expected: fmt.Errorf("key cannot be an empty string"),
		},
		{
			Name: "force error when value is empty",
			Given: TestCaseGiven{
				Producer: producer,
				Topic:    "SomeTopic",
				Key:      "SomeKey",
			},
			Expected: fmt.Errorf("value cannot be nil"),
		},
		{
			Name: "force error when translating the topic",
			Given: TestCaseGiven{
				Producer: producer,
				Topic:    "AnyWrongTopic",
				Key:      "SomeKey",
				Value:    "test",
			},
			Expected: fmt.Errorf("Topic translation failed for topic: AnyWrongTopic"),
		},
		{
			Name: "Success scenario",
			Given: TestCaseGiven{
				Producer: producer,
				Topic:    schema.TopicIntrospect,
				Key:      "SomeKey",
				Value:    "test",
			},
			Expected: nil,
		},
	}

	// The tests below requires kafka running
	for _, testCase := range testCases {
		t.Log(testCase.Name)
		err = Produce(
			testCase.Given.Producer,
			testCase.Given.Topic,
			testCase.Given.Key,
			testCase.Given.Value,
			testCase.Given.Headers...)
		if testCase.Expected != nil {
			require.Error(t, err)
			assert.Equal(t, testCase.Expected.Error(), err.Error())
		} else {
			assert.NoError(t, err)
		}
	}
}
