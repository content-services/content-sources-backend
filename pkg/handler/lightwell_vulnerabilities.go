package handler

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

type LightwellVulnerabilityHandler struct {
	DaoRegistry dao.DaoRegistry
}

func checkLightwellBeaconAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := CheckLightwellBeaconAccessible(c.Request().Context()); err != nil {
			return err
		}
		return next(c)
	}
}

func RegisterLightwellVulnerabilityRoutes(group *echo.Group, daoReg *dao.DaoRegistry) {
	if group == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("daoReg is nil")
	}

	h := LightwellVulnerabilityHandler{DaoRegistry: *daoReg}
	addRepoRoute(group, http.MethodGet, "/lightwell/beacon/vulnerabilities/customers/", h.listCustomerIds, rbac.RbacVerbRead, checkLightwellBeaconAccessible)
	addRepoRoute(group, http.MethodGet, "/lightwell/beacon/vulnerabilities/ltwlsupt-ticket-ids/", h.listLtwlsuptTicketIds, rbac.RbacVerbRead, checkLightwellBeaconAccessible)
	addRepoRoute(group, http.MethodGet, "/lightwell/beacon/vulnerabilities/", h.listVulnerabilities, rbac.RbacVerbRead, checkLightwellBeaconAccessible)
}

// ListLightwellCustomerIds godoc
// @Summary      List Lightwell customer IDs
// @ID           listLightwellCustomerIds
// @Description  List distinct customer IDs that have Lightwell vulnerabilities.
// @Tags         lightwell_vulnerabilities
// @Accept       json
// @Produce      json
// @Success      200 {object} api.LightwellCustomerIdsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/beacon/vulnerabilities/customers/ [get]
func (h *LightwellVulnerabilityHandler) listCustomerIds(c echo.Context) error {
	ids, err := h.DaoRegistry.LightwellVulnerability.ListCustomerIds(c.Request().Context())
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing lightwell customer ids", err.Error())
	}
	if ids == nil {
		ids = []string{}
	}
	return c.JSON(http.StatusOK, api.LightwellCustomerIdsResponse{Data: ids})
}

// ListLightwellLtwlsuptTicketIds godoc
// @Summary      List Lightwell support ticket IDs
// @ID           listLightwellLtwlsuptTicketIds
// @Description  List distinct Lightwell support ticket IDs for a customer.
// @Tags         lightwell_vulnerabilities
// @Accept       json
// @Produce      json
// @Param        customer_id query string true "Customer ID (required)."
// @Success      200 {object} api.LightwellLtwlsuptTicketIdsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/beacon/vulnerabilities/ltwlsupt-ticket-ids/ [get]
func (h *LightwellVulnerabilityHandler) listLtwlsuptTicketIds(c echo.Context) error {
	customerID := strings.TrimSpace(c.QueryParam("customer_id"))
	if customerID == "" {
		return ce.NewErrorResponse(http.StatusBadRequest, "Missing customer_id", "customer_id is required")
	}

	ids, err := h.DaoRegistry.LightwellVulnerability.ListLtwlsuptTicketIds(c.Request().Context(), customerID)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing lightwell support ticket ids", err.Error())
	}
	if ids == nil {
		ids = []string{}
	}
	return c.JSON(http.StatusOK, api.LightwellLtwlsuptTicketIdsResponse{Data: ids})
}

// ListLightwellVulnerabilities godoc
// @Summary      List Lightwell vulnerabilities
// @ID           listLightwellVulnerabilities
// @Description  List Lightwell vulnerabilities for a customer, with filters, pagination, and aggregate counts.
// @Tags         lightwell_vulnerabilities
// @Accept       json
// @Produce      json
// @Param        customer_id query string true "Customer ID (required)."
// @Param        severity query string false "Comma-separated severities to filter on."
// @Param        stage query string false "Comma-separated stages to filter on."
// @Param        complexity query string false "Comma-separated complexities to filter on (Standard, Complex, Extensive)."
// @Param        ltwlsupt_ticket_id query string false "Comma-separated Lightwell support ticket IDs to filter on."
// @Param        flag query string false "Comma-separated flags to filter on (embargo, duplicate)."
// @Param        search query string false "Search vulnerability_id, component_name, and title. Minimum 2 characters."
// @Param        offset query int false "Starting point for retrieving a subset of results. Default value:`0`."
// @Param        limit query int false "Number of items to include in response. Default value: `100`. Maximum: `200`."
// @Success      200 {object} api.LightwellVulnerabilityCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/beacon/vulnerabilities/ [get]
func (h *LightwellVulnerabilityHandler) listVulnerabilities(c echo.Context) error {
	opts, err := parseLightwellVulnerabilityListOptions(c)
	if err != nil {
		return err
	}

	rows, aggregates, stageCounts, total, err := h.DaoRegistry.LightwellVulnerability.List(c.Request().Context(), opts)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing lightwell vulnerabilities", err.Error())
	}
	if rows == nil {
		rows = []api.LightwellVulnerabilityResponse{}
	}

	resp := api.LightwellVulnerabilityCollectionResponse{
		Data: rows,
		Meta: api.LightwellVulnerabilityCollectionMeta{
			CriticalCount: aggregates.CriticalCount,
			EmbargoCount:  aggregates.EmbargoCount,
			StageCounts:   stageCountsToMap(stageCounts),
		},
	}
	return c.JSON(http.StatusOK, setCollectionResponseMetadata(&resp, c, total))
}

func parseLightwellVulnerabilityListOptions(c echo.Context) (dao.ListLightwellVulnerabilitiesOptions, error) {
	customerID := strings.TrimSpace(c.QueryParam("customer_id"))
	if customerID == "" {
		return dao.ListLightwellVulnerabilitiesOptions{}, ce.NewErrorResponse(http.StatusBadRequest, "Missing customer_id", "customer_id is required")
	}

	search := strings.TrimSpace(c.QueryParam("search"))
	if search != "" && utf8.RuneCountInString(search) < 2 {
		return dao.ListLightwellVulnerabilitiesOptions{}, ce.NewErrorResponse(http.StatusBadRequest, "Invalid search", "search must be at least 2 characters")
	}

	limit, offset := parseLightwellPagination(c)
	opts := dao.ListLightwellVulnerabilitiesOptions{
		CustomerID:        customerID,
		Severities:        splitCSVQueryParam(c, "severity"),
		Stages:            splitCSVQueryParam(c, "stage"),
		Complexities:      splitCSVQueryParam(c, "complexity"),
		LtwlsuptTicketIDs: splitCSVQueryParam(c, "ltwlsupt_ticket_id"),
		Flags:             splitCSVQueryParam(c, "flag"),
		Limit:             limit,
		Offset:            offset,
	}

	if search != "" {
		opts.Search = &search
	}
	return opts, nil
}

func parseLightwellPagination(c echo.Context) (int32, int32) {
	var limit int32 = DefaultLimit
	var offset int32 = DefaultOffset
	_ = echo.QueryParamsBinder(c).
		Int32("limit", &limit).
		Int32("offset", &offset).
		BindError()
	if limit > MaxLimit {
		limit = MaxLimit
	}
	if limit == 0 {
		limit = DefaultLimit
	}
	return limit, offset
}

func splitCSVQueryParam(c echo.Context, name string) []string {
	raw := strings.TrimSpace(c.QueryParam(name))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func stageCountsToMap(rows []dao.LightwellVulnerabilityStageCount) map[string]int64 {
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.Stage] = row.Count
	}
	return counts
}
