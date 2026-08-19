package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type LightwellVulnerabilitiesSuite struct {
	suite.Suite
	reg *dao.MockDaoRegistry
}

func TestLightwellVulnerabilitiesSuite(t *testing.T) {
	suite.Run(t, new(LightwellVulnerabilitiesSuite))
}

func (suite *LightwellVulnerabilitiesSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
}

func (suite *LightwellVulnerabilitiesSuite) serveRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())
	RegisterLightwellVulnerabilityRoutes(pathPrefix, suite.reg.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (suite *LightwellVulnerabilitiesSuite) newGet(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(suite.T()))
	return req
}

func (suite *LightwellVulnerabilitiesSuite) TestListCustomerIds() {
	expected := []string{"demo-customer-1", "demo-customer-2"}
	suite.reg.LightwellVulnerability.On("ListCustomerIds", test.MockCtx()).Return(expected, nil)

	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/customers/", api.FullRootPath())
	code, body, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)

	var resp api.LightwellCustomerIdsResponse
	assert.NoError(suite.T(), json.Unmarshal(body, &resp))
	assert.Equal(suite.T(), expected, resp.Data)
}

func (suite *LightwellVulnerabilitiesSuite) TestListRequiresCustomerID() {
	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/", api.FullRootPath())
	code, _, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, code)
}

func (suite *LightwellVulnerabilitiesSuite) TestListSearchOneChar() {
	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1&search=a", api.FullRootPath())
	code, _, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, code)
}

func (suite *LightwellVulnerabilitiesSuite) TestListWithFiltersAndDuplicateOf() {
	dupOf := "LWL-2026-4027"
	search := "log4j"
	now := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	rows := []api.LightwellVulnerabilityResponse{{
		UUID:              "00000000-0000-4000-8000-000000000020",
		VulnerabilityID:   "LWL-2026-4108",
		ComponentName:     "ua-parser-js",
		Package:           "ua-parser-js",
		ComponentVersion:  "0.7.33",
		Severity:          "Moderate",
		Stage:             "Validation",
		Complexity:        "Standard",
		SubmittedDate:     now,
		LastUpdated:       now,
		AgeDays:           19,
		Blocked:           false,
		Duplicate:         true,
		DuplicateOf:       &dupOf,
		LtwlsuptTicketIDs: []string{"batch-3"},
	}}
	aggregates := dao.LightwellVulnerabilityAggregates{
		TotalCount:    1,
		CriticalCount: 0,
		EmbargoCount:  0,
		BlockedCount:  0,
	}
	stageCounts := []dao.LightwellVulnerabilityStageCount{{Stage: "Validation", Count: 1}}

	opts := dao.ListLightwellVulnerabilitiesOptions{
		CustomerID:        "demo-customer-1",
		Severities:        []string{"Moderate", "Critical"},
		Stages:            []string{"Validation"},
		Complexities:      []string{"Standard"},
		LtwlsuptTicketIDs: []string{"batch-3"},
		Flags:             []string{"duplicate"},
		Search:            &search,
		Limit:             100,
		Offset:            0,
	}
	suite.reg.LightwellVulnerability.On("List", test.MockCtx(), opts).Return(rows, aggregates, stageCounts, int64(1), nil)

	q := url.Values{}
	q.Set("customer_id", "demo-customer-1")
	q.Set("severity", "Moderate,Critical")
	q.Set("stage", "Validation")
	q.Set("complexity", "Standard")
	q.Set("ltwlsupt_ticket_id", "batch-3")
	q.Set("flag", "duplicate")
	q.Set("search", "log4j")
	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?%s", api.FullRootPath(), q.Encode())

	code, body, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)

	var resp api.LightwellVulnerabilityCollectionResponse
	assert.NoError(suite.T(), json.Unmarshal(body, &resp))
	assert.Len(suite.T(), resp.Data, 1)
	assert.Equal(suite.T(), "LWL-2026-4108", resp.Data[0].VulnerabilityID)
	assert.Equal(suite.T(), "00000000-0000-4000-8000-000000000020", resp.Data[0].UUID)
	assert.Contains(suite.T(), string(body), `"uuid":"00000000-0000-4000-8000-000000000020"`)
	assert.NotContains(suite.T(), string(body), `"id":`)
	assert.Equal(suite.T(), []string{"batch-3"}, resp.Data[0].LtwlsuptTicketIDs)
	assert.Contains(suite.T(), string(body), `"ltwlsupt_ticket_ids":["batch-3"]`)
	assert.False(suite.T(), resp.Data[0].Blocked)
	assert.Contains(suite.T(), string(body), `"blocked":false`)
	require := assert.New(suite.T())
	require.NotNil(resp.Data[0].DuplicateOf)
	assert.Equal(suite.T(), "LWL-2026-4027", *resp.Data[0].DuplicateOf)
	assert.Equal(suite.T(), int64(1), resp.Meta.Count)
	assert.Equal(suite.T(), int64(0), resp.Meta.CriticalCount)
	assert.Equal(suite.T(), int64(1), resp.Meta.StageCounts["Validation"])
}

