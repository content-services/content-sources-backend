package event

import (
	"fmt"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/content-services/content-sources-backend/pkg/event/schema"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewConsumer(t *testing.T) {
	var (
		consumer *kafka.Consumer
		err      error
	)

	type TestCase struct {
		Given    *KafkaConfig
		Expected error
	}

	testCases := []TestCase{
		// When config is nil
		{
			Given:    nil,
			Expected: fmt.Errorf("config cannot be nil"),
		},
		// Failing kafka.NewConsumer
		{
			Given: &KafkaConfig{
				Bootstrap: struct{ Servers string }{},
			},
			Expected: fmt.Errorf("Configuration property \"auto.offset.reset\" cannot be set to empty value"),
		},
		// Fail SubscribeTopics because unknown group
		{
			Given: &KafkaConfig{
				Bootstrap: struct{ Servers string }{
					Servers: "localhost:9092",
				},
				Auto: struct {
					Offset struct{ Reset string }
					Commit struct{ Interval struct{ Ms int } }
				}{
					Offset: struct{ Reset string }{
						Reset: "latest",
					},
				},
			},
			Expected: fmt.Errorf("Local: Unknown group"),
		},
		// Fail SubscribeTopics because unknown group
		{
			Given: &KafkaConfig{
				Bootstrap: struct{ Servers string }{
					Servers: "localhost:9092",
				},
				Auto: struct {
					Offset struct{ Reset string }
					Commit struct{ Interval struct{ Ms int } }
				}{
					Offset: struct{ Reset string }{
						Reset: "latest",
					},
				},
			},
			Expected: fmt.Errorf("Local: Unknown group"),
		},
		// Success return
		{
			Given: &KafkaConfig{
				Bootstrap: struct{ Servers string }{
					Servers: "localhost:9092",
				},
				Auto: struct {
					Offset struct{ Reset string }
					Commit struct{ Interval struct{ Ms int } }
				}{
					Offset: struct{ Reset string }{
						Reset: "latest",
					},
				},
				Group: struct{ Id string }{
					Id: "main",
				},
				Topics: []string{
					schema.TopicIntrospect,
				},
			},
			Expected: nil,
		},
		// Success return with sasl
		{
			Given: &KafkaConfig{
				Bootstrap: struct{ Servers string }{
					Servers: "localhost:9092",
				},
				Auto: struct {
					Offset struct{ Reset string }
					Commit struct{ Interval struct{ Ms int } }
				}{
					Offset: struct{ Reset string }{
						Reset: "latest",
					},
				},
				Group: struct{ Id string }{
					Id: "main",
				},
				Topics: []string{
					schema.TopicIntrospect,
				},
				Sasl: struct {
					Username  string
					Password  string
					Mechanism string
					Protocol  string
				}{
					Username:  "myusername",
					Password:  "mypassword",
					Mechanism: "SCRAM-SHA-512",
					Protocol:  "sasl_plaintext",
				},
				Capath: "",
			},
			Expected: nil,
		},
	}

	for _, testCase := range testCases {
		consumer, err = NewConsumer(testCase.Given)
		if testCase.Expected != nil {
			assert.Nil(t, consumer)
			require.Error(t, err)
			assert.Equal(t, testCase.Expected.Error(), err.Error())
		} else {
			assert.NotNil(t, consumer)
			assert.NoError(t, err)
			if err != nil {
				assert.Equal(t, "", err.Error())
			}
		}
	}
}

func TestGetHeader(t *testing.T) {
	var (
		msg    *kafka.Message
		header *kafka.Header
		err    error
	)
	msg = &kafka.Message{
		Headers: []kafka.Header{
			{
				Key:   "key1",
				Value: []byte("value1"),
			},
		},
	}

	// nil msg
	header, err = getHeader(nil, "")
	require.Error(t, err)
	assert.Nil(t, header)
	assert.Equal(t, "msg is nil", err.Error())

	// key is empty
	header, err = getHeader(msg, "")
	require.Error(t, err)
	assert.Nil(t, header)
	assert.Equal(t, "key is empty", err.Error())

	// a non existing key
	header, err = getHeader(msg, "nonexisting")
	require.Error(t, err)
	assert.Nil(t, header)
	assert.Equal(t, "could not find 'nonexisting' in message header", err.Error())

	// an existing key
	header, err = getHeader(msg, "key1")
	assert.NoError(t, err)
	require.NotNil(t, header)
	assert.Equal(t, "value1", string(header.Value))
}

