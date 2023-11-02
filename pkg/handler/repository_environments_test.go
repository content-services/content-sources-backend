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
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/openlyinc/pointy"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type EnvironmentSuite struct {
	suite.Suite
	echo *echo.Echo
	dao  dao.MockDaoRegistry
}

func (suite *EnvironmentSuite) SetupTest() {
	suite.echo = echo.New()
	suite.echo.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	suite.echo.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))
	suite.dao = *dao.GetMockDaoRegistry(suite.T())
}

func (suite *EnvironmentSuite) TearDownTest() {
	require.NoError(suite.T(), suite.echo.Shutdown(context.Background()))
}

func (suite *EnvironmentSuite) serveEnvironmentsRouter(req *http.Request) (int, []byte, error) {
	var (
		err error
	)

	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))
	pathPrefix := router.Group(api.FullRootPath())

	router.HTTPErrorHandler = config.CustomHTTPErrorHandler

	rh := RepositoryEnvironmentHandler{
		Dao: *suite.dao.ToDaoRegistry(),
	}
	RegisterRepositoryEnvironmentRoutes(pathPrefix, &rh.Dao)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *EnvironmentSuite) TestRegisterRepositoryEnvironmentRoutes() {
	t := suite.T()
	router := suite.echo
	pathPrefix := router.Group(api.FullRootPath())

	rh := RepositoryEnvironmentHandler{
		Dao: *suite.dao.ToDaoRegistry(),
	}
	assert.NotPanics(t, func() {
		RegisterRepositoryEnvironmentRoutes(pathPrefix, &rh.Dao)
	})
}

func (suite *EnvironmentSuite) TestListRepositoryEnvironments() {
	type ComparisonFunc func(*testing.T, *api.RepositoryEnvironmentCollectionResponse)
	type TestCaseExpected struct {
		Code       int
		Comparison ComparisonFunc
	}
	type TestCaseGiven struct {
		Params string
		UUID   string
		Page   api.PaginationData
		Search string
		SortBy string
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
				Params: `limit=50`,
				UUID:   "uuid-for-repo",
				Page:   api.PaginationData{Limit: 50},
			},
			Expected: TestCaseExpected{
				Code: http.StatusOK,
				Comparison: func(t *testing.T, response *api.RepositoryEnvironmentCollectionResponse) {
					assert.NotNil(t, response)
					assert.Equal(t, 1, len(response.Data))
				},
			},
		},
		{
			Name: "ISE",
			Given: TestCaseGiven{
				UUID: "uuid-for-repo",
				Page: api.PaginationData{Limit: 100},
			},
			Expected: TestCaseExpected{
				Code: http.StatusInternalServerError,
			},
		},
		{
			Name: "Not found",
			Given: TestCaseGiven{
				UUID: "not-an-actual-repo",
				Page: api.PaginationData{Limit: 100},
			},
			Expected: TestCaseExpected{
				Code: http.StatusNotFound,
			},
		},
	}

	for _, testCase := range testCases {
		suite.T().Log(testCase.Name)

		path := fmt.Sprintf("%s/repositories/%s/environments?%s", api.FullRootPath(), testCase.Given.UUID, testCase.Given.Params)
		switch {
		case testCase.Expected.Code >= 200 && testCase.Expected.Code < 300:
			{
				suite.dao.Environment.On("List", test_handler.MockOrgId, testCase.Given.UUID, testCase.Given.Page.Limit,
					testCase.Given.Page.Offset, testCase.Given.Search, testCase.Given.Page.SortBy).
					Return(api.RepositoryEnvironmentCollectionResponse{
						Data: []api.RepositoryEnvironment{
							{
								ID:          "environment-1",
								Name:        "environment-1",
								Description: "environment 1",
							},
						},
						Meta:  api.ResponseMetadata{},
						Links: api.Links{},
					}, int64(1), nil)
			}
		case testCase.Expected.Code == http.StatusInternalServerError:
			{
				suite.dao.Environment.On("List", test_handler.MockOrgId, testCase.Given.UUID, testCase.Given.Page.Limit,
					testCase.Given.Page.Offset, testCase.Given.Search, testCase.Given.Page.SortBy).
					Return(api.RepositoryEnvironmentCollectionResponse{}, int64(0), echo.NewHTTPError(http.StatusInternalServerError, "ISE"))
			}
		case testCase.Expected.Code == http.StatusNotFound:
			{
				suite.dao.Environment.On("List", test_handler.MockOrgId, testCase.Given.UUID, testCase.Given.Page.Limit,
					testCase.Given.Page.Offset, testCase.Given.Search, testCase.Given.Page.SortBy).
					Return(api.RepositoryEnvironmentCollectionResponse{}, int64(0), &ce.DaoError{NotFound: true})
			}
		}

		// Prepare request
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))
		req.Header.Set("Content-Type", "application/json")

		// Execute the request
		code, body, err := suite.serveEnvironmentsRouter(req)

		response := api.RepositoryEnvironmentCollectionResponse{}
		if code == 200 {
			err = json.Unmarshal(body, &response)
			assert.Nil(suite.T(), err)
		}

		// Check results
		assert.Equal(suite.T(), testCase.Expected.Code, code)
		require.NoError(suite.T(), err)
		if testCase.Expected.Comparison != nil {
			testCase.Expected.Comparison(suite.T(), &response)
		}
	}
}

