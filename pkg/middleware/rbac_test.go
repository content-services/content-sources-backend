package middleware

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/client"
	"github.com/content-services/content-sources-backend/pkg/config"
	mocks_client "github.com/content-services/content-sources-backend/pkg/test/mocks/client"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockXRhUserIdentity(t *testing.T, org_id string, accNumber string) string {
	var (
		err       error
		xrhid     identity.XRHID
		jsonBytes []byte
	)
	xrhid.Identity.OrgID = org_id
	xrhid.Identity.AccountNumber = accNumber
	xrhid.Identity.Internal.OrgID = org_id

	jsonBytes, err = json.Marshal(xrhid)
	require.NoError(t, err)

	return b64.StdEncoding.EncodeToString([]byte(jsonBytes))
}

func hasToCallMock(resource string, verb client.RbacVerb, rbacAllowed bool, rbacError error) bool {
	if resource == "" && verb == "" && rbacAllowed == false && rbacError == nil {
		return false
	}
	return true
}

func handleItWorked(c echo.Context) error {
	switch c.Request().Method {
	case http.MethodGet:
		return c.JSON(http.StatusOK, "It worked")
	default:
		return c.JSON(http.StatusOK, nil)
	}
}

func rbacServe(t *testing.T, req *http.Request, resource string, verb client.RbacVerb, skipper echo_middleware.Skipper, rbacAllowed bool, rbacError error, generateIdentity bool) *httptest.ResponseRecorder {
	var (
		xrhid string
		rw    *httptest.ResponseRecorder
	)

	require.NotNil(t, req)

	if generateIdentity {
		xrhid = mockXRhUserIdentity(t, "12345", "12345")
		require.NotEqual(t, "", xrhid)
	}

	mockRbacClient := mocks_client.NewRbac(t)
	require.NotNil(t, mockRbacClient)
	if hasToCallMock(resource, verb, rbacAllowed, rbacError) {
		mockRbacClient.On("Allowed", xrhid, resource, verb).Return(rbacAllowed, rbacError)
	}

	e := echo.New()
	require.NotNil(t, e)

	// Add Rbac middleware
	e.Use(
		NewRbac(Rbac{
			BaseUrl:        config.Get().Clients.RbacBaseUrl,
			Skipper:        skipper,
			PermissionsMap: ServicePermissions,
			Client:         mockRbacClient,
		}),
	)

	// Add a handler to avoid 404
	e.Add(req.Method, req.URL.Path, handleItWorked)

	rw = httptest.NewRecorder()
	require.NotNil(t, rw)

	if generateIdentity {
		req.Header.Set(xrhidHeader, xrhid)
	}

	e.ServeHTTP(rw, req)
	mockRbacClient.AssertExpectations(t)

	return rw
}

func skipperTrue(c echo.Context) bool {
	return true
}

func skipperFalse(c echo.Context) bool {
	return false
}

func TestNewRbacPanics(t *testing.T) {
	var pm *PermissionsMap = ServicePermissions
	var client = mocks_client.NewRbac(t)
	var skipper echo_middleware.Skipper = skipperTrue
	require.Panics(t, func() {
		NewRbac(Rbac{
			BaseUrl:        "http://localhost:8800/api/rbac/v1",
			Skipper:        skipper,
			PermissionsMap: nil,
			Client:         client,
		})
	})
	require.Panics(t, func() {
		NewRbac(Rbac{
			BaseUrl:        "http://localhost:8800/api/rbac/v1",
			Skipper:        skipper,
			PermissionsMap: pm,
			Client:         nil,
		})
	})
}

