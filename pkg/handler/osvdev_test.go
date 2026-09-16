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

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// These tests exercise the osv.dev mock against the curated static records embedded
// in pkg/lightwell/osv (ecosystem "Maven"). No database or DAO mock is needed.
const (
	testEcosystem = "Maven"
	// springID is the record for org.springframework:spring-core, fixed at
	// 5.3.18.rhlw-00010 (base version 5.3.18 is affected).
	springID      = "RHLW-2026-00001"
	springPackage = "org.springframework:spring-core"
)

type OsvDevSuite struct {
	suite.Suite
}

func TestOsvDevSuite(t *testing.T) {
	suite.Run(t, new(OsvDevSuite))
}

// serve mounts the identity middleware (with the standard skipper) and registers
// the osv routes on the raw engine, exactly like the real server. No identity
// header is ever set, verifying the routes are public.
func (suite *OsvDevSuite) serve(req *http.Request, enabled bool) (int, []byte) {
	config.Get().Features.LightwellOsvDemo.Enabled = enabled

	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	RegisterOsvDevRoutes(router)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func postJSON(path, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// recordID unmarshals just the id field of a raw OSV record.
func recordID(raw json.RawMessage) string {
	var r struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &r)
	return r.ID
}

func (suite *OsvDevSuite) TestQueryHit() {
	req := postJSON("/demo/osvdev/v1/query",
		`{"version":"5.3.18","package":{"name":"`+springPackage+`","ecosystem":"`+testEcosystem+`"}}`)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)

	var resp api.OsvVulnerabilityList
	assert.NoError(suite.T(), json.Unmarshal(body, &resp))
	assert.Len(suite.T(), resp.Vulns, 1)
	assert.Equal(suite.T(), springID, recordID(resp.Vulns[0]))
}

func (suite *OsvDevSuite) TestQueryHitByPurl() {
	req := postJSON("/demo/osvdev/v1/query",
		`{"version":"5.3.18","package":{"purl":"pkg:maven/org.springframework/spring-core@5.3.18"}}`)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)

	var resp api.OsvVulnerabilityList
	assert.NoError(suite.T(), json.Unmarshal(body, &resp))
	assert.Len(suite.T(), resp.Vulns, 1)
	assert.Equal(suite.T(), springID, recordID(resp.Vulns[0]))
}

func (suite *OsvDevSuite) TestQueryPatchedVersionMisses() {
	// The remediated version is fixed, so it must not be reported as affected.
	req := postJSON("/demo/osvdev/v1/query",
		`{"version":"5.3.18.rhlw-00010","package":{"name":"`+springPackage+`","ecosystem":"`+testEcosystem+`"}}`)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)
	assert.JSONEq(suite.T(), `{}`, string(body))
}

func (suite *OsvDevSuite) TestQueryMissReturnsEmptyObject() {
	req := postJSON("/demo/osvdev/v1/query", `{"version":"9.9.9","package":{"name":"does-not-exist"}}`)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)
	assert.JSONEq(suite.T(), `{}`, string(body))
}

func (suite *OsvDevSuite) TestQueryBatchIndexAlignedStubs() {
	body := `{"queries":[{"version":"5.3.18","package":{"name":"` + springPackage + `"}},{"commit":"deadbeef"}]}`
	req := postJSON("/demo/osvdev/v1/querybatch", body)
	code, respBody := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)

	var resp api.OsvBatchVulnerabilityList
	assert.NoError(suite.T(), json.Unmarshal(respBody, &resp))
	assert.Len(suite.T(), resp.Results, 2)
	assert.Len(suite.T(), resp.Results[0].Vulns, 1)
	assert.Equal(suite.T(), springID, resp.Results[0].Vulns[0].ID)
	assert.NotEmpty(suite.T(), resp.Results[0].Vulns[0].Modified)
	assert.Empty(suite.T(), resp.Results[1].Vulns)
}

func (suite *OsvDevSuite) TestGetVuln() {
	req := httptest.NewRequest(http.MethodGet, "/demo/osvdev/v1/vulns/"+springID, nil)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)
	assert.Equal(suite.T(), springID, recordID(body))
}

func (suite *OsvDevSuite) TestGetVulnNotFound() {
	req := httptest.NewRequest(http.MethodGet, "/demo/osvdev/v1/vulns/NOPE", nil)
	code, _ := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusNotFound, code)
}

func (suite *OsvDevSuite) TestGetRecordFile() {
	req := httptest.NewRequest(http.MethodGet, "/demo/osvdev/"+testEcosystem+"/"+springID+".json", nil)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)
	assert.Equal(suite.T(), springID, recordID(body))
}

func (suite *OsvDevSuite) TestGetRecordFileWrongEcosystem() {
	req := httptest.NewRequest(http.MethodGet, "/demo/osvdev/PyPI/"+springID+".json", nil)
	code, _ := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusNotFound, code)
}

func (suite *OsvDevSuite) TestEcosystems() {
	req := httptest.NewRequest(http.MethodGet, "/demo/osvdev/ecosystems.txt", nil)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)
	assert.Equal(suite.T(), testEcosystem+"\n", string(body))
}

func (suite *OsvDevSuite) TestExportAllZip() {
	req := httptest.NewRequest(http.MethodGet, "/demo/osvdev/all.zip", nil)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	assert.NoError(suite.T(), err)
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	assert.ElementsMatch(suite.T(), []string{
		"RHLW-2026-00001.json", "RHLW-2026-00002.json", "RHLW-2026-00003.json",
		"RHLW-2026-00004.json", "RHLW-2026-00005.json",
	}, names)
}

func (suite *OsvDevSuite) TestExportEcosystemZip() {
	req := httptest.NewRequest(http.MethodGet, "/demo/osvdev/"+testEcosystem+"/all.zip", nil)
	code, body := suite.serve(req, true)
	assert.Equal(suite.T(), http.StatusOK, code)

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), zr.File, 5)
}

func (suite *OsvDevSuite) TestDisabledReturns404() {
	req := postJSON("/demo/osvdev/v1/query", `{"package":{"name":"`+springPackage+`"}}`)
	code, _ := suite.serve(req, false)
	assert.Equal(suite.T(), http.StatusNotFound, code)
}
