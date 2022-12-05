package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveEndChar(t *testing.T) {
	type TestCaseGiven struct {
		Source string
		Suffix string
	}
	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected string
	}

	var testCases = []TestCase{
		{
			Name: "Normal success",
			Given: TestCaseGiven{
				Source: "https://www.example.test/",
				Suffix: "/",
			},
			Expected: "https://www.example.test",
		},
		{
			Name: "Several suffixes",
			Given: TestCaseGiven{
				Source: "https://www.example.test//////",
				Suffix: "/",
			},
			Expected: "https://www.example.test",
		},
		{
			Name: "Empty source string",
			Given: TestCaseGiven{
				Source: "",
				Suffix: "/",
			},
			Expected: "",
		},
		{
			Name: "Empty resulting string",
			Given: TestCaseGiven{
				Source: "//////",
				Suffix: "/",
			},
			Expected: "",
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		var result string
		assert.NotPanics(t, func() {
			result = removeEndSuffix(testCase.Given.Source, testCase.Given.Suffix)
		})
		assert.Equal(t, testCase.Expected, result)
	}
}