func TestRbacMiddleware(t *testing.T) {
	type TestCaseGivenRbac struct {
		Resource string
		Verb     client.RbacVerb
		Allowed  bool
		Err      error
	}
	type TestCaseGivenRequest struct {
		Method string
		Path   string
	}
	type TestCaseGiven struct {
		GenerateIdentity bool
		Request          TestCaseGivenRequest
		MockResponse     TestCaseGivenRbac
		Skipper          echo_middleware.Skipper
	}
	type TestCaseExpected struct {
		Code int
		Body string
	}
	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected TestCaseExpected
	}

	testPath := "/api/content-sources/v1"

	testCases := []TestCase{
		{
			Name: "Skipper return true",
			Given: TestCaseGiven{
				GenerateIdentity: false,
				Request: TestCaseGivenRequest{
					Method: http.MethodGet,
					Path:   testPath + "/repositories/",
				},
				MockResponse: TestCaseGivenRbac{
					// Resource: "repositories",
					// Verb:     client.RbacVerbRead,
					// Allowed:  true,
					// Err:      nil,
				},
				Skipper: skipperTrue,
			},
			Expected: TestCaseExpected{
				Code: http.StatusOK,
				Body: "\"It worked\"\n",
			},
		},
		{
			Name: "Skipper return false",
			Given: TestCaseGiven{
				GenerateIdentity: true,
				Request: TestCaseGivenRequest{
					Method: http.MethodGet,
					Path:   testPath + "/repositories/",
				},
				MockResponse: TestCaseGivenRbac{
					Resource: "repositories",
					Verb:     client.RbacVerbRead,
					Allowed:  true,
					Err:      nil,
				},
				Skipper: skipperFalse,
			},
			Expected: TestCaseExpected{
				Code: http.StatusOK,
				Body: "\"It worked\"\n",
			},
		},
		{
			Name: "Skipper is nil",
			Given: TestCaseGiven{
				GenerateIdentity: true,
				Request: TestCaseGivenRequest{
					Method: http.MethodGet,
					Path:   testPath + "/repositories/",
				},
				MockResponse: TestCaseGivenRbac{
					Resource: "repositories",
					Verb:     client.RbacVerbRead,
					Allowed:  true,
					Err:      nil,
				},
				Skipper: nil,
			},
			Expected: TestCaseExpected{
				Code: http.StatusOK,
				Body: "\"It worked\"\n",
			},
		},
		{
			Name: "Resource is mapped to empty string",
			Given: TestCaseGiven{
				GenerateIdentity: false,
				Request: TestCaseGivenRequest{
					Method: http.MethodGet,
					Path:   testPath + "/",
				},
				MockResponse: TestCaseGivenRbac{
					// Resource: "repositories",
					// Verb:     client.RbacVerbRead,
					// Allowed:  true,
					// Err:      nil,
				},
				Skipper: nil,
			},
			Expected: TestCaseExpected{
				Code: http.StatusUnauthorized,
				Body: "{\"message\":\"Unauthorized\"}\n",
			},
		},
		{
			Name: "Verb is mapped to empty string",
			Given: TestCaseGiven{
				GenerateIdentity: false,
				Request: TestCaseGivenRequest{
					Method: "CONNECT",
					Path:   testPath,
				},
				MockResponse: TestCaseGivenRbac{
					// Resource: "repositories",
					// Verb:     client.RbacVerbRead,
					// Allowed:  true,
					// Err:      nil,
				},
				Skipper: nil,
			},
			Expected: TestCaseExpected{
				Code: http.StatusUnauthorized,
				Body: "{\"message\":\"Unauthorized\"}\n",
			},
		},
		{
			Name: "x-rh-identity is empty",
			Given: TestCaseGiven{
				GenerateIdentity: false,
				Request: TestCaseGivenRequest{
					Method: http.MethodGet,
					Path:   testPath,
				},
				MockResponse: TestCaseGivenRbac{
					// Resource: "repositories",
					// Verb:     client.RbacVerbRead,
					// Allowed:  true,
					// Err:      nil,
				},
				Skipper: nil,
			},
			Expected: TestCaseExpected{
				Code: http.StatusUnauthorized,
				Body: "{\"message\":\"Unauthorized\"}\n",
			},
		},
		{
			Name: "error checking permissions",
			Given: TestCaseGiven{
				GenerateIdentity: true,
				Request: TestCaseGivenRequest{
					Method: http.MethodGet,
					Path:   testPath + "/repositories/",
				},
				MockResponse: TestCaseGivenRbac{
					Resource: "repositories",
					Verb:     client.RbacVerbRead,
					Allowed:  true,
					Err:      fmt.Errorf("error parsing response"),
				},
				Skipper: nil,
			},
			Expected: TestCaseExpected{
				Code: http.StatusUnauthorized,
				Body: "{\"message\":\"Unauthorized\"}\n",
			},
		},
		{
			Name: "request not allowed",
			Given: TestCaseGiven{
				GenerateIdentity: true,
				Request: TestCaseGivenRequest{
					Method: http.MethodGet,
					Path:   testPath + "/repositories/",
				},
				MockResponse: TestCaseGivenRbac{
					Resource: "repositories",
					Verb:     client.RbacVerbRead,
					Allowed:  false,
					Err:      nil,
				},
				Skipper: nil,
			},
			Expected: TestCaseExpected{
				Code: http.StatusUnauthorized,
				Body: "{\"message\":\"Unauthorized\"}\n",
			},
		},
	}
	for _, testCase := range testCases {
		t.Log(testCase.Name)
		req, err := http.NewRequest(
			testCase.Given.Request.Method,
			testCase.Given.Request.Path,
			nil,
		)
		require.NoError(t, err)
		response := rbacServe(t, req,
			testCase.Given.MockResponse.Resource,
			testCase.Given.MockResponse.Verb,
			testCase.Given.Skipper,
			testCase.Given.MockResponse.Allowed,
			testCase.Given.MockResponse.Err,
			testCase.Given.GenerateIdentity)
		require.NotNil(t, response)
		assert.Equal(t, testCase.Expected.Code, response.Code)
		assert.Equal(t, testCase.Expected.Body, response.Body.String())
	}
}

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
		Resource string
		Verb     client.RbacVerb
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
				Resource: "repositories",
				Verb:     client.RbacVerbRead,
			},
			Expected: true,
		},
		{
			Name: "Empty method",
			Given: TestCaseGiven{
				Method:   "",
				Path:     "/api/" + application + "/v1/repositories",
				Resource: "repositories",
				Verb:     client.RbacVerbRead,
			},
			Expected: false,
		},
		{
			Name: "Empty path",
			Given: TestCaseGiven{
				Method:   http.MethodGet,
				Path:     "",
				Resource: "repositories",
				Verb:     client.RbacVerbRead,
			},
			Expected: false,
		},
		{
			Name: "Empty resource",
			Given: TestCaseGiven{
				Method:   http.MethodGet,
				Path:     "/api/" + application + "/v1/repositories",
				Resource: "",
				Verb:     client.RbacVerbRead,
			},
			Expected: false,
		},
		{
			Name: "Empty verb",
			Given: TestCaseGiven{
				Method:   http.MethodGet,
				Path:     "/api/" + application + "/v1/repositories",
				Resource: "repositories",
				Verb:     client.RbacVerbUndefined,
			},
			Expected: false,
		},
		{
			Name: "Resource wildcard",
			Given: TestCaseGiven{
				Method:   http.MethodGet,
				Path:     "/api/" + application + "/v1/repositories",
				Resource: "*",
				Verb:     client.RbacVerbRead,
			},
			Expected: false,
		},
		{
			Name: "Verb wildcard",
			Given: TestCaseGiven{
				Method:   http.MethodGet,
				Path:     "/api/" + application + "/v1/repositories",
				Resource: "repositories",
				Verb:     client.RbacVerbAny,
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
		Resource string
		Verb     client.RbacVerb
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
				Resource: "",
				Verb:     client.RbacVerbUndefined,
				Error:    fmt.Errorf(""),
			},
		},
		{
			Name: "GET */repositories",
			Given: TestCaseGiven{
				Method: http.MethodGet,
				Path:   "repositories",
			},
			Expected: TestCaseExpected{
				Resource: "repositories",
				Verb:     client.RbacVerbRead,
				Error:    nil,
			},
		},
		{
			Name: "POST */repository_parameters/validate",
			Given: TestCaseGiven{
				Method: http.MethodPost,
				Path:   "repository_parameters/validate",
			},
			Expected: TestCaseExpected{
				Resource: "repositories",
				Verb:     client.RbacVerbRead,
				Error:    nil,
			},
		},
		{
			Name: "Method no mapped",
			Given: TestCaseGiven{
				Method: "NOTEXISTING",
				Path:   "repository_parameters/validate",
			},
			Expected: TestCaseExpected{
				Resource: "",
				Verb:     client.RbacVerbUndefined,
				Error:    fmt.Errorf("no permission found for method=NOTEXISTING and path=/repository_parameters/validate"),
			},
		},
		{
			Name: "Path no mapped",
			Given: TestCaseGiven{
				Method: http.MethodPost,
				Path:   "rpms/validate",
			},
			Expected: TestCaseExpected{
				Resource: "",
				Verb:     client.RbacVerbUndefined,
				Error:    fmt.Errorf("no permission found for method=POST and path=/rpms/validate"),
			},
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		res, verb, err := ServicePermissions.Permission(testCase.Given.Method, testCase.Given.Path)
		if testCase.Expected.Error == nil {
			require.NoError(t, err)
			assert.Equal(t, testCase.Expected.Resource, res)
			assert.Equal(t, testCase.Expected.Verb, verb)
		} else {
			require.Error(t, err)
			assert.Equal(t, "", res)
			assert.Equal(t, client.RbacVerbUndefined, verb)
		}
	}
}