func (suite *LightwellVulnerabilitiesSuite) TestListUnknownFiltersEmpty() {
	opts := dao.ListLightwellVulnerabilitiesOptions{
		CustomerID: "demo-customer-1",
		Severities: []string{"NonexistentSeverity"},
		Limit:      100,
		Offset:     0,
	}
	suite.reg.LightwellVulnerability.On("List", test.MockCtx(), opts).Return(
		[]api.LightwellVulnerabilityResponse{},
		dao.LightwellVulnerabilityAggregates{},
		[]dao.LightwellVulnerabilityStageCount{},
		int64(0),
		nil,
	)

	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1&severity=NonexistentSeverity", api.FullRootPath())
	code, body, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)

	var resp api.LightwellVulnerabilityCollectionResponse
	assert.NoError(suite.T(), json.Unmarshal(body, &resp))
	assert.Empty(suite.T(), resp.Data)
	assert.Equal(suite.T(), int64(0), resp.Meta.Count)
}

func (suite *LightwellVulnerabilitiesSuite) TestListFlagEmbargo() {
	opts := dao.ListLightwellVulnerabilitiesOptions{
		CustomerID: "demo-customer-1",
		Flags:      []string{"embargo"},
		Limit:      100,
		Offset:     0,
	}
	suite.reg.LightwellVulnerability.On("List", test.MockCtx(), opts).Return(
		[]api.LightwellVulnerabilityResponse{{VulnerabilityID: "LWL-2026-4401", Embargo: true}},
		dao.LightwellVulnerabilityAggregates{TotalCount: 1, EmbargoCount: 1},
		[]dao.LightwellVulnerabilityStageCount{{Stage: "Submitted", Count: 1}},
		int64(1),
		nil,
	)

	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1&flag=embargo", api.FullRootPath())
	code, body, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)

	var resp api.LightwellVulnerabilityCollectionResponse
	assert.NoError(suite.T(), json.Unmarshal(body, &resp))
	assert.Len(suite.T(), resp.Data, 1)
	assert.True(suite.T(), resp.Data[0].Embargo)
	assert.Equal(suite.T(), int64(1), resp.Meta.EmbargoCount)
}

func (suite *LightwellVulnerabilitiesSuite) TestListWhitespaceCustomerID() {
	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=%%20%%20", api.FullRootPath())
	code, _, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, code)
}

func (suite *LightwellVulnerabilitiesSuite) TestListSearchTwoChars() {
	search := "ab"
	opts := dao.ListLightwellVulnerabilitiesOptions{
		CustomerID: "demo-customer-1",
		Search:     &search,
		Limit:      100,
		Offset:     0,
	}
	suite.reg.LightwellVulnerability.On("List", test.MockCtx(), opts).Return(
		[]api.LightwellVulnerabilityResponse{},
		dao.LightwellVulnerabilityAggregates{},
		[]dao.LightwellVulnerabilityStageCount{},
		int64(0),
		nil,
	)

	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1&search=ab", api.FullRootPath())
	code, _, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)
}

