package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/lightwell/osv"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type OsvDevSuite struct {
	suite.Suite
	reg *dao.MockDaoRegistry
}

func TestOsvDevSuite(t *testing.T) {
	suite.Run(t, new(OsvDevSuite))
}

func (suite *OsvDevSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
}

// serve mounts the identity middleware (with the standard skipper) and registers
// the osv routes on the raw engine, exactly like the real server. No identity
// header is ever set, verifying the routes are public.
func (suite *OsvDevSuite) serve(req *http.Request, enabled bool) (int, []byte) {
	config.Get().Features.LightwellOsvDemo.Enabled = enabled

	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	RegisterOsvDevRoutes(router, suite.reg.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func (suite *OsvDevSuite) advisories() []models.LightwellAdvisory {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	a := models.LightwellAdvisory{
		AdvisoryID:    "LW-2026-1",
		PackageName:   "left-pad",
		FixedVersions: []string{"1.3.0"},
		Details:       "left-pad flaw",
	}
	a.CreatedAt = now
	a.UpdatedAt = now
	b := models.LightwellAdvisory{
		AdvisoryID:    "LW-2026-2",
		PackageName:   "log4j",
		FixedVersions: []string{"2.17.0"},
	}
	b.CreatedAt = now
	b.UpdatedAt = now
	return []models.LightwellAdvisory{a, b}
}

func postJSON(path, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func (suite *OsvDevSuite) TestQueryHit() {
	suite.reg.LightwellAdvisory.On("ListForOsv", mock.Anything).Return(suite.advisories(), nil)

	req := postJSON("/demo/osvdev/v1/query", `{"version":"1.2.0","package":{"name":"left-pad"}}`)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)

	var resp api.OsvVulnerabilityList
	assert.NoError(suite.T(), json.Unmarshal(body, &resp))
	assert.Len(suite.T(), resp.Vulns, 1)
	assert.Equal(suite.T(), "LW-2026-1", resp.Vulns[0].ID)
}

func (suite *OsvDevSuite) TestQueryMissReturnsEmptyObject() {
	suite.reg.LightwellAdvisory.On("ListForOsv", mock.Anything).Return(suite.advisories(), nil)

	req := postJSON("/demo/osvdev/v1/query", `{"version":"9.9.9","package":{"name":"left-pad"}}`)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)
	assert.JSONEq(suite.T(), `{}`, string(body))
}

func (suite *OsvDevSuite) TestQueryBatchIndexAlignedStubs() {
	suite.reg.LightwellAdvisory.On("ListForOsv", mock.Anything).Return(suite.advisories(), nil)

	body := `{"queries":[{"version":"1.0.0","package":{"name":"left-pad"}},{"commit":"deadbeef"}]}`
	req := postJSON("/demo/osvdev/v1/querybatch", body)
	code, respBody := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)

	var resp api.OsvBatchVulnerabilityList
	assert.NoError(suite.T(), json.Unmarshal(respBody, &resp))
	assert.Len(suite.T(), resp.Results, 2)
	assert.Len(suite.T(), resp.Results[0].Vulns, 1)
	assert.Equal(suite.T(), "LW-2026-1", resp.Results[0].Vulns[0].ID)
	assert.NotEmpty(suite.T(), resp.Results[0].Vulns[0].Modified)
	assert.Empty(suite.T(), resp.Results[1].Vulns)
}

func (suite *OsvDevSuite) TestGetVuln() {
	suite.reg.LightwellAdvisory.On("ListForOsv", mock.Anything).Return(suite.advisories(), nil)

	req := httptest.NewRequest(http.MethodGet, "/demo/osvdev/v1/vulns/LW-2026-2", nil)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)

	var rec api.OsvVulnerability
	assert.NoError(suite.T(), json.Unmarshal(body, &rec))
	assert.Equal(suite.T(), "LW-2026-2", rec.ID)
}

func (suite *OsvDevSuite) TestGetVulnNotFound() {
	suite.reg.LightwellAdvisory.On("ListForOsv", mock.Anything).Return(suite.advisories(), nil)

	req := httptest.NewRequest(http.MethodGet, "/demo/osvdev/v1/vulns/NOPE", nil)
	code, _ := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusNotFound, code)
}

func (suite *OsvDevSuite) TestExportAllZip() {
	suite.reg.LightwellAdvisory.On("ListForOsv", mock.Anything).Return(suite.advisories(), nil)

	req := httptest.NewRequest(http.MethodGet, "/demo/osvdev/all.zip", nil)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	assert.NoError(suite.T(), err)
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	assert.ElementsMatch(suite.T(), []string{"LW-2026-1.json", "LW-2026-2.json"}, names)
}

func (suite *OsvDevSuite) TestExportEcosystemZip() {
	suite.reg.LightwellAdvisory.On("ListForOsv", mock.Anything).Return(suite.advisories(), nil)

	req := httptest.NewRequest(http.MethodGet, "/demo/osvdev/"+strings.ReplaceAll(osv.DefaultEcosystem, " ", "%20")+"/all.zip", nil)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), zr.File, 2)
}

func (suite *OsvDevSuite) TestDisabledReturns404() {
	req := postJSON("/demo/osvdev/v1/query", `{"package":{"name":"left-pad"}}`)
	code, _ := suite.serve(req, false)
	assert.Equal(suite.T(), http.StatusNotFound, code)
}

func (suite *OsvDevSuite) TestDaoError() {
	suite.reg.LightwellAdvisory.On("ListForOsv", mock.Anything).Return(nil, &ce.DaoError{Message: "db down"})

	req := postJSON("/demo/osvdev/v1/query", `{"package":{"name":"left-pad"}}`)
	code, _ := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusInternalServerError, code)
}
