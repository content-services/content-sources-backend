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
	"github.com/content-services/content-sources-backend/pkg/middleware"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	mock_dao "github.com/content-services/content-sources-backend/pkg/test/mocks"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/openlyinc/pointy"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func serveRpmsRouter(req *http.Request, mockDao *mock_dao.RpmDao) (int, []byte, error) {
	var (
		err error
	)

	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipLiveness))
	pathPrefix := router.Group(fullRootPath())

	router.HTTPErrorHandler = config.CustomHTTPErrorHandler

	rh := RepositoryRpmHandler{
		Dao: mockDao,
	}
	RegisterRepositoryRpmRoutes(pathPrefix, &rh.Dao)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

type RpmSuite struct {
	suite.Suite
	echo *echo.Echo
}

func (suite *RpmSuite) SetupTest() {
	suite.echo = echo.New()
	suite.echo.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	suite.echo.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipLiveness))
}

func (suite *RpmSuite) TearDownTest() {
	require.NoError(suite.T(), suite.echo.Shutdown(context.Background()))
}

func (suite *RpmSuite) TestRegisterRepositoryRpmRoutes() {
	t := suite.T()
	router := suite.echo
	pathPrefix := router.Group(fullRootPath())

	mockDao := mock_dao.NewRpmDao(t)
	rh := RepositoryRpmHandler{
		Dao: mockDao,
	}
	assert.NotPanics(t, func() {
		RegisterRepositoryRpmRoutes(pathPrefix, &rh.Dao)
	})
}

func TestListRepositoryRpms(t *testing.T) {
	type ComparisonFunc func(*testing.T, *api.RepositoryRpmCollectionResponse)
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
				Comparison: func(t *testing.T, response *api.RepositoryRpmCollectionResponse) {
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
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)

		mockRpmDao := mock_dao.NewRpmDao(t)
		path := fmt.Sprintf("%s/repositories/%s/rpms?%s", fullRootPath(), testCase.Given.UUID, testCase.Given.Params)
		switch {
		case testCase.Expected.Code >= 200 && testCase.Expected.Code < 300:
			{
				mockRpmDao.On("List", test_handler.MockOrgId, testCase.Given.UUID, testCase.Given.Page.Limit,
					testCase.Given.Page.Offset, testCase.Given.Search, testCase.Given.Page.SortBy).
					Return(api.RepositoryRpmCollectionResponse{
						Data: []api.RepositoryRpm{
							{
								Name:    "rpm-1",
								Summary: "Rpm1",
								Arch:    "x86_64",
							},
						},
						Meta:  api.ResponseMetadata{},
						Links: api.Links{},
					}, int64(1), nil)
			}
		case testCase.Expected.Code == http.StatusInternalServerError:
			{
				mockRpmDao.On("List", test_handler.MockOrgId, testCase.Given.UUID, testCase.Given.Page.Limit,
					testCase.Given.Page.Offset, testCase.Given.Search, testCase.Given.Page.SortBy).
					Return(api.RepositoryRpmCollectionResponse{}, int64(0), echo.NewHTTPError(http.StatusInternalServerError, "ISE"))
			}
		}

		// Prepare request
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
		req.Header.Set("Content-Type", "application/json")

		// Execute the request
		code, body, err := serveRpmsRouter(req, mockRpmDao)

		response := api.RepositoryRpmCollectionResponse{}
		if code == 200 {
			err = json.Unmarshal(body, &response)
			assert.Nil(t, err)
		}

		// Check results
		assert.Equal(t, testCase.Expected.Code, code)
		require.NoError(t, err)
		if testCase.Expected.Comparison != nil {
			testCase.Expected.Comparison(t, &response)
		}

		mockRpmDao.AssertExpectations(t)
	}
}

