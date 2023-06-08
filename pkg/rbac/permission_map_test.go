package rbac

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPermissionsMap(t *testing.T) {
	var result *PermissionsMap
	require.NotPanics(t, func() {
		result = NewPermissionsMap()
	})
	assert.NotNil(t, result)
}

func TestAdd(t *testing.T) {
	type TestCaseGiven struct {
		Method   string
		Path     string
		Resource Resource
		Verb     Verb
	}
	type TestCaseExpected bool
	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected TestCaseExpected
	}
	testCases := []TestCase{
		{
			Name: "Success case",
			Given: TestCaseGiven{
				Method:   http.MethodGet,
				Path:     "/api/" + application + "/v1/repositories",
				Resource: ResourceRepositories,
				Verb:     RbacVerbRead,
			},
			Expected: true,
		},
		{
			Name: "Empty method",
			Given: TestCaseGiven{
				Method:   "",
				Path:     "/api/" + application + "/v1/repositories",
				Resource: ResourceRepositories,
				Verb:     RbacVerbRead,
			},
			Expected: false,
		},
		{
			Name: "Empty path",
			Given: TestCaseGiven{
				Method:   http.MethodGet,
				Path:     "",
				Resource: ResourceRepositories,
				Verb:     RbacVerbRead,
			},
			Expected: false,
		},
		{
			Name: "Empty resource",
			Given: TestCaseGiven{
				Method:   http.MethodGet,
				Path:     "/api/" + application + "/v1/repositories",
				Resource: ResourceUndefined,
				Verb:     RbacVerbRead,
			},
			Expected: false,
		},
		{
			Name: "Empty verb",
			Given: TestCaseGiven{
				Method:   http.MethodGet,
				Path:     "/api/" + application + "/v1/repositories",
				Resource: ResourceRepositories,
				Verb:     RbacVerbUndefined,
			},
			Expected: false,
		},
		{
			Name: "Resource wildcard",
			Given: TestCaseGiven{
				Method:   http.MethodGet,
				Path:     "/api/" + application + "/v1/repositories",
				Resource: ResourceAny,
				Verb:     RbacVerbRead,
			},
			Expected: false,
		},
		{
			Name: "Verb wildcard",
			Given: TestCaseGiven{
				Method:   http.MethodGet,
				Path:     "/api/" + application + "/v1/repositories",
				Resource: ResourceRepositories,
				Verb:     RbacVerbAny,
			},
			Expected: false,
		},
	}
	for _, testCase := range testCases {
		t.Logf("TestAdd:%s", testCase.Name)
		result := NewPermissionsMap().Add(testCase.Given.Method, testCase.Given.Path, testCase.Given.Resource, testCase.Given.Verb)
		if testCase.Expected {
			assert.NotNil(t, result)
		} else {
			assert.Nil(t, result)
		}
	}

	assert.NotPanics(t, func() {
		result := NewPermissionsMap().
			Add(http.MethodGet, "/repositories", "repositories", "read").
			Add(http.MethodGet, "/rpms", "repositories", "read").
			Add(http.MethodGet, "/repositories", "repositories", "write")
		assert.NotNil(t, result)
	})
}

func TestPermissionWithServicePermissions(t *testing.T) {
	type TestCaseGiven struct {
		Method string
		Path   string
	}
	type TestCaseExpected struct {
		Resource Resource
		Verb     Verb
		Error    error
	}
	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected TestCaseExpected
	}

	testCases := []TestCase{
		{
			Name: "No mapped value",
			Given: TestCaseGiven{
				Method: http.MethodGet,
				Path:   "notexistingmap",
			},
			Expected: TestCaseExpected{
				Resource: ResourceUndefined,
				Verb:     RbacVerbUndefined,
				Error:    fmt.Errorf(""),
			},
		},
		{
			Name: "GET */repositories",
			Given: TestCaseGiven{
				Method: http.MethodGet,
				Path:   "/repositories/",
			},
			Expected: TestCaseExpected{
				Resource: ResourceRepositories,
				Verb:     RbacVerbRead,
				Error:    nil,
			},
		},
		{
			Name: "POST */repository_parameters/validate/",
			Given: TestCaseGiven{
				Method: http.MethodPost,
				Path:   "/repository_parameters/validate/",
			},
			Expected: TestCaseExpected{
				Resource: ResourceRepositories,
				Verb:     RbacVerbWrite,
				Error:    nil,
			},
		},
		{
			Name: "Method no mapped",
			Given: TestCaseGiven{
				Method: "NOTEXISTING",
				Path:   "/repository_parameters/validate",
			},
			Expected: TestCaseExpected{
				Resource: "",
				Verb:     RbacVerbUndefined,
				Error:    fmt.Errorf("no permission found for method=NOTEXISTING and path=/repository_parameters/validate"),
			},
		},
		{
			Name: "Path no mapped",
			Given: TestCaseGiven{
				Method: http.MethodPost,
				Path:   "/rpms/validate",
			},
			Expected: TestCaseExpected{
				Resource: "",
				Verb:     RbacVerbUndefined,
				Error:    fmt.Errorf("no permission found for method=POST and path=/rpms/validate"),
			},
		},
	}
	ServicePermissions = NewPermissionsMap().
		Add(http.MethodGet, "/repositories/", "repositories", "read").
		Add(http.MethodPost, "/repositories/", "repositories", "write").
		Add(http.MethodPost, "/repository_parameters/validate/", "repositories", "write")

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		res, verb, err := ServicePermissions.Permission(testCase.Given.Method, testCase.Given.Path)
		if testCase.Expected.Error == nil {
			require.NoError(t, err)
			assert.Equal(t, testCase.Expected.Resource, res)
			assert.Equal(t, testCase.Expected.Verb, verb)
		} else {
			require.Error(t, err)
			assert.Equal(t, Resource(""), res)
			assert.Equal(t, RbacVerbUndefined, verb)
		}
	}
}