func (suite *EnvironmentSuite) TestSearchEnvironmentPreprocessInput() {
	type TestCase struct {
		Name     string
		Given    *api.SearchSharedRepositoryEntityRequest
		Expected *api.SearchSharedRepositoryEntityRequest
	}

	var testCases []TestCase = []TestCase{
		{
			Name:     "nil argument do nothing",
			Given:    nil,
			Expected: nil,
		},
		{
			Name: "structure with all nil does not evoque panic",
			Given: &api.SearchSharedRepositoryEntityRequest{
				URLs:   nil,
				UUIDs:  nil,
				Search: "",
				Limit:  nil,
			},
			Expected: &api.SearchSharedRepositoryEntityRequest{
				URLs:   nil,
				UUIDs:  nil,
				Search: "",
				Limit:  pointy.Int(api.SearchSharedRepositoryEntityRequestLimitDefault),
			},
		},
		{
			Name: "Limit nil result in LimitDefault",
			Given: &api.SearchSharedRepositoryEntityRequest{
				URLs:   nil,
				UUIDs:  nil,
				Search: "",
				Limit:  nil,
			},
			Expected: &api.SearchSharedRepositoryEntityRequest{
				URLs:   nil,
				UUIDs:  nil,
				Search: "",
				Limit:  pointy.Int(api.SearchSharedRepositoryEntityRequestLimitDefault),
			},
		},
		{
			Name: "Limit exceeding SearchSharedRepositoryEntityRequestLimitMaximum is reduced to SearchSharedRepositoryEntityRequestLimitMaximum",
			Given: &api.SearchSharedRepositoryEntityRequest{
				URLs:   nil,
				UUIDs:  nil,
				Search: "",
				Limit:  pointy.Int(api.SearchSharedRepositoryEntityRequestLimitMaximum + 1),
			},
			Expected: &api.SearchSharedRepositoryEntityRequest{
				URLs:   nil,
				UUIDs:  nil,
				Search: "",
				Limit:  pointy.Int(api.SearchSharedRepositoryEntityRequestLimitMaximum),
			},
		},
		{
			Name: "List of URL with end slash are trimmed",
			Given: &api.SearchSharedRepositoryEntityRequest{
				URLs: []string{
					"https://www.example.test/resource/",
					"https://www.example.test/resource///",
					"//",
				},
				UUIDs:  nil,
				Search: "",
				Limit:  nil,
			},
			Expected: &api.SearchSharedRepositoryEntityRequest{
				URLs: []string{
					"https://www.example.test/resource",
					"https://www.example.test/resource",
					"",
				},
				UUIDs:  nil,
				Search: "",
				Limit:  pointy.Int(api.SearchSharedRepositoryEntityRequestLimitDefault),
			},
		},
	}

	for _, testCase := range testCases {
		suite.T().Log(testCase.Name)
		assert.NotPanics(suite.T(), func() {
			preprocessInput(testCase.Given)
		})
		if testCase.Expected == nil {
			continue
		}
		if testCase.Expected.URLs != nil {
			require.NotNil(suite.T(), testCase.Given.URLs)
			assert.Equal(suite.T(), testCase.Expected.URLs, testCase.Given.URLs)
		} else {
			assert.Nil(suite.T(), testCase.Given.URLs)
		}
		if testCase.Expected.UUIDs != nil {
			require.NotNil(suite.T(), testCase.Given.UUIDs)
			assert.Equal(suite.T(), testCase.Expected.UUIDs, testCase.Given.UUIDs)
		} else {
			assert.Nil(suite.T(), testCase.Given.UUIDs)
		}
		assert.Equal(suite.T(), testCase.Expected.Search, testCase.Given.Search)
		if testCase.Expected.Limit != nil {
			require.NotNil(suite.T(), testCase.Given.Limit)
			assert.Equal(suite.T(), *testCase.Expected.Limit, *testCase.Given.Limit)
		} else {
			assert.Nil(suite.T(), testCase.Expected.Limit)
		}
	}
}

