package integration

import (
	"context"
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
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/suite"
)

type LightwellVulnerabilitiesSuite struct {
	suite.Suite
	dao             *dao.DaoRegistry
	customerID      string
	networkVulnID   string
	submittedVulnID string
	ticketNetwork   string
	ticketSubmitted string
	prevEnabled     bool
	prevAccounts    *[]string
}

func TestLightwellVulnerabilitiesSuite(t *testing.T) {
	suite.Run(t, new(LightwellVulnerabilitiesSuite))
}

func (s *LightwellVulnerabilitiesSuite) SetupSuite() {
	if db.LightwellQueries == nil {
		s.Require().NoError(db.Connect())
	}
	s.dao = dao.GetDaoRegistry(db.DB)
}

func (s *LightwellVulnerabilitiesSuite) SetupTest() {
	s.Require().NotNil(db.LightwellQueries)

	feature := &config.Get().Features.LightwellBeacon
	s.prevEnabled = feature.Enabled
	s.prevAccounts = feature.Accounts
	feature.Enabled = true
	feature.Accounts = &[]string{test_handler.MockAccountNumber}

	now := time.Now().UTC()
	suffix := now.UnixNano()
	s.customerID = fmt.Sprintf("lw-int-cust-%d", suffix)
	s.networkVulnID = fmt.Sprintf("LWL-INT-NETWORK-%d", suffix)
	s.submittedVulnID = fmt.Sprintf("LWL-INT-SUBMITTED-%d", suffix)
	s.ticketNetwork = fmt.Sprintf("lw-int-ticket-a-%d", suffix)
	s.ticketSubmitted = fmt.Sprintf("lw-int-ticket-b-%d", suffix)

	network := s.saveInput(s.networkVulnID, dao.LightwellVulnerabilityTicket{
		TicketID:   s.ticketNetwork,
		CustomerID: s.customerID,
	})
	network.Stage = "Lightwell Network"
	network.Severity = "Critical"
	network.Language = utils.Ptr("java")
	network.PublishedVersions = []string{"2.17.1.rhlw-00002", "2.17.1.rhlw-00001"}
	network.LastUpdated = now
	_, err := s.dao.LightwellVulnerability.Save(context.Background(), network)
	s.Require().NoError(err)

	submitted := s.saveInput(s.submittedVulnID, dao.LightwellVulnerabilityTicket{
		TicketID:   s.ticketSubmitted,
		CustomerID: s.customerID,
	})
	submitted.Stage = "Submitted"
	submitted.Severity = "Moderate"
	submitted.LastUpdated = now.Add(-time.Hour)
	_, err = s.dao.LightwellVulnerability.Save(context.Background(), submitted)
	s.Require().NoError(err)
}

func (s *LightwellVulnerabilitiesSuite) TearDownTest() {
	if s.dao != nil {
		ctx := context.Background()
		if s.networkVulnID != "" {
			_, _ = s.dao.LightwellVulnerability.DeleteByKey(ctx, s.networkVulnID)
		}
		if s.submittedVulnID != "" {
			_, _ = s.dao.LightwellVulnerability.DeleteByKey(ctx, s.submittedVulnID)
		}
	}
	feature := &config.Get().Features.LightwellBeacon
	feature.Enabled = s.prevEnabled
	feature.Accounts = s.prevAccounts
}

func (s *LightwellVulnerabilitiesSuite) saveInput(vulnID string, tickets ...dao.LightwellVulnerabilityTicket) dao.LightwellVulnerabilityInput {
	now := time.Now().UTC()
	return dao.LightwellVulnerabilityInput{
		VulnerabilityKey: vulnID,
		VulnerabilityID:  vulnID,
		ComponentName:    "log4j-core",
		ComponentVersion: "2.17.1",
		Severity:         "Critical",
		Stage:            "Submitted",
		Complexity:       "Standard",
		SubmittedDate:    now,
		LastUpdated:      now,
		Tickets:          tickets,
	}
}

func (s *LightwellVulnerabilitiesSuite) serve(req *http.Request) (int, []byte) {
	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	handler.RegisterLightwellVulnerabilityRoutes(router.Group(api.FullRootPath()), s.dao)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	response := rr.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	s.Require().NoError(err)
	return response.StatusCode, body
}

func (s *LightwellVulnerabilitiesSuite) get(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(s.T()))
	return req
}

func (s *LightwellVulnerabilitiesSuite) TestListCustomerIds() {
	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/customers/", api.FullRootPath())
	code, body := s.serve(s.get(path))
	s.Equal(http.StatusOK, code)

	var resp api.LightwellCustomerIdsResponse
	s.Require().NoError(json.Unmarshal(body, &resp))
	s.Contains(resp.Data, s.customerID)
}

func (s *LightwellVulnerabilitiesSuite) TestListTicketIds() {
	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/ltwlsupt-ticket-ids/?customer_id=%s", api.FullRootPath(), url.QueryEscape(s.customerID))
	code, body := s.serve(s.get(path))
	s.Equal(http.StatusOK, code)

	var resp api.LightwellLtwlsuptTicketIdsResponse
	s.Require().NoError(json.Unmarshal(body, &resp))
	s.Equal([]string{s.ticketNetwork, s.ticketSubmitted}, resp.Data)
}

