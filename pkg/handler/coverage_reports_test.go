package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CoverageReportSuite struct {
	suite.Suite
	reg *dao.MockDaoRegistry
}

func TestCoverageReportSuite(t *testing.T) {
	suite.Run(t, new(CoverageReportSuite))
}

func (suite *CoverageReportSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
}

func (suite *CoverageReportSuite) serveCoverageReportRouter(req *http.Request, enabled bool, authorized bool) (int, []byte, error) {
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	pathPrefix := router.Group(api.FullRootPath())

	if enabled {
		config.Get().Features.LightwellBeaconAndLens.Enabled = true
	} else {
		config.Get().Features.LightwellBeaconAndLens.Enabled = false
	}

	if authorized {
		config.Get().Features.LightwellBeaconAndLens.Accounts = &[]string{test_handler.MockAccountNumber}
	} else {
		config.Get().Features.LightwellBeaconAndLens.Accounts = &[]string{seeds.RandomAccountId()}
	}

	RegisterCoverageReportRoutes(pathPrefix, suite.reg.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *CoverageReportSuite) TestGetCoverageReport() {
	t := suite.T()
	orgID := test_handler.MockOrgId
	reportUUID := "550e8400-e29b-41d4-a716-446655440000"

	expected := api.CoverageReportResponse{
		UUID:   reportUUID,
		Status: "completed",
		Total:  15,
	}

	suite.reg.CoverageReport.On("Fetch", test.MockCtx(), orgID, reportUUID).Return(expected, nil)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s/coverage_reports/%s", api.FullRootPath(), reportUUID), nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.CoverageReportResponse
	assert.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, expected.UUID, response.UUID)
	assert.Equal(t, expected.Status, response.Status)
	assert.Equal(t, expected.Total, response.Total)
}

func (suite *CoverageReportSuite) TestGetCoverageReportNotFound() {
	t := suite.T()
	orgID := test_handler.MockOrgId
	reportUUID := "00000000-0000-0000-0000-000000000099"

	suite.reg.CoverageReport.On("Fetch", test.MockCtx(), orgID, reportUUID).
		Return(api.CoverageReportResponse{}, &ce.DaoError{Message: "not found", NotFound: true})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s/coverage_reports/%s", api.FullRootPath(), reportUUID), nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveCoverageReportRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *CoverageReportSuite) TestGetCoverageReportNotAccessible() {
	t := suite.T()
	reportUUID := "550e8400-e29b-41d4-a716-446655440000"
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s/coverage_reports/%s", api.FullRootPath(), reportUUID), nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Neither the user nor account is allowed")
}

func (suite *CoverageReportSuite) TestListCoverageReportPackages() {
	t := suite.T()
	orgID := test_handler.MockOrgId
	reportUUID := "550e8400-e29b-41d4-a716-446655440000"

	expectedData := []api.CoverageReportPackageResponse{
		{Name: "spring-core", Version: "6.1.0", Ecosystem: "Java", Covered: true},
		{Name: "unknown-pkg", Version: "1.0.0", Ecosystem: "Java", Covered: false},
	}
	expectedResp := api.CoverageReportPackageCollectionResponse{Data: expectedData}

	suite.reg.CoverageReport.On("ListPackages", test.MockCtx(), orgID, reportUUID,
		api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset, SortBy: DefaultSortBy},
		api.ListCoverageReportPackagesRequest{},
	).Return(expectedResp, int64(2), nil)

	path := fmt.Sprintf("%s/coverage_reports/%s/packages", api.FullRootPath(), reportUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.CoverageReportPackageCollectionResponse
	assert.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, 2, len(response.Data))
	assert.Equal(t, int64(2), response.Meta.Count)
}