func TestIsValidEvent(t *testing.T) {
	assert.True(t, isValidEvent(message.HdrTypeIntrospect))
	assert.False(t, isValidEvent("AnyOtherKey"))
}

func TestValidateMessage(t *testing.T) {
	type TestCaseGiven struct {
		Schemas schema.TopicSchemas
		Message *kafka.Message
	}
	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected error
	}

	schemas, err := schema.LoadSchemas()
	require.NoError(t, err)

	testCases := []TestCase{
		// nil schemas
		{
			Name: "force error when schemas is nil",
			Given: TestCaseGiven{
				Schemas: nil,
				Message: nil,
			},
			Expected: fmt.Errorf("schemas is empty"),
		},
		// nil message
		{
			Name: "force error when message is nil",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: nil,
			},
			Expected: fmt.Errorf("msg cannot be nil"),
		},
		// No 'Type' header
		{
			Name: "force error when no 'Type' header",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: &kafka.Message{
					Headers: []kafka.Header{},
				},
			},
			Expected: fmt.Errorf("header '%s' not found: could not find '%s' in message header", string(message.HdrType), string(message.HdrType)),
		},
		// It is not a valid event
		{
			Name: "force error with an invalid event",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: &kafka.Message{
					Headers: []kafka.Header{
						{
							Key:   string(message.HdrType),
							Value: []byte("AnEventThatDoesNotExist"),
						},
					},
				},
			},
			Expected: fmt.Errorf("event not valid: Type=\"AnEventThatDoesNotExist\""),
		},
		// No Topic
		{
			Name: "force error when no topic is specified",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: &kafka.Message{
					Headers: []kafka.Header{
						{
							Key:   string(message.HdrType),
							Value: []byte(message.HdrTypeIntrospect),
						},
					},
					TopicPartition: kafka.TopicPartition{
						Topic: nil,
					},
				},
			},
			Expected: fmt.Errorf("topic cannot be nil"),
		},
		// Topic not found
		{
			Name: "force error when the topic is not found",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: &kafka.Message{
					Headers: []kafka.Header{
						{
							Key:   string(message.HdrType),
							Value: []byte(message.HdrTypeIntrospect),
						},
					},
					TopicPartition: kafka.TopicPartition{
						Topic: pointy.String("ATopicThatDoesNotExist"),
					},
				},
			},
			Expected: fmt.Errorf("topic '%s' not found in schema mapping", "ATopicThatDoesNotExist"),
		},
		// Validate bytes return false
		{
			Name: "force error when schema validation fails",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: &kafka.Message{
					Headers: []kafka.Header{
						{
							Key:   string(message.HdrType),
							Value: []byte(message.HdrTypeIntrospect),
						},
					},
					TopicPartition: kafka.TopicPartition{
						Topic: pointy.String(schema.TopicIntrospect),
					},
					Value: []byte(`{}`),
				},
			},
			Expected: fmt.Errorf("error validating schema: \"uuid\" value is required: / = map[], \"url\" value is required: / = map[]"),
		},
		// Validate bytes return true
		{
			Name: "force error when schema validation fails",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: &kafka.Message{
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
					Value: []byte(`{
						"uuid":"",
						"url":""
					}`),
				},
			},
			Expected: nil,
		},
	}

	for _, testCase := range testCases {
		t.Logf("Testing case '%s'", testCase.Name)
		err := validateMessage(testCase.Given.Schemas, testCase.Given.Message)
		if testCase.Expected != nil {
			require.Error(t, err)
			require.Equal(t, testCase.Expected.Error(), err.Error())
		} else {
			assert.NoError(t, err)
		}
	}
}

type MockEventable struct {
	mock.Mock
}

func (m *MockEventable) OnMessage(msg *kafka.Message) error {
	args := m.MethodCalled("OnMessage", msg)
	return args.Error(0)
}