func (s *LightwellVulnerabilitiesSuite) TestListVulnerabilities() {
	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=%s", api.FullRootPath(), url.QueryEscape(s.customerID))
	code, body := s.serve(s.get(path))
	s.Equal(http.StatusOK, code)

	var resp api.LightwellVulnerabilityCollectionResponse
	s.Require().NoError(json.Unmarshal(body, &resp))
	s.Require().Len(resp.Data, 2)

	byID := map[string]api.LightwellVulnerabilityResponse{}
	for _, row := range resp.Data {
		byID[row.VulnerabilityID] = row
	}

	network := byID[s.networkVulnID]
	s.Equal("Lightwell Network", network.Status)
	s.Equal("Critical", network.Severity)
	s.Equal("log4j-core", network.ComponentName)
	s.Equal("log4j-core", network.Package)
	s.Equal([]string{"2.17.1.rhlw-00002", "2.17.1.rhlw-00001"}, network.PublishedVersions)
	s.Equal([]string{s.ticketNetwork}, network.LtwlsuptTicketIDs)
	s.Require().NotNil(network.Ecosystem)
	s.Equal("java", *network.Ecosystem)
	s.NotEmpty(network.UUID)
	s.Contains(string(body), `"published_versions":["2.17.1.rhlw-00002","2.17.1.rhlw-00001"]`)
	s.Contains(string(body), `"status":"Lightwell Network"`)
	s.NotContains(string(body), `"stage"`)

	submitted := byID[s.submittedVulnID]
	s.Equal("Submitted", submitted.Status)
	s.Equal("Moderate", submitted.Severity)
	s.Equal([]string{}, submitted.PublishedVersions)
	s.Equal([]string{s.ticketSubmitted}, submitted.LtwlsuptTicketIDs)
	s.Contains(string(body), `"published_versions":[]`)
	s.NotContains(string(body), `"published_versions":null`)

	s.Equal(int64(2), resp.Meta.Count)
	s.Equal(int64(1), resp.Meta.CriticalCount)
	s.Equal(int64(0), resp.Meta.EmbargoCount)
	s.Equal(int64(1), resp.Meta.StatusCounts["Lightwell Network"])
	s.Equal(int64(1), resp.Meta.StatusCounts["Submitted"])
}

func (s *LightwellVulnerabilitiesSuite) TestListVulnerabilitiesFilterByStatus() {
	q := url.Values{}
	q.Set("customer_id", s.customerID)
	q.Set("status", "Lightwell Network")
	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?%s", api.FullRootPath(), q.Encode())
	code, body := s.serve(s.get(path))
	s.Equal(http.StatusOK, code)

	var resp api.LightwellVulnerabilityCollectionResponse
	s.Require().NoError(json.Unmarshal(body, &resp))
	s.Require().Len(resp.Data, 1)
	s.Equal(s.networkVulnID, resp.Data[0].VulnerabilityID)
	s.Equal("Lightwell Network", resp.Data[0].Status)
	s.Equal([]string{"2.17.1.rhlw-00002", "2.17.1.rhlw-00001"}, resp.Data[0].PublishedVersions)
	s.Equal(int64(1), resp.Meta.Count)
	s.Equal(int64(1), resp.Meta.CriticalCount)
	s.Equal(int64(1), resp.Meta.StatusCounts["Lightwell Network"])
	s.NotContains(resp.Meta.StatusCounts, "Submitted")
}

func (s *LightwellVulnerabilitiesSuite) TestListVulnerabilitiesSearch() {
	q := url.Values{}
	q.Set("customer_id", s.customerID)
	q.Set("search", "NETWORK")
	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?%s", api.FullRootPath(), q.Encode())
	code, body := s.serve(s.get(path))
	s.Equal(http.StatusOK, code)

	var resp api.LightwellVulnerabilityCollectionResponse
	s.Require().NoError(json.Unmarshal(body, &resp))
	s.Require().Len(resp.Data, 1)
	s.Equal(s.networkVulnID, resp.Data[0].VulnerabilityID)
	s.Equal(int64(1), resp.Meta.Count)
}

func (s *LightwellVulnerabilitiesSuite) TestListUnknownCustomerEmpty() {
	path := fmt.Sprintf("%s/lightwell/beacon/vulnerabilities/?customer_id=%s", api.FullRootPath(), url.QueryEscape("lw-int-missing-customer"))
	code, body := s.serve(s.get(path))
	s.Equal(http.StatusOK, code)

	var resp api.LightwellVulnerabilityCollectionResponse
	s.Require().NoError(json.Unmarshal(body, &resp))
	s.Empty(resp.Data)
	s.Equal(int64(0), resp.Meta.Count)
	s.Equal(int64(0), resp.Meta.CriticalCount)
	s.Empty(resp.Meta.StatusCounts)
}
