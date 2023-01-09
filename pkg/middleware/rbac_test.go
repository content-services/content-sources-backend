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

func TestFromHttpVerbToRbacVerb(t *testing.T) {
	type TestCase struct {
		Name     string
		Given    string
		Expected client.RbacVerb
	}

	testCases := []TestCase{
		{
			Name:     "empty method",
			Given:    "",
			Expected: client.RbacVerbUndefined,
		},
		{
			Name:     "non existing method",
			Given:    "ANYOTHERTHING",
			Expected: client.RbacVerbUndefined,
		},
		{
			Name:     "GET",
			Given:    echo.GET,
			Expected: client.RbacVerbRead,
		},
		{
			Name:     "POST",
			Given:    echo.POST,
			Expected: client.RbacVerbWrite,
		},
		{
			Name:     "PUT",
			Given:    echo.PUT,
			Expected: client.RbacVerbWrite,
		},
		{
			Name:     "PATCH",
			Given:    echo.PATCH,
			Expected: client.RbacVerbWrite,
		},
		{
			Name:     "DELETE",
			Given:    echo.DELETE,
			Expected: client.RbacVerbWrite,
		},
		{
			Name:     "OPTIONS method map to undefined verb",
			Given:    echo.OPTIONS,
			Expected: client.RbacVerbUndefined,
		},
		{
			Name:     "HEAD method map to undefined verb",
			Given:    echo.HEAD,
			Expected: client.RbacVerbUndefined,
		},
		{
			Name:     "CONNECT method map to undefined verb",
			Given:    echo.CONNECT,
			Expected: client.RbacVerbUndefined,
		},
		{
			Name:     "PROPFIND method map to undefined verb",
			Given:    echo.PROPFIND,
			Expected: client.RbacVerbUndefined,
		},
		{
			Name:     "REPORT method map to undefined verb",
			Given:    echo.REPORT,
			Expected: client.RbacVerbUndefined,
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		result := fromHttpVerbToRbacVerb(testCase.Given)
		assert.Equal(t, testCase.Expected, result)
	}
}

func TestFromPathToResource(t *testing.T) {
	// func fromPathToResource(path string) string

	type TestCase struct {
		Name     string
		Given    string // URI Path
		Expected string // Resource translation
	}

	testCases := []TestCase{
		{
			Name:     "Empty path",
			Given:    "",
			Expected: "",
		},
		{
			Name:     "Violate the minimum item len",
			Given:    "/api",
			Expected: "",
		},
		{
			Name:     "Check /beta/api/content-sources/v1/repositories",
			Given:    "/beta/api/content-sources/v1/repositories",
			Expected: "repositories",
		},
		{
			Name:     "Check no api match for beta /beta/api2/content-sources/v1/repositories",
			Given:    "/beta/api2/content-sources/v1/repositories",
			Expected: "",
		},
		{
			Name:     "Check no api match for no beta /api2/content-sources/v1/repositories",
			Given:    "/api2/content-sources/v1/repositories",
			Expected: "",
		},
		{
			Name:     "Check match for no beta /api/content-sources/v1/repositories",
			Given:    "/api/content-sources/v1/repositories",
			Expected: "repositories",
		},
		{
			Name:     "Check match for beta /beta/api/content-sources/v1/repositories",
			Given:    "/beta/api/content-sources/v1/repositories",
			Expected: "repositories",
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		result := fromPathToResource(testCase.Given)
		assert.Equal(t, testCase.Expected, result)
	}
}

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
			BaseUrl: config.Get().Clients.RbacBaseUrl,
			Skipper: skipper,
		}, mockRbacClient),
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

	testPath := "/api/content-sources/v1/repositories/"

	testCases := []TestCase{
		{
			Name: "Skipper return true",
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
					Path:   testPath,
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
					Path:   testPath,
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
					Path:   "/api/content-sources/v1/",
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
					Path:   testPath,
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
					Path:   testPath,
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

func TestPermission(t *testing.T) {
	p := NewPermissionsMap().
		Add(http.MethodGet, "/repositories", "repositories", "read").
		Add(http.MethodDelete, "/repositories", "repositories", "write").
		Add(http.MethodGet, "/rpms", "repositories", "read").
		Add(http.MethodPost, "/repositories/validate", "repositories", "read")

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
				Path:   "/repositories",
			},
			Expected: TestCaseExpected{
				Resource: "repositories",
				Verb:     client.RbacVerbRead,
				Error:    nil,
			},
		},
		{
			Name: "GET */repositories",
			Given: TestCaseGiven{
				Method: http.MethodGet,
				Path:   "/repositories",
			},
			Expected: TestCaseExpected{
				Resource: "repositories",
				Verb:     client.RbacVerbRead,
				Error:    nil,
			},
		},
		{
			Name: "POST */rpms/validate",
			Given: TestCaseGiven{
				Method: http.MethodPost,
				Path:   "/rpms/validate",
			},
			Expected: TestCaseExpected{
				Resource: "repositories",
				Verb:     client.RbacVerbRead,
				Error:    nil,
			},
		},
	}

	for _, testCase := range testCases {
		res, verb, err := p.Permission(testCase.Given.Method, testCase.Given.Path)
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
