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
// TODO Simplify current mapping so:
//      - One topic has one schema
//      - Remove requirement of 'Type' header.
//      - Reduce mapping nest at one level.

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

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

// //go:embed "header.message.json"
// var schemaHeader string

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

func (ts *TopicSchemas) GetSchemaMap(topic string) {

}

// LoadSchemas unmarshall all the embedded schemas and
//   return all them in the output schemas variable.
// schemas is a hashmap map[string]*gojsonschema.Schema that
//   can be used to immediately validate schemas against
//   unmarshalled schemas.
// Return the resulting list of schemas, or nil if an
// an error happens.
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
				return nil, fmt.Errorf("[LoadSchemas] error unmarshalling for topic '%s' schema '%s': %w", topic, key, err)
			}
			output[topic][key] = schema
		}
	}

	return output, nil
}

func LoadSchemaFromString(schema string) (*Schema, error) {
	var err error
	var output *Schema
	rs := &jsonschema.Schema{}
	if err = json.Unmarshal([]byte(schema), rs); err != nil {
		return nil, fmt.Errorf("[LoadSchemaFromString] error unmarshalling schema '%s': %w", schema, err)
	}
	output = (*Schema)(rs)
	return output, nil
}

func (s *Schema) ValidateBytes(data []byte) error {
	var jsSchema *jsonschema.Schema = (*jsonschema.Schema)(s)
	parseErrs, err := jsSchema.ValidateBytes(context.Background(), data)
	if err != nil {
		return err
	}
	if len(parseErrs) == 0 {
		return nil
	}

	return s.prepareParseErrorList(parseErrs)
}

func (s *Schema) Validate(data interface{}) error {
	var jsSchema *jsonschema.Schema = (*jsonschema.Schema)(s)
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
