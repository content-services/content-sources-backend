package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
)

type LightwellAdvisoryHandler struct {
	Store store.Querier
}

func RegisterLightwellAdvisoryRoutes(engine *echo.Group, querier store.Querier) {
	h := LightwellAdvisoryHandler{Store: querier}
	addRepoRoute(engine, http.MethodGet, "/lightwell/advisories", h.list, rbac.RbacVerbRead)
}

// listLightwellAdvisories godoc
// @Summary      List Lightwell Advisories
// @ID           listLightwellAdvisories
// @Description  List security advisories for Lightwell remediated packages with optional filtering.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        repository_uuid  query  string  false  "Filter by repository UUID"
// @Param        package_name     query  string  false  "Filter by package name (substring match)"
// @Param        severity_min     query  string  false  "Minimum severity level (low, moderate, important, critical)"
// @Param        cve_id           query  string  false  "Filter by CVE ID (exact match)"
// @Param        limit            query  int     false  "Limit of results to return"
// @Param        offset           query  int     false  "Offset into results"
// @Success      200 {object} api.LightwellAdvisoryCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/advisories [get]
func (h *LightwellAdvisoryHandler) list(c echo.Context) error {
	page := ParsePagination(c)
	filters := parseLightwellAdvisoryFilters(c)

	severityMin, err := parseSeverityMin(filters.SeverityMin)
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid severity_min", err.Error())
	}

	var repoUUID pgtype.UUID
	if filters.RepositoryUUID != "" {
		parsed, err := uuid.Parse(filters.RepositoryUUID)
		if err != nil {
			return ce.NewErrorResponse(http.StatusBadRequest, "Invalid repository_uuid", err.Error())
		}
		repoUUID = pgtype.UUID{Bytes: parsed, Valid: true}
	}

	var packageName *string
	if filters.PackageName != "" {
		packageName = &filters.PackageName
	}

	var cveID *string
	if filters.CveID != "" {
		cveID = &filters.CveID
	}

	rows, err := h.Store.ListAdvisories(c.Request().Context(), store.ListAdvisoriesParams{
		RepositoryConfigUuid: repoUUID,
		PackageName:          packageName,
		SeverityMin:          severityMin,
		CveID:                cveID,
		PageOffset:           int32(page.Offset), //nolint:gosec // bounded by ParsePagination
		PageLimit:            int32(page.Limit),  //nolint:gosec // bounded by MaxLimit (200)
	})
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error listing advisories", err.Error())
	}

	var totalCount int64
	if len(rows) > 0 {
		totalCount = rows[0].TotalCount
	}

	resp := mapAdvisoryRowsToResponse(rows)
	collResp := setCollectionResponseMetadata(&resp, c, totalCount)
	return c.JSON(http.StatusOK, collResp)
}

func mapAdvisoryRowsToResponse(rows []store.ListAdvisoriesRow) api.LightwellAdvisoryCollectionResponse {
	data := make([]api.LightwellAdvisoryResponse, 0, len(rows))
	for _, row := range rows {
		refURLs := row.ReferenceUrls
		if refURLs == nil {
			refURLs = []string{}
		}
		fixedVersions := row.FixedVersions
		if fixedVersions == nil {
			fixedVersions = []string{}
		}
		data = append(data, api.LightwellAdvisoryResponse{
			AdvisoryID:    row.AdvisoryID,
			Severity:      row.Severity,
			Details:       row.Details,
			ReferenceURLs: refURLs,
			PackageName:   row.PackageName,
			FixedVersions: fixedVersions,
			Repository:    row.RepoName,
		})
	}
	return api.LightwellAdvisoryCollectionResponse{Data: data}
}

func parseLightwellAdvisoryFilters(c echo.Context) api.LightwellAdvisoryFilterData {
	var filters api.LightwellAdvisoryFilterData
	_ = echo.QueryParamsBinder(c).
		String("repository_uuid", &filters.RepositoryUUID).
		String("package_name", &filters.PackageName).
		String("severity_min", &filters.SeverityMin).
		String("cve_id", &filters.CveID).
		BindError()
	return filters
}

var severityMap = map[string]int16{
	"low":       1,
	"moderate":  2,
	"important": 3,
	"critical":  4,
}

func parseSeverityMin(s string) (pgtype.Int2, error) {
	if s == "" {
		return pgtype.Int2{}, nil
	}
	val, ok := severityMap[s]
	if !ok {
		return pgtype.Int2{}, &invalidSeverityError{severity: s}
	}
	return pgtype.Int2{Int16: val, Valid: true}, nil
}

type invalidSeverityError struct {
	severity string
}

func (e *invalidSeverityError) Error() string {
	return "invalid severity: " + e.severity + " (must be one of: low, moderate, important, critical)"
}
