package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPathWithString(t *testing.T) {
	type TestCase struct {
		Name   string
		Given  string
		Expect []string
	}
	testCases := []TestCase{
		{
			Name:   "Empty string",
			Given:  "",
			Expect: []string{},
		},
		{
			Name:   "Root path",
			Given:  "/",
			Expect: []string{},
		},
		{
			Name:   "some plain text",
			Given:  "anything",
			Expect: []string{},
		},
		{
			Name:   "Path to internal ping",
			Given:  "/ping",
			Expect: []string{"ping"},
		},
		{
			Name:   "Repositories endpoint without end slash",
			Given:  "/api/content-sources/v1/repositories",
			Expect: []string{"api", "content-sources", "v1", "repositories"},
		},
		{
			Name:   "Repositories endpoint with end slash",
			Given:  "/api/content-sources/v1/repositories/",
			Expect: []string{"api", "content-sources", "v1", "repositories"},
		},
	}
	for _, testCase := range testCases {
		t.Log(testCase.Name)
		result := NewPathWithString(testCase.Given)
		assert.Equal(t, testCase.Expect, []string(result))
	}
}

func TestRemovePrefixes(t *testing.T) {
	type TestCase struct {
		Given  string
		Expect []string
	}
	testCases := []TestCase{
		{
			Given:  "/api/content-sources/v1",
			Expect: []string{},
		},
		{
			Given:  "/beta/api/content-sources/v1",
			Expect: []string{},
		},
		{
			Given:  "/beta/api/content-sources/v1/repositories",
			Expect: []string{"repositories"},
		},
		{
			Given:  "/apielse/content-sources/v1/repositories",
			Expect: []string{},
		},
		{
			Given:  "/api/else/v1/repositories",
			Expect: []string{},
		},
		{
			Given:  "/api/content-sources/else/repositories",
			Expect: []string{},
		},
		{
			Given:  "/api/content-sources/v1/repositories",
			Expect: []string{"repositories"},
		},
		{
			Given:  "/api/content-sources/v1/repositories/",
			Expect: []string{"repositories"},
		},
		{
			Given:  "/api/content-sources/v1/repositories/validation",
			Expect: []string{"repositories", "validation"},
		},
		{
			Given:  "/api/content-sources/v1/repositories/validation/",
			Expect: []string{"repositories", "validation"},
		},
	}
	for _, testCase := range testCases {
		result := NewPathWithString(testCase.Given).RemovePrefixes()
		assert.Equal(t, testCase.Expect, []string(result))
	}
}

func TestStartWithResources(t *testing.T) {
	type TestCaseGiven struct {
		Path      string
		Resources [][]string
	}
	type TestCase struct {
		Given  TestCaseGiven
		Expect bool
	}
	testCases := []TestCase{
		{
			Given: TestCaseGiven{
				Path: "/api/content-sources/v1/repositories",
				Resources: [][]string{
					{"repositories"},
				},
			},
			Expect: true,
		},
		{
			Given: TestCaseGiven{
				Path: "/api/content-sources/v1/repositories",
				Resources: [][]string{
					{"repositories", "validation"},
				},
			},
			Expect: false,
		},
		{
			Given: TestCaseGiven{
				Path: "/api/content-sources/v1/repositories",
				Resources: [][]string{
					{"rpms"},
				},
			},
			Expect: false,
		},
		{
			Given: TestCaseGiven{
				Path: "/api/content-sources/v1/repositories",
				Resources: [][]string{
					{"rpms"},
					{"repositories", "validation"},
				},
			},
			Expect: false,
		},
		{
			Given: TestCaseGiven{
				Path: "/api/content-sources/v1/repositories",
				Resources: [][]string{
					{"rpms"},
					{"repositories", "validation"},
					{"repositories"},
				},
			},
			Expect: true,
		},
	}
	for _, testCase := range testCases {
		target := NewPathWithString(testCase.Given.Path).RemovePrefixes()
		result := target.StartWithResources(testCase.Given.Resources...)
		assert.Equal(t, testCase.Expect, result)
	}
}
