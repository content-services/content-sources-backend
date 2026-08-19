package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CoverageReportSuite struct {
	suite.Suite
	echo *echo.Echo
}

func TestCoverageReportSuite(t *testing.T) {
	suite.Run(t, new(CoverageReportSuite))
}

func (suite *CoverageReportSuite) SetupTest() {
	suite.echo = echo.New()
	suite.echo.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	suite.echo.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
}

func (suite *CoverageReportSuite) TearDownTest() {
	require.NoError(suite.T(), suite.echo.Shutdown(context.Background()))
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

	RegisterCoverageReportRoutes(pathPrefix)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *CoverageReportSuite) TestCreateCoverageReport_Stub() {
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

	code, body, err := suite.serveCoverageReportRouter(req, true, true)
	assert.Nil(suite.T(), err)

	var response api.CoverageReportResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)

	assert.Equal(t, http.StatusCreated, code)
	assert.Equal(t, response.Status, "pending")
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
	assert.Nil(suite.T(), err)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Neither the user nor account is allowed")
}

func (suite *CoverageReportSuite) TestGetCoverageReport_Stub() {
	t := suite.T()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s/coverage_reports/%s", api.FullRootPath(), stubCompletedReportUUID), nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, true)
	assert.Nil(suite.T(), err)

	var response api.CoverageReportResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, response.Status, "completed")
	assert.Equal(t, response.UUID, stubCompletedReportUUID)
}

func (suite *CoverageReportSuite) TestGetCoverageReportNotAccessible() {
	t := suite.T()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s/coverage_reports/%s", api.FullRootPath(), stubCompletedReportUUID), nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, false)
	assert.Nil(suite.T(), err)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Neither the user nor account is allowed")
}

func (suite *CoverageReportSuite) TestGetHighCoverageReport_Stub() {
	t := suite.T()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s/coverage_reports/%s", api.FullRootPath(), stubHighCoverageReportUUID), nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, true)
	assert.Nil(suite.T(), err)

	var response api.CoverageReportResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "completed", response.Status)
	assert.Equal(t, stubHighCoverageReportUUID, response.UUID)
	assert.Equal(t, 10, response.Total)
	assert.Equal(t, 8, response.ExactMatches)
	assert.Equal(t, 1, response.PartialMatches)
	assert.Equal(t, 1, response.Unmatched)
}

func (suite *CoverageReportSuite) TestListCoverageReportPackages_Stub() {
	t := suite.T()
	path := fmt.Sprintf("%s/coverage_reports/%s/packages", api.FullRootPath(), stubCompletedReportUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, true)
	assert.Nil(suite.T(), err)

	var response api.CoverageReportPackagesResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 15, response.Total)
}

func (suite *CoverageReportSuite) TestListCoverageReportPackagesNotAccessible() {
	t := suite.T()
	path := fmt.Sprintf("%s/coverage_reports/%s/packages", api.FullRootPath(), stubCompletedReportUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, false)
	assert.Nil(suite.T(), err)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Neither the user nor account is allowed")
}

func (suite *CoverageReportSuite) TestListCoverageReportPackagesNameFilter_Stub() {
	t := suite.T()
	path := fmt.Sprintf("%s/coverage_reports/%s/packages?search=json5", api.FullRootPath(), stubCompletedReportUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, true)
	assert.Nil(suite.T(), err)

	var response api.CoverageReportPackagesResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, response.Total)
	assert.Equal(t, "json5", response.Results[0].Name)
}

func (suite *CoverageReportSuite) TestListCoverageReportPackagesEcosystemFilter_Stub() {
	t := suite.T()
	path := fmt.Sprintf("%s/coverage_reports/%s/packages?ecosystem=Java", api.FullRootPath(), stubCompletedReportUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, true)
	assert.Nil(suite.T(), err)

	var response api.CoverageReportPackagesResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 9, response.Total)
	for _, pkg := range response.Results {
		assert.Equal(t, "Java", pkg.Ecosystem)
	}
}

func (suite *CoverageReportSuite) TestListCoverageReportPackagesStatusFilter_Stub() {
	t := suite.T()
	path := fmt.Sprintf("%s/coverage_reports/%s/packages?status=in_network", api.FullRootPath(), stubCompletedReportUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := suite.serveCoverageReportRouter(req, true, true)
	assert.Nil(suite.T(), err)

	var response api.CoverageReportPackagesResponse
	err = json.Unmarshal(body, &response)
	assert.Nil(suite.T(), err)

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 11, response.Total)
	for _, pkg := range response.Results {
		assert.Equal(t, "in_network", pkg.Status)
	}
}

func (suite *CoverageReportSuite) TestGetCoverageReportNotFound_Stub() {
	t := suite.T()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s/coverage_reports/%s", api.FullRootPath(), "00000000-0000-0000-0000-000000000099"), nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveCoverageReportRouter(req, true, true)
	assert.Nil(suite.T(), err)

	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *CoverageReportSuite) TestListCoverageReportPackagesNotFound_Stub() {
	t := suite.T()
	path := fmt.Sprintf("%s/coverage_reports/%s/packages", api.FullRootPath(), "00000000-0000-0000-0000-000000000099")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveCoverageReportRouter(req, true, true)
	assert.Nil(suite.T(), err)

	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *CoverageReportSuite) TestListCoverageReportPackagesPendingReport_Stub() {
	t := suite.T()
	path := fmt.Sprintf("%s/coverage_reports/%s/packages", api.FullRootPath(), stubPendingReportUUID)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := suite.serveCoverageReportRouter(req, true, true)
	assert.Nil(suite.T(), err)

	assert.Equal(t, http.StatusNotFound, code)
}

func (suite *CoverageReportSuite) TestCreateCoverageReportMissingFile_Stub() {
	t := suite.T()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/coverage_reports/", api.FullRootPath()), nil)
	require.NoError(t, err)

	code, _, err := suite.serveCoverageReportRouter(req, true, true)
	assert.Nil(suite.T(), err)

	assert.Equal(t, http.StatusBadRequest, code)
}

func (suite *CoverageReportSuite) TestCreateCoverageReportEmptyBody_Stub() {
	t := suite.T()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/coverage_reports/", api.FullRootPath()), io.NopCloser(bytes.NewReader(nil)))
	require.NoError(t, err)

	code, _, err := suite.serveCoverageReportRouter(req, true, true)
	assert.Nil(suite.T(), err)

	assert.Equal(t, http.StatusBadRequest, code)
}
