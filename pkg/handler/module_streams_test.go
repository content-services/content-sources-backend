package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ModuleStreamsSuite struct {
	suite.Suite
	echo *echo.Echo
	dao  dao.MockDaoRegistry
}

func TestModuleStreamsSuite(t *testing.T) {
	suite.Run(t, new(ModuleStreamsSuite))
}

func (suite *ModuleStreamsSuite) SetupTest() {
	suite.echo = echo.New()
	suite.echo.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	suite.echo.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	suite.dao = *dao.GetMockDaoRegistry(suite.T())
}

func (suite *ModuleStreamsSuite) TearDownTest() {
	require.NoError(suite.T(), suite.echo.Shutdown(context.Background()))
}

func (suite *ModuleStreamsSuite) serveModuleStreamsRouter(req *http.Request) (int, []byte, error) {
	var (
		err error
	)

	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	pathPrefix := router.Group(api.FullRootPath())

	router.HTTPErrorHandler = config.CustomHTTPErrorHandler

	rh := ModuleStreamsHandler{
		Dao: *suite.dao.ToDaoRegistry(),
	}
	RegisterModuleStreamsRoutes(pathPrefix, &rh.Dao)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *ModuleStreamsSuite) TestSearchSnapshotModuleStreams() {
	t := suite.T()

	config.Load()
	config.Get().Features.Snapshots.Enabled = true
	config.Get().Features.Snapshots.Accounts = &[]string{test_handler.MockAccountNumber}
	defer resetFeatures()

	type TestCaseExpected struct {
		Code int
		Body string
	}

	type TestCaseGiven struct {
		Method string
		Body   string
	}

	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected TestCaseExpected
	}

	var testCases []TestCase = []TestCase{
		{
			Name: "Success scenario",
			Given: TestCaseGiven{
				Method: http.MethodPost,
				Body:   `{"uuids":["abcd"],"rpm_names":[],"search":"demo"}`,
			},
			Expected: TestCaseExpected{
				Code: http.StatusOK,
				Body: "[]\n",
			},
		},
		{
			Name: "Evoke a StatusBadRequest response",
			Given: TestCaseGiven{
				Method: http.MethodPost,
				Body:   "{",
			},
			Expected: TestCaseExpected{
				Code: http.StatusBadRequest,
				Body: "{\"errors\":[{\"status\":400,\"title\":\"Error binding parameters\",\"detail\":\"code=400, message=unexpected EOF, internal=unexpected EOF\"}]}\n",
			},
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)

		path := fmt.Sprintf("%s/snapshots/module_streams/search", api.FullRootPath())
		switch {
		case testCase.Expected.Code >= 200 && testCase.Expected.Code < 300:
			{
				var bodyRequest api.SearchSnapshotModuleStreamsRequest
				err := json.Unmarshal([]byte(testCase.Given.Body), &bodyRequest)
				require.NoError(t, err)
				suite.dao.ModuleStream.On("SearchSnapshotModuleStreams", mock.AnythingOfType("*context.valueCtx"), test_handler.MockOrgId, bodyRequest).
					Return([]api.SearchModuleStreams{}, nil)
			}
		default:
			{
			}
		}

		var bodyRequest io.Reader
		if testCase.Given.Body == "" {
			bodyRequest = nil
		} else {
			bodyRequest = strings.NewReader(testCase.Given.Body)
		}

		// Prepare request
		req := httptest.NewRequest(testCase.Given.Method, path, bodyRequest)
		req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
		req.Header.Set("Content-Type", "application/json")

		// Execute the request
		code, body, err := suite.serveModuleStreamsRouter(req)

		// Check results
		assert.Equal(t, testCase.Expected.Code, code)
		require.NoError(t, err)
		assert.Equal(t, testCase.Expected.Body, string(body))
	}
}

func (suite *ModuleStreamsSuite) TestSearchRepoModuleStreams() {
	t := suite.T()

	config.Load()
	config.Get().Features.Snapshots.Enabled = true
	config.Get().Features.Snapshots.Accounts = &[]string{test_handler.MockAccountNumber}
	defer resetFeatures()

	type TestCaseExpected struct {
		Code int
		Body string
	}

	type TestCaseGiven struct {
		Method string
		Body   string
	}

	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected TestCaseExpected
	}

	var testCases = []TestCase{
		{
			Name: "Success scenario",
			Given: TestCaseGiven{
				Method: http.MethodPost,
				Body:   `{"urls":["URL"],"rpm_names":[],"search":"demo"}`,
			},
			Expected: TestCaseExpected{
				Code: http.StatusOK,
				Body: "{\"data\":[],\"meta\":{\"limit\":0,\"offset\":0,\"count\":0},\"links\":{\"first\":\"\",\"last\":\"\"}}\n",
			},
		},
		{
			Name: "Evoke a StatusBadRequest response",
			Given: TestCaseGiven{
				Method: http.MethodPost,
				Body:   "{",
			},
			Expected: TestCaseExpected{
				Code: http.StatusBadRequest,
				Body: "{\"errors\":[{\"status\":400,\"title\":\"Error binding parameters\",\"detail\":\"code=400, message=unexpected EOF, internal=unexpected EOF\"}]}\n",
			},
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)

		path := fmt.Sprintf("%s/module_streams/search", api.FullRootPath())
		switch {
		case testCase.Expected.Code >= 200 && testCase.Expected.Code < 300:
			{
				var bodyRequest api.SearchModuleStreamsRequest
				err := json.Unmarshal([]byte(testCase.Given.Body), &bodyRequest)
				require.NoError(t, err)
				suite.dao.ModuleStream.On("SearchRepositoryModuleStreams", mock.AnythingOfType("*context.valueCtx"), test_handler.MockOrgId, bodyRequest).
					Return([]api.SearchModuleStreams{}, nil)
			}
		default:
			{
			}
		}

		var bodyRequest io.Reader
		if testCase.Given.Body == "" {
			bodyRequest = nil
		} else {
			bodyRequest = strings.NewReader(testCase.Given.Body)
		}

		// Prepare request
		req := httptest.NewRequest(testCase.Given.Method, path, bodyRequest)
		req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
		req.Header.Set("Content-Type", "application/json")

		// Execute the request
		code, body, err := suite.serveModuleStreamsRouter(req)

		// Check results
		assert.Equal(t, testCase.Expected.Code, code)
		require.NoError(t, err)
		assert.Equal(t, testCase.Expected.Body, string(body))
	}
}