func (suite *EnvironmentSuite) TestSearchEnvironmentByName() {
	t := suite.T()

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
				Body:   `{"urls":["https://www.example.test"],"search":"demo","limit":50}`,
			},
			Expected: TestCaseExpected{
				Code: http.StatusOK,
				Body: "[{\"environment_name\":\"demo-1\",\"description\":\"Environment demo 1\"},{\"environment_name\":\"demo-2\",\"description\":\"Environment demo 2\"},{\"environment_name\":\"demo-3\",\"description\":\"Environment demo 3\"}]\n",
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
		{
			Name: "Evoke a StatusInternalServerError response",
			Given: TestCaseGiven{
				Method: http.MethodPost,
				Body:   `{"search":"demo"}`,
			},
			Expected: TestCaseExpected{
				Code: http.StatusInternalServerError,
				Body: "{\"errors\":[{\"status\":500,\"title\":\"Error searching environments\",\"detail\":\"code=500, message=must contain at least 1 URL or 1 UUID\"}]}\n",
			},
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)

		path := fmt.Sprintf("%s/environments/names", api.FullRootPath())
		switch {
		case testCase.Expected.Code >= 200 && testCase.Expected.Code < 300:
			{
				var bodyRequest api.SearchSharedRepositoryEntityRequest
				err := json.Unmarshal([]byte(testCase.Given.Body), &bodyRequest)
				require.NoError(t, err)
				suite.dao.Environment.On("Search", test_handler.MockOrgId, bodyRequest).
					Return([]api.SearchEnvironmentResponse{
						{
							EnvironmentName: "demo-1",
							Description:     "Environment demo 1",
						},
						{
							EnvironmentName: "demo-2",
							Description:     "Environment demo 2",
						},
						{
							EnvironmentName: "demo-3",
							Description:     "Environment demo 3",
						},
					}, nil)
			}
		case testCase.Expected.Code == http.StatusBadRequest:
			{
			}
		case testCase.Expected.Code == http.StatusInternalServerError:
			{
				var bodyRequest api.SearchSharedRepositoryEntityRequest
				err := json.Unmarshal([]byte(testCase.Given.Body), &bodyRequest)
				bodyRequest.Limit = pointy.Int(api.SearchSharedRepositoryEntityRequestLimitDefault)
				require.NoError(t, err)
				suite.dao.Environment.On("Search", test_handler.MockOrgId, bodyRequest).
					Return(nil, echo.NewHTTPError(http.StatusInternalServerError, "must contain at least 1 URL or 1 UUID"))
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
		code, body, err := suite.serveEnvironmentsRouter(req)

		// Check results
		assert.Equal(t, testCase.Expected.Code, code)
		require.NoError(t, err)
		assert.Equal(t, testCase.Expected.Body, string(body))
	}
}

func TestEnvironmentSuite(t *testing.T) {
	suite.Run(t, new(EnvironmentSuite))
}