func (suite *CoverageReportSuite) TestListCoverageReportPackagesNotFound() {
	t := suite.T()
	orgID := test_handler.MockOrgId
	reportUUID := "00000000-0000-0000-0000-000000000099"

	suite.reg.CoverageReport.On("ListPackages", test.MockCtx(), orgID, reportUUID,
		api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset, SortBy: DefaultSortBy},
		api.ListCoverageReportPackagesRequest{},
	).Return(api.CoverageReportPackageCollectionResponse{}, int64(0), &ce.DaoError{Message: "not found", NotFound: true})

	path := fmt.Sprintf("%s/coverage_reports/%s/packages", api.FullRootPath(), reportUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveCoverageReportRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *CoverageReportSuite) TestListCoverageReportPackagesNotAccessible() {
	t := suite.T()
	reportUUID := "550e8400-e29b-41d4-a716-446655440000"
	path := fmt.Sprintf("%s/coverage_reports/%s/packages", api.FullRootPath(), reportUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Neither the user nor account is allowed")
}

func (suite *CoverageReportSuite) TestListCoverageReportPackagesWithFilters() {
	t := suite.T()
	orgID := test_handler.MockOrgId
	reportUUID := "550e8400-e29b-41d4-a716-446655440000"

	covered := true
	expectedResp := api.CoverageReportPackageCollectionResponse{
		Data: []api.CoverageReportPackageResponse{
			{Name: "spring-core", Version: "6.1.0", Ecosystem: "Java", Covered: true},
		},
	}

	suite.reg.CoverageReport.On("ListPackages", test.MockCtx(), orgID, reportUUID,
		api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset, SortBy: DefaultSortBy},
		api.ListCoverageReportPackagesRequest{Covered: &covered, Ecosystem: "Java", Search: "spring"},
	).Return(expectedResp, int64(1), nil)

	path := fmt.Sprintf("%s/coverage_reports/%s/packages?covered=true&ecosystem=Java&search=spring", api.FullRootPath(), reportUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	var response api.CoverageReportPackageCollectionResponse
	assert.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, 1, len(response.Data))
	assert.True(t, response.Data[0].Covered)
}

func (suite *CoverageReportSuite) TestCreateCoverageReport() {
	t := suite.T()
	reqBody := &bytes.Buffer{}
	writer := multipart.NewWriter(reqBody)
	part, err := writer.CreateFormFile("file", "sbom.json")
	require.NoError(t, err)
	_, err = part.Write([]byte(`{"bomFormat":"CycloneDX"}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	expectedReport := api.CoverageReportResponse{
		UUID:      uuid.NewString(),
		Status:    config.TaskStatusPending,
		CreatedAt: time.Now(),
	}
	suite.reg.CoverageReport.On("Create", mock.Anything, mock.Anything, mock.Anything).
		Return(expectedReport, nil)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("%s/coverage_reports/", api.FullRootPath()), reqBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, true)
	assert.NoError(t, err)

	var response api.CoverageReportResponse
	assert.NoError(t, json.Unmarshal(body, &response))

	assert.Equal(t, http.StatusCreated, code)
	assert.Equal(t, expectedReport.Status, response.Status)
	assert.Equal(t, expectedReport.UUID, response.UUID)
}

func (suite *CoverageReportSuite) TestCreateCoverageReportNotAccessible() {
	t := suite.T()
	reqBody := &bytes.Buffer{}
	writer := multipart.NewWriter(reqBody)
	part, err := writer.CreateFormFile("file", "sbom.json")
	require.NoError(t, err)
	_, err = part.Write([]byte(`{"bomFormat":"CycloneDX"}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("%s/coverage_reports/", api.FullRootPath()), reqBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Neither the user nor account is allowed")
}

func (suite *CoverageReportSuite) TestCreateMissingFile() {
	t := suite.T()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/coverage_reports/", api.FullRootPath()), nil)
	require.NoError(t, err)

	code, _, err := suite.serveCoverageReportRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *CoverageReportSuite) TestCreateCoverageReportEmptyBody() {
	t := suite.T()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/coverage_reports/", api.FullRootPath()), io.NopCloser(bytes.NewReader(nil)))
	require.NoError(t, err)

	code, _, err := suite.serveCoverageReportRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
}