func TestProcessConsumedMessage(t *testing.T) {
	type TestCaseGiven struct {
		Schemas schema.TopicSchemas
		Message *kafka.Message
		Handler Eventable
	}
	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected error
	}

	msgValid := &kafka.Message{
		Key: []byte("this-is-my-key"),
		TopicPartition: kafka.TopicPartition{
			Topic: pointy.String(schema.TopicIntrospect),
		},
		Headers: []kafka.Header{
			{
				Key:   "Type",
				Value: []byte(message.HdrTypeIntrospect),
			},
		},
		Value: []byte(`{"uuid":"my-uuid","url":"https://packages.cloud.google.com/yum/repos/google-compute-engine-el8-x86_64-stable"}`),
	}
	msgNoValid := &kafka.Message{
		Key: []byte("this-is-my-key"),
		TopicPartition: kafka.TopicPartition{
			Topic: pointy.String(schema.TopicIntrospect),
		},
		Headers: []kafka.Header{
			{
				Key:   string(message.HdrType),
				Value: []byte(message.HdrTypeIntrospect),
			},
		},
		Value: []byte(`{}`),
	}
	mockOnMessageFailure := &MockEventable{}
	mockOnMessageFailure.On("OnMessage", msgValid).Return(fmt.Errorf("Error in handler"))
	mockOnMessageSuccess := &MockEventable{}
	mockOnMessageSuccess.On("OnMessage", msgValid).Return(nil)

	schemas, err := schema.LoadSchemas()
	require.NoError(t, err)
	require.NotNil(t, schemas)

	testCases := []TestCase{
		// nil arguments return error
		{
			Name: "force error for nil arguments",
			Given: TestCaseGiven{
				Schemas: nil,
				Message: nil,
				Handler: nil,
			},
			Expected: fmt.Errorf("schemas, msg or handler is nil"),
		},
		// nil topic
		{
			Name: "force error when topic is nil",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: &kafka.Message{
					TopicPartition: kafka.TopicPartition{
						Topic: nil,
					},
				},
				Handler: mockOnMessageFailure,
			},
			Expected: fmt.Errorf("Topic cannot be nil"),
		},
		// Wrong topic
		{
			Name: "force error when topic does not exist",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: &kafka.Message{
					TopicPartition: kafka.TopicPartition{
						Topic: pointy.String("AnyNonExistingTopic"),
					},
				},
				Handler: mockOnMessageFailure,
			},
			Expected: fmt.Errorf("Topic maping not found for: AnyNonExistingTopic"),
		},
		// Invalid message
		{
			Name: "force error when message is not validated",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: &kafka.Message{
					TopicPartition: kafka.TopicPartition{
						Topic: pointy.String(schema.TopicIntrospect),
					},
				},
				Handler: mockOnMessageFailure,
			},
			Expected: fmt.Errorf("header 'Type' not found: could not find 'Type' in message header"),
		},
		// Error validating message schema
		{
			Name: "force error when validating message schema",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: msgNoValid,
				Handler: mockOnMessageFailure,
			},
			Expected: fmt.Errorf("error validating schema: \"uuid\" value is required: / = map[], \"url\" value is required: / = map[]"),
		},
		// Valid message but failure on handler
		{
			Name: "force error when the handler return error",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: msgValid,
				Handler: mockOnMessageFailure,
			},
			Expected: fmt.Errorf("Error in handler"),
		},
		// Valid message handled
		{
			Name: "success case where the message is handled",
			Given: TestCaseGiven{
				Schemas: schemas,
				Message: msgValid,
				Handler: mockOnMessageSuccess,
			},
			Expected: nil,
		},
	}

	TopicTranslationConfig = NewTopicTranslationWithDefaults()

	for _, testCase := range testCases {
		t.Logf("Testing case: '%s'", testCase.Name)
		result := processConsumedMessage(
			testCase.Given.Schemas,
			testCase.Given.Message,
			testCase.Given.Handler)
		if testCase.Expected != nil {
			require.Error(t, result)
			assert.Equal(t, testCase.Expected.Error(), result.Error())
		} else {
			assert.NoError(t, result)
		}
	}
}
