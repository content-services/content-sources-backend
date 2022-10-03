package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadEnv(t *testing.T) {
	const existingKey = "EXISTING_KEY"
	const existingValue = "existing"
	const unexistingKey = "UNEXISTING_KEY"
	const defaultValue = "default"
	type TestCaseGiven struct {
		Key          string
		DefaultValue string
	}
	type TestCaseExpected string
	type TestCase struct {
		Given    TestCaseGiven
		Expected TestCaseExpected
	}

	var testCases = []TestCase{
		{
			Given: TestCaseGiven{
				Key:          existingKey,
				DefaultValue: defaultValue,
			},
			Expected: existingValue,
		},
		{
			Given: TestCaseGiven{
				Key:          unexistingKey,
				DefaultValue: defaultValue,
			},
			Expected: defaultValue,
		},
	}

	os.Unsetenv(unexistingKey)
	os.Setenv(existingKey, existingValue)

	for _, testCase := range testCases {
		result := readEnv(testCase.Given.Key, testCase.Given.DefaultValue)
		assert.Equal(t, string(testCase.Expected), result)
	}
}

// TODO addEventConfigDefaults cannot be fully tested because
//      dependencies that avoid to load a custom clowder file
//      It is loaded in the 'func init' of the package and
//      call an internal private function to load the configuration
