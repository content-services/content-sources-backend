package middleware

import (
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockXRhUserIdentity(t *testing.T, org_id string, username string) string {
	var (
		err       error
		xrhid     identity.XRHID
		jsonBytes []byte
	)
	xrhid.Identity.OrgID = org_id
	xrhid.Identity.AccountNumber = "11111"
	xrhid.Identity.User = &identity.User{Username: username}
	xrhid.Identity.Internal.OrgID = org_id

	jsonBytes, err = json.Marshal(xrhid)
	require.NoError(t, err)

	return b64.StdEncoding.EncodeToString([]byte(jsonBytes))
}

func hasToCallMock(resource rbac.Resource, verb rbac.Verb, rbacAllowed bool, rbacError error) bool {
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

func rbacServe(t *testing.T, req *http.Request, resource rbac.Resource, verb rbac.Verb, skipper echo_middleware.Skipper, rbacAllowed bool, rbacError error, generateIdentity bool) *httptest.ResponseRecorder {
	var (
		xrhid string
		rw    *httptest.ResponseRecorder
	)

	require.NotNil(t, req)

	if generateIdentity {
		xrhid = mockXRhUserIdentity(t, "12345", "12345")
		require.NotEqual(t, "", xrhid)
	}

	mockRbacClient := rbac.NewMockClientWrapper(t)
	require.NotNil(t, mockRbacClient)
	if hasToCallMock(resource, verb, rbacAllowed, rbacError) {
		mockRbacClient.On("Allowed", req.Context(), resource, verb).Return(rbacAllowed, rbacError)
	}

	e := echo.New()
	require.NotNil(t, e)

	// Add ClientWrapper middleware
	e.Use(
		NewRbac(Rbac{
			BaseUrl:        config.Get().Clients.RbacBaseUrl,
			Skipper:        skipper,
			PermissionsMap: rbac.ServicePermissions,
			Client:         mockRbacClient,
		}),
	)

	handler.RegisterRoutes(context.Background(), e)

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
	var pm *rbac.PermissionsMap = rbac.ServicePermissions
	var client = rbac.NewMockClientWrapper(t)
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
		Resource rbac.Resource
		Verb     rbac.Verb
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
				// Mock is not called on this scenario
				MockResponse: TestCaseGivenRbac{},
				Skipper:      skipperTrue,
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
					Verb:     rbac.RbacVerbRead,
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
					Verb:     rbac.RbacVerbRead,
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
				// Mock is not called on this scenario
				MockResponse: TestCaseGivenRbac{},
				Skipper:      nil,
			},
			Expected: TestCaseExpected{
				Code: http.StatusBadRequest,
				Body: "{\"message\":\"Bad Request\"}\n",
			},
		},
		{
			Name: "Verb is mapped to empty string",
			Given: TestCaseGiven{
				GenerateIdentity: false,
				Request: TestCaseGivenRequest{
					Method: "CONNECT",
					Path:   testPath + "/repositories/",
				},
				// Mock is not called on this scenario
				MockResponse: TestCaseGivenRbac{},
				Skipper:      nil,
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
					Path:   testPath + "/repositories/",
				},
				// Mock is not called on this scenario
				MockResponse: TestCaseGivenRbac{},
				Skipper:      nil,
			},
			Expected: TestCaseExpected{
				Code: http.StatusBadRequest,
				Body: "{\"message\":\"Bad Request\"}\n",
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
					Verb:     rbac.RbacVerbRead,
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
					Verb:     rbac.RbacVerbRead,
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