func TestSearchRpmPreprocessInput(t *testing.T) {
	type TestCase struct {
		Name     string
		Given    *api.SearchRpmRequest
		Expected *api.SearchRpmRequest
	}

	var testCases []TestCase = []TestCase{
		{
			Name:     "nil argument do nothing",
			Given:    nil,
			Expected: nil,
		},
		{
			Name: "structure with all nil does not evoque panic",
			Given: &api.SearchRpmRequest{
				URLs:   nil,
				UUIDs:  nil,
				Search: "",
				Limit:  nil,
			},
			Expected: &api.SearchRpmRequest{
				URLs:   nil,
				UUIDs:  nil,
				Search: "",
				Limit:  pointy.Int(api.SearchRpmRequestLimitDefault),
			},
		},
		{
			Name: "Limit nil result in LimitDefault",
			Given: &api.SearchRpmRequest{
				URLs:   nil,
				UUIDs:  nil,
				Search: "",
				Limit:  nil,
			},
			Expected: &api.SearchRpmRequest{
				URLs:   nil,
				UUIDs:  nil,
				Search: "",
				Limit:  pointy.Int(api.SearchRpmRequestLimitDefault),
			},
		},
		{
			Name: "Limit exceeding SearchRpmRequestLimitMaximum is reduced to SearchRpmRequestLimitMaximum",
			Given: &api.SearchRpmRequest{
				URLs:   nil,
				UUIDs:  nil,
				Search: "",
				Limit:  pointy.Int(api.SearchRpmRequestLimitMaximum + 1),
			},
			Expected: &api.SearchRpmRequest{
				URLs:   nil,
				UUIDs:  nil,
				Search: "",
				Limit:  pointy.Int(api.SearchRpmRequestLimitMaximum),
			},
		},
		{
			Name: "List of URL with end slash are trimmed",
			Given: &api.SearchRpmRequest{
				URLs: []string{
					"https://www.example.test/resource/",
					"https://www.example.test/resource///",
					"//",
				},
				UUIDs:  nil,
				Search: "",
				Limit:  nil,
			},
			Expected: &api.SearchRpmRequest{
				URLs: []string{
					"https://www.example.test/resource",
					"https://www.example.test/resource",
					"",
				},
				UUIDs:  nil,
				Search: "",
				Limit:  pointy.Int(api.SearchRpmRequestLimitDefault),
			},
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		h := RepositoryRpmHandler{
			Dao: nil,
		}
		assert.NotPanics(t, func() {
			h.searchRpmPreprocessInput(testCase.Given)
		})
		if testCase.Expected == nil {
			continue
		}
		if testCase.Expected.URLs != nil {
			require.NotNil(t, testCase.Given.URLs)
			assert.Equal(t, testCase.Expected.URLs, testCase.Given.URLs)
		} else {
			assert.Nil(t, testCase.Given.URLs)
		}
		if testCase.Expected.UUIDs != nil {
			require.NotNil(t, testCase.Given.UUIDs)
			assert.Equal(t, testCase.Expected.UUIDs, testCase.Given.UUIDs)
		} else {
			assert.Nil(t, testCase.Given.UUIDs)
		}
		assert.Equal(t, testCase.Expected.Search, testCase.Given.Search)
		if testCase.Expected.Limit != nil {
			require.NotNil(t, testCase.Given.Limit)
			assert.Equal(t, *testCase.Expected.Limit, *testCase.Given.Limit)
		} else {
			assert.Nil(t, testCase.Expected.Limit)
		}
	}
}

func (suite *RpmSuite) TestSearchRpmByName() {
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
				Body: "[{\"package_name\":\"demo-1\",\"summary\":\"Package demo 1\"},{\"package_name\":\"demo-2\",\"summary\":\"Package demo 2\"},{\"package_name\":\"demo-3\",\"summary\":\"Package demo 3\"}]\n",
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
				Body: "{\"errors\":[{\"status\":500,\"title\":\"Error searching RPMs\",\"detail\":\"code=500, message=must contain at least 1 URL or 1 UUID\"}]}\n",
			},
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)

		mockRpmDao := mock_dao.NewRpmDao(t)
		path := fmt.Sprintf("%s/rpms/names", fullRootPath())
		switch {
		case testCase.Expected.Code >= 200 && testCase.Expected.Code < 300:
			{
				var bodyRequest api.SearchRpmRequest
				err := json.Unmarshal([]byte(testCase.Given.Body), &bodyRequest)
				require.NoError(t, err)
				mockRpmDao.On("Search", test_handler.MockOrgId, bodyRequest).
					Return([]api.SearchRpmResponse{
						{
							PackageName: "demo-1",
							Summary:     "Package demo 1",
						},
						{
							PackageName: "demo-2",
							Summary:     "Package demo 2",
						},
						{
							PackageName: "demo-3",
							Summary:     "Package demo 3",
						},
					}, nil)
			}
		case testCase.Expected.Code == http.StatusBadRequest:
			{
			}
		case testCase.Expected.Code == http.StatusInternalServerError:
			{
				var bodyRequest api.SearchRpmRequest
				err := json.Unmarshal([]byte(testCase.Given.Body), &bodyRequest)
				bodyRequest.Limit = pointy.Int(api.SearchRpmRequestLimitDefault)
				require.NoError(t, err)
				mockRpmDao.On("Search", test_handler.MockOrgId, bodyRequest).
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
		code, body, err := serveRpmsRouter(req, mockRpmDao)

		// Check results
		assert.Equal(t, testCase.Expected.Code, code)
		require.NoError(t, err)
		assert.Equal(t, testCase.Expected.Body, string(body))
		mockRpmDao.AssertExpectations(t)
	}
}

func TestRpmSuite(t *testing.T) {
	suite.Run(t, new(RpmSuite))
}
