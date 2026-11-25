package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

const demandSignalDateFormat = "2006-01-02"

type AdminLensHandler struct {
	DaoRegistry dao.DaoRegistry
}

func checkAdminLensDemandSignalsAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := CheckAdminLensDemandSignalsAccessible(c.Request().Context()); err != nil {
			return err
		}
		return next(c)
	}
}

func RegisterAdminLensRoutes(engine *echo.Group, daoReg *dao.DaoRegistry) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("daoReg is nil")
	}

	h := AdminLensHandler{DaoRegistry: *daoReg}
	addRepoRoute(engine, http.MethodGet, "/admin/lens/demand_signals/", h.listDemandSignals, rbac.RbacVerbRead, checkAdminLensDemandSignalsAccessible)
}

// listDemandSignals handles GET /admin/lens/demand_signals/.
//
// It returns Lens coverage demand signals aggregated across all orgs: one row per
// distinct (ecosystem, namespace, name, version) with the number of signals recorded
// for it. Signals are written when a Lens coverage analysis finds a package that is
// not (or only partially) covered, so the counts indicate which packages prospects
// are asking for that we should consider adding.
//
// Because the data is global and not scoped to the caller's org, the route is gated
// by the admin_lens_demand_signals feature (see CheckAdminLensDemandSignalsAccessible)
// and is intentionally left out of the public OpenAPI document.
//
// Query parameters (all optional):
//   - ecosystem: exact, case-sensitive match on the ecosystem (e.g. "Java", "Python").
//   - after_created_date: YYYY-MM-DD; include signals created at or after 00:00 UTC
//     on this day (inclusive).
//   - before_created_date: YYYY-MM-DD; include signals created before 00:00 UTC on
//     this day (exclusive). Pairing an inclusive "after" with an exclusive "before"
//     lets consecutive windows (e.g. month by month) tile with no gaps or overlap.
//   - limit, offset: standard pagination.
//
// Rows are ordered by namespace, name, version, then ecosystem. meta.count is the
// number of aggregated rows (groups), not the number of underlying signals.
//
// Responds 200 with an api.AdminDemandSignalCollectionResponse, or 400 if the
// feature is disabled, the caller is not allowed, or a date is not YYYY-MM-DD.
func (h *AdminLensHandler) listDemandSignals(c echo.Context) error {
	pageData := ParsePagination(c)
	filterData, err := ParseAdminDemandSignalFilters(c)
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid date filter", err.Error())
	}

	resp, total, err := h.DaoRegistry.CoverageReport.ListDemandSignals(c.Request().Context(), pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing demand signals", err.Error())
	}
	return c.JSON(http.StatusOK, SetCollectionResponseMetadata(&resp, c, total))
}

func ParseAdminDemandSignalFilters(c echo.Context) (api.AdminDemandSignalFilterData, error) {
	filterData := api.AdminDemandSignalFilterData{Ecosystem: c.QueryParam("ecosystem")}
	var err error
	if filterData.BeforeCreatedDate, err = parseDemandSignalDate(c, "before_created_date"); err != nil {
		return filterData, err
	}
	if filterData.AfterCreatedDate, err = parseDemandSignalDate(c, "after_created_date"); err != nil {
		return filterData, err
	}
	return filterData, nil
}

func parseDemandSignalDate(c echo.Context, param string) (*time.Time, error) {
	value := c.QueryParam(param)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(demandSignalDateFormat, value)
	if err != nil {
		return nil, fmt.Errorf("%s must be in YYYY-MM-DD format", param)
	}
	return &parsed, nil
}
