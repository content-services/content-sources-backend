package schema

import (
	"fmt"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/openlyinc/pointy"
	"github.com/qri-io/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSchemas(t *testing.T) {
	s, err := LoadSchemas()
	assert.NoError(t, err)
	assert.NotNil(t, s)
}

func TestLoadSchemaFromString(t *testing.T) {
	type TestCase struct {
		Name     string
		Given    string
		Expected error
	}

	testCases := []TestCase{
		{
			Name:     "force error when unmarshalling schema",
			Given:    "{{",
			Expected: fmt.Errorf("error unmarshalling schema '{{': invalid character '{' looking for beginning of object key string"),
		},
		{
			Name:     "success scenario",
			Given:    "{}",
			Expected: nil,
		},
	}

	for _, testCase := range testCases {
		s, err := LoadSchemaFromString(testCase.Given)
		if testCase.Expected != nil {
			require.Error(t, err)
			assert.Equal(t, testCase.Expected.Error(), err.Error())
			assert.Nil(t, s)
		} else {
			assert.NoError(t, err)
			assert.NotNil(t, s)
		}
	}
}

func TestGetSchemaMap(t *testing.T) {
	var (
		ts TopicSchemas
		sm SchemaMap
	)

	ts = TopicSchemas{
		"platform.content-sources.introspect": SchemaMap{
			"Introspect": &Schema{},
		},
	}

	sm = ts.GetSchemaMap("noexist")
	assert.Nil(t, sm)

	sm = ts.GetSchemaMap("platform.content-sources.introspect")
	require.NotNil(t, sm)
}

func TestGetSchema(t *testing.T) {
	var schm *Schema
	sm := SchemaMap(make(map[string]*Schema))
	sm[message.HdrTypeIntrospect] = &Schema{}

	schm = sm.GetSchema(message.HdrTypeIntrospect)
	require.NotNil(t, schm)
	assert.Equal(t, sm[message.HdrTypeIntrospect], schm)

	schm = sm.GetSchema("NotExistingKey")
	assert.Nil(t, schm)
}

func TestValidateBytes(t *testing.T) {
	type TestCase struct {
		Name     string
		Given    []byte
		Expected error
	}
	testCases := []TestCase{
		{
			Name:     "force error when data is nil",
			Given:    nil,
			Expected: fmt.Errorf("data cannot be nil"),
		},
		{
			Name: "force error when no valid data",
			Given: []byte(`{
				"uuid": "my-uuid"
			}`),
			Expected: fmt.Errorf("error validating schema: \"url\" value is required: / = map[uuid:my-uuid], min length of 36 characters required: my-uuid: /uuid = my-uuid"),
		},
		{
			Name: "success scenario",
			Given: []byte(`{
				"uuid": "6c623bb0-511e-11ed-ac56-482ae3863d30",
				"url": "https://example.test"
			}`),
		},
	}

	schema, err := LoadSchemaFromString(schemaMessageIntrospect)
	require.NoError(t, err)
	require.NotNil(t, schema)

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		err = schema.ValidateBytes(testCase.Given)
		if testCase.Expected != nil {
			require.Error(t, err)
			assert.Equal(t, testCase.Expected.Error(), err.Error())
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestValidate(t *testing.T) {
	type TestCase struct {
		Name     string
		Given    interface{}
		Expected error
	}

	testCases := []TestCase{
		{
			Name:     "force error when nil is provided",
			Given:    nil,
			Expected: fmt.Errorf("data cannot be nil"),
		},
		// FIXME This test should return a failure but it is not happening
		// {
		// 	Name: "force failure by using a struct that does not match the schema",
		// 	Given: struct{ AnyField string }{
		// 		AnyField: "AnyValue",
		// 	},
		// 	Expected: fmt.Errorf("data structure does not match the schema"),
		// },
		{
			Name: "success scenario",
			Given: message.IntrospectRequestMessage{
				Uuid: "any-uuid",
				Url:  "https://example.test",
			},
			Expected: nil,
		},
	}

	schema, err := LoadSchemaFromString(schemaMessageIntrospect)
	require.NoError(t, err)
	require.NotNil(t, schema)

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		err = schema.Validate(testCase.Given)
		if testCase.Expected != nil {
			require.Error(t, err)
			assert.Equal(t, testCase.Expected.Error(), err.Error())
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestPrepareParseErrorList(t *testing.T) {
	schema, err := LoadSchemaFromString(`{}`)
	require.NoError(t, err)
	require.NotNil(t, schema)
	err = schema.prepareParseErrorList(
		[]jsonschema.KeyError{
			{
				Message:      "test",
				PropertyPath: "/",
				InvalidValue: "test",
			},
		},
	)
	require.Error(t, err)
	assert.Equal(t, "error validating schema: test: / = test", err.Error())
}

func TestIsValidEvent(t *testing.T) {
	assert.True(t, isValidEvent(message.HdrTypeIntrospect))
	assert.False(t, isValidEvent("AnyOtherKey"))
}

func TestValidateMessage(t *testing.T) {
	type TestCaseGiven struct {
		Schemas TopicSchemas
		Message *kafka.Message
	}
	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected error
	}

	schemas, err := LoadSchemas()
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
						Topic: pointy.String(TopicIntrospect),
					},
					Value: []byte(`{}`),
				},
			},
			Expected: fmt.Errorf("error validating schema: \"uuid\" value is required: / = map[], \"url\" value is required: / = map[]"),
		},
		{
			Name: "force error when message content fails validation",
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
						Topic: pointy.String(TopicIntrospect),
					},
					Value: []byte(`{
						"uuid":"",
						"url":""
					}`),
				},
			},
			Expected: fmt.Errorf("error validating schema: min length of 36 characters required: : /uuid = , min length of 10 characters required: : /url = , invalid uri: uri missing scheme prefix: /url = "),
		},
		// Validate bytes return true
		{
			Name: "Success message schema validation",
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
						Topic: pointy.String(TopicIntrospect),
					},
					Value: []byte(`{
						"uuid":"98777ed4-511b-11ed-bfa5-482ae3863d30",
						"url":"https://example.test"
					}`),
				},
			},
			Expected: nil,
		},
	}

	for _, testCase := range testCases {
		t.Logf("Testing case '%s'", testCase.Name)
		err := testCase.Given.Schemas.ValidateMessage(testCase.Given.Message)
		if testCase.Expected != nil {
			require.Error(t, err)
			require.Equal(t, testCase.Expected.Error(), err.Error())
		} else {
			assert.NoError(t, err)
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
