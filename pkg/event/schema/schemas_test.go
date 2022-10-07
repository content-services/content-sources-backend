package schema

import (
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/event/message"
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
			Expected: fmt.Errorf("error validating schema: \"url\" value is required: / = map[uuid:my-uuid]"),
		},
		{
			Name: "success scenario",
			Given: []byte(`{
				"uuid": "my-uuid",
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