func (suite *LightwellVulnerabilitiesSuite) TestListCSVSpacesAndEmptyValues() {
	opts := dao.ListLightwellVulnerabilitiesOptions{
		CustomerID: "demo-customer-1",
		Severities: []string{"Moderate", "Critical"},
		Limit:      100,
		Offset:     0,
	}
	suite.reg.LightwellVulnerability.On("List", test.MockCtx(), opts).Return(
		[]api.LightwellVulnerabilityResponse{},
		dao.LightwellVulnerabilityAggregates{},
		[]dao.LightwellVulnerabilityStageCount{},
		int64(0),
		nil,
	)

	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1&severity=Moderate,%%20Critical", api.FullRootPath())
	code, _, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)
}

func (suite *LightwellVulnerabilitiesSuite) TestListEmptyCSVIgnored() {
	opts := dao.ListLightwellVulnerabilitiesOptions{
		CustomerID: "demo-customer-1",
		Limit:      100,
		Offset:     0,
	}
	suite.reg.LightwellVulnerability.On("List", test.MockCtx(), opts).Return(
		[]api.LightwellVulnerabilityResponse{},
		dao.LightwellVulnerabilityAggregates{},
		[]dao.LightwellVulnerabilityStageCount{},
		int64(0),
		nil,
	)

	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1&severity=,", api.FullRootPath())
	code, _, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)
}

func (suite *LightwellVulnerabilitiesSuite) TestListFlagsCSVForwarded() {
	opts := dao.ListLightwellVulnerabilitiesOptions{
		CustomerID: "demo-customer-1",
		Flags:      []string{"embargo", "blocked"},
		Limit:      100,
		Offset:     0,
	}
	suite.reg.LightwellVulnerability.On("List", test.MockCtx(), opts).Return(
		[]api.LightwellVulnerabilityResponse{},
		dao.LightwellVulnerabilityAggregates{},
		[]dao.LightwellVulnerabilityStageCount{},
		int64(0),
		nil,
	)

	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1&flag=embargo,blocked", api.FullRootPath())
	code, _, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)
}

func (suite *LightwellVulnerabilitiesSuite) TestListInvalidFlagForwardsToDao() {
	opts := dao.ListLightwellVulnerabilitiesOptions{
		CustomerID: "demo-customer-1",
		Flags:      []string{"bogus"},
		Limit:      100,
		Offset:     0,
	}
	suite.reg.LightwellVulnerability.On("List", test.MockCtx(), opts).Return(
		[]api.LightwellVulnerabilityResponse{},
		dao.LightwellVulnerabilityAggregates{},
		[]dao.LightwellVulnerabilityStageCount{},
		int64(0),
		nil,
	)

	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1&flag=bogus", api.FullRootPath())
	code, _, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)
}

func (suite *LightwellVulnerabilitiesSuite) TestListPaginationForwarded() {
	opts := dao.ListLightwellVulnerabilitiesOptions{
		CustomerID: "demo-customer-1",
		Limit:      200,
		Offset:     10,
	}
	suite.reg.LightwellVulnerability.On("List", test.MockCtx(), opts).Return(
		[]api.LightwellVulnerabilityResponse{},
		dao.LightwellVulnerabilityAggregates{},
		[]dao.LightwellVulnerabilityStageCount{},
		int64(0),
		nil,
	)

	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1&limit=500&offset=10", api.FullRootPath())
	code, _, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, code)
}

func (suite *LightwellVulnerabilitiesSuite) TestListDaoError() {
	opts := dao.ListLightwellVulnerabilitiesOptions{
		CustomerID: "demo-customer-1",
		Limit:      100,
		Offset:     0,
	}
	suite.reg.LightwellVulnerability.On("List", test.MockCtx(), opts).Return(
		[]api.LightwellVulnerabilityResponse{},
		dao.LightwellVulnerabilityAggregates{},
		[]dao.LightwellVulnerabilityStageCount{},
		int64(0),
		&ce.DaoError{Message: "db down"},
	)

	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=demo-customer-1", api.FullRootPath())
	code, _, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusInternalServerError, code)
}

func (suite *LightwellVulnerabilitiesSuite) TestListCustomerIdsDaoError() {
	suite.reg.LightwellVulnerability.On("ListCustomerIds", test.MockCtx()).Return(nil, &ce.DaoError{Message: "db down"})

	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/customers/", api.FullRootPath())
	code, _, err := suite.serveRouter(suite.newGet(path))
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusInternalServerError, code)
}
