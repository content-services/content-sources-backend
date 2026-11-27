package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
)

// @Summary List Beacon package summaries
// @ID listLightwellPackageSummaries
// @Description Group matching vulnerability records by ecosystem, component name and component version.
// @Tags lightwell_vulnerabilities
// @Produce json
// @Param customer_id query string true "Customer ID"
// @Param severity query string false "Comma-separated severities"
// @Param status query string false "Comma-separated statuses"
// @Param complexity query string false "Comma-separated complexities"
// @Param ltwlsupt_ticket_id query string false "Comma-separated Lightwell support ticket IDs"
// @Param flag query string false "Comma-separated flags (embargo, duplicate)"
// @Param search query string false "Search vulnerability ID, package name or title"
// @Param all_addressed query bool false "Only packages whose customer records are all in Lightwell Network"
// @Param limit query int false "Maximum packages to return"
// @Param offset query int false "Package offset"
// @Success 200 {object} api.LightwellBeaconPackageCollection
// @Failure 400 {object} ce.ErrorResponse
// @Failure 401 {object} ce.ErrorResponse
// @Failure 428 {object} ce.ErrorResponse
// @Failure 500 {object} ce.ErrorResponse
// @Router /lightwell/beacon/packages/ [get]
func (h LightwellVulnerabilityHandler) listBeaconPackages(c echo.Context) error {
	opts, err := parseBeaconPackageListOptions(c)
	if err != nil {
		return err
	}
	var allAddressed bool
	if err := echo.QueryParamsBinder(c).Bool("all_addressed", &allAddressed).BindError(); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid all_addressed", err.Error())
	}
	if !beaconCustomerAllowed(c, opts.CustomerID) {
		resp := api.LightwellBeaconPackageCollection{Data: []api.LightwellBeaconPackage{}}
		resp.Meta.VulnerabilitySummary.StatusCounts = map[string]int64{}
		return c.JSON(http.StatusOK, setBeaconPackageMetadata(&resp, c, 0))
	}
	resp, count, err := h.DaoRegistry.LightwellVulnerability.ListBeaconPackages(c.Request().Context(), opts, allAddressed)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing Beacon packages", err.Error())
	}
	return c.JSON(http.StatusOK, setBeaconPackageMetadata(&resp, c, count))
}

// @Summary List vulnerability records for a Beacon package
// @ID listLightwellPackageVulnerabilities
// @Description Return all records for the customer and package in pipeline order, with summary fields across all pages.
// @Tags lightwell_vulnerabilities
// @Produce json
// @Param customer_id query string true "Customer ID"
// @Param ecosystem query string false "Exact ecosystem (empty for missing ecosystem)"
// @Param component_name query string true "Exact component name"
// @Param component_version query string true "Exact component version (may be empty)"
// @Param limit query int false "Maximum records to return"
// @Param offset query int false "Record offset"
// @Success 200 {object} api.LightwellBeaconPackageVulnerabilityCollection
// @Failure 400 {object} ce.ErrorResponse
// @Failure 401 {object} ce.ErrorResponse
// @Failure 428 {object} ce.ErrorResponse
// @Failure 500 {object} ce.ErrorResponse
// @Router /lightwell/beacon/packages/vulnerabilities/ [get]
func (h LightwellVulnerabilityHandler) listBeaconPackageVulnerabilities(c echo.Context) error {
	opts, err := parseBeaconPackagePagination(c)
	if err != nil {
		return err
	}
	if !c.QueryParams().Has("component_name") || !c.QueryParams().Has("component_version") {
		return ce.NewErrorResponse(http.StatusBadRequest, "Missing package identity", "component_name and component_version are required")
	}
	if !beaconCustomerAllowed(c, opts.CustomerID) {
		resp := api.LightwellBeaconPackageVulnerabilityCollection{Data: []api.LightwellBeaconPackageVulnerability{}}
		resp.Meta.StatusCounts = map[string]int64{}
		return c.JSON(http.StatusOK, setBeaconPackageMetadata(&resp, c, 0))
	}
	key := dao.BeaconPackageKey{
		Ecosystem: c.QueryParam("ecosystem"), ComponentName: c.QueryParam("component_name"),
		ComponentVersion: c.QueryParam("component_version"),
	}
	resp, count, err := h.DaoRegistry.LightwellVulnerability.ListBeaconPackageVulnerabilities(c.Request().Context(), opts, key)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing Beacon package vulnerabilities", err.Error())
	}
	return c.JSON(http.StatusOK, setBeaconPackageMetadata(&resp, c, count))
}

func parseBeaconPackageListOptions(c echo.Context) (dao.ListLightwellVulnerabilitiesOptions, error) {
	if err := validateBeaconPackagePagination(c); err != nil {
		return dao.ListLightwellVulnerabilitiesOptions{}, err
	}
	return parseLightwellVulnerabilityListOptions(c)
}

func parseBeaconPackagePagination(c echo.Context) (dao.ListLightwellVulnerabilitiesOptions, error) {
	if err := validateBeaconPackagePagination(c); err != nil {
		return dao.ListLightwellVulnerabilitiesOptions{}, err
	}
	customerID := strings.TrimSpace(c.QueryParam("customer_id"))
	if customerID == "" {
		return dao.ListLightwellVulnerabilitiesOptions{}, ce.NewErrorResponse(http.StatusBadRequest, "Missing customer_id", "customer_id is required")
	}
	limit, offset := parseLightwellPagination(c)
	return dao.ListLightwellVulnerabilitiesOptions{CustomerID: customerID, Limit: limit, Offset: offset}, nil
}

func validateBeaconPackagePagination(c echo.Context) error {
	if c.QueryParams().Has("sort_by") {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid sort_by", "Beacon packages use a fixed sort order")
	}
	var limit, offset int32
	err := echo.QueryParamsBinder(c).Int32("limit", &limit).Int32("offset", &offset).BindError()
	if err != nil || limit < 0 || offset < 0 {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid pagination", "limit and offset must be nonnegative integers")
	}
	return nil
}

func setBeaconPackageMetadata(collection api.CollectionMetadataSettable, c echo.Context, count int64) api.CollectionMetadataSettable {
	return setCollectionMetadata(collection, c, ParsePagination(c), count, func(c echo.Context, limit, offset int) string {
		req := c.Request()
		query := req.URL.Query()
		query.Set("limit", strconv.Itoa(limit))
		query.Set("offset", strconv.Itoa(offset))
		return req.URL.EscapedPath() + "?" + query.Encode()
	})
}
