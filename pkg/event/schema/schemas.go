package schema

// Tools related with schema validation
// http://json-schema.org
// http://json-schema.org/latest/json-schema-core.html
// http://json-schema.org/latest/json-schema-validation.html
//
// Fancy online tools
// https://www.liquid-technologies.com/online-json-to-schema-converter
// https://app.quicktype.io/
//
// To generate the code from the schemas is used:
// https://github.com/atombender/go-jsonschema
//
// To validate the schemas against a data structure is
// used: https://github.com/qri-io/jsonschema
//
// Regular expression tools, useful when pattern attribute could be used:
// https://www.regexpal.com
// https://regex101.com/
//
// Just to mention that 'pattern' does not work into the validation.
// The regular expressions are different in ECMA Script and GoLang,
// but maybe this library could make the differences work:
// https://github.com/dlclark/regexp2#ecmascript-compatibility-mode
// anyway that would be something to be added as a PR to some of the
// above libraries; however, into the above library it is mentioned
// that it deal with ASCII, but not very well with unicode. That
// is a concern, more when using for message validation that could
// be a source of bugs and vulnerabilities.
//

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/qri-io/jsonschema"
)

const (
	SchemaIntrospectKey = "Introspect"

	// Topic constants
	TopicIntrospect = "platform.content-sources.introspect"
)

var AllowedTopics = []string{
	TopicIntrospect,
}

// https://pkg.go.dev/embed

// Embed message schemas here

//go:embed "introspectRequest.message.json"
var schemaMessageIntrospect string

var (
	schemaKey2JsonSpec = map[string]map[string]string{
		TopicIntrospect: {
			SchemaIntrospectKey: schemaMessageIntrospect,
		},
	}
)

type Schema jsonschema.Schema

type SchemaMap map[string](*Schema)

type TopicSchemas map[string]SchemaMap

// GetSchemaMap return a SchemaMap associated to one topic.
// topic the topic which want to retrieve the SchemaMap.
// Return a SchemaMap associated to the topic or nil if the
//   topic is not found.
func (ts *TopicSchemas) GetSchemaMap(topic string) SchemaMap {
	if value, ok := (*ts)[topic]; ok {
		return value
	}
	return nil
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

// ValidateMessage check the msg is accomplish the schema defined for it.
// msg is a reference to a kafka.Message struct.
// Return nil if the check is success else an error reference is filled.
func (ts *TopicSchemas) ValidateMessage(msg *kafka.Message) error {
	var (
		err     error
		event   *kafka.Header
		sm      SchemaMap
		s       *Schema
		schemas map[string]SchemaMap
	)
	schemas = *ts
	if len(schemas) == 0 {
		return fmt.Errorf("schemas is empty")
	}
	if msg == nil {
		return fmt.Errorf("msg cannot be nil")
	}
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
	if sm = ts.GetSchemaMap(topic); sm == nil {
		return fmt.Errorf("topic '%s' not found in schema mapping", topic)
	}
	if s = sm.GetSchema(string(event.Value)); s == nil {
		return fmt.Errorf("schema '%s' not found in schema mapping", string(event.Value))
	}

	return s.ValidateBytes(msg.Value)
}

// GetSchema retrieve a *Schema associated to the indicated event.
// event is the name of the event we want to retrieve.
//   See pkg/event/message/headers.go
// Return the reference to the Schema data or nil if the event
//   is not found.
func (sm *SchemaMap) GetSchema(event string) *Schema {
	if value, ok := (*sm)[event]; ok {
		return value
	}
	return nil
}

// LoadSchemas unmarshall all the embedded schemas and
//   return all them in the output schemas variable.
//   See also LoadSchemaFromString.
// schemas is a hashmap map[string]*gojsonschema.Schema that
//   can be used to immediately validate schemas against
//   unmarshalled schemas.
// Return the resulting list of schemas, or nil if an
//   an error happens.
func LoadSchemas() (TopicSchemas, error) {
	var (
		output TopicSchemas = TopicSchemas{}
		schema *Schema
		err    error
	)

	for topic, schemas := range schemaKey2JsonSpec {
		output[topic] = make(map[string]*Schema, 2)
		for key, schemaSerialized := range schemas {
			if schema, err = LoadSchemaFromString(schemaSerialized); err != nil {
				return nil, fmt.Errorf("error unmarshalling for topic '%s' schema '%s': %w", topic, key, err)
			}
			output[topic][key] = schema
		}
	}

	return output, nil
}

// LoadSchemaFromString unmarshall a schema from
//   its string representation in json format.
// schemas is a string representation in json format
//   for gojsonschema.Schema.
// Return the resulting list of schemas, or nil if an
//   an error happens.
func LoadSchemaFromString(schema string) (*Schema, error) {
	var err error
	var output *Schema
	rs := &jsonschema.Schema{}
	if err = json.Unmarshal([]byte(schema), rs); err != nil {
		return nil, fmt.Errorf("error unmarshalling schema '%s': %w", schema, err)
	}
	output = (*Schema)(rs)
	return output, nil
}

// ValidateBytes validate that a slice of bytes which
//   represent an event message match the Schema.
// data is a byte slice with the event message representation.
// Return nil if check is success, else a filled error.
func (s *Schema) ValidateBytes(data []byte) error {
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}
	jsSchema := (*jsonschema.Schema)(s)
	parseErrs, err := jsSchema.ValidateBytes(context.Background(), data)
	if err != nil {
		return err
	}
	if len(parseErrs) == 0 {
		return nil
	}

	return s.prepareParseErrorList(parseErrs)
}

// Validate check that data interface accomplish the Schema.
// data is any type, it cannot be nil.
// Return nil if the check is success, else a filled error.
func (s *Schema) Validate(data interface{}) error {
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}
	jsSchema := (*jsonschema.Schema)(s)
	vs := jsSchema.Validate(context.Background(), data)
	if len(*vs.Errs) == 0 {
		return nil
	}
	return s.prepareParseErrorList(*vs.Errs)
}

func (s *Schema) prepareParseErrorList(parseErrs []jsonschema.KeyError) error {
	var errorList []string = []string{}
	for _, item := range parseErrs {
		errorList = append(errorList, fmt.Sprintf(
			"%s: %s = %s",
			item.Message,
			item.PropertyPath,
			item.InvalidValue,
		))
	}
	return fmt.Errorf("error validating schema: %s", strings.Join(errorList, ", "))
}
