package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type AdminTaskHandler struct {
	DaoRegistry dao.DaoRegistry
}

func checkAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := CheckAdminTaskAccessible(c.Request().Context()); err != nil {
			return err
		}
		return next(c)
	}
}

func RegisterAdminTaskRoutes(engine *echo.Group, daoReg *dao.DaoRegistry) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("taskInfoReg is nil")
	}

	adminTaskHandler := AdminTaskHandler{
		DaoRegistry: *daoReg,
	}
	addRoute(engine, http.MethodGet, "/admin/tasks/", adminTaskHandler.listTasks, rbac.RbacVerbRead, checkAccessible)
	addRoute(engine, http.MethodGet, "/admin/tasks/:uuid", adminTaskHandler.fetch, rbac.RbacVerbRead, checkAccessible)
}

func (adminTaskHandler *AdminTaskHandler) listTasks(c echo.Context) error {
	pageData := ParsePagination(c)
	filterData := ParseAdminTaskFilters(c)

	tasks, totalTasks, err := adminTaskHandler.DaoRegistry.AdminTask.List(pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing tasks", err.Error())
	}

	return c.JSON(http.StatusOK, setCollectionResponseMetadata(&tasks, c, totalTasks))
}

func (adminTaskHandler *AdminTaskHandler) fetch(c echo.Context) error {
	id := c.Param("uuid")

	response, err := adminTaskHandler.DaoRegistry.AdminTask.Fetch(id)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching task", err.Error())
	}
	return c.JSON(http.StatusOK, response)
}

func ParseAdminTaskFilters(c echo.Context) api.AdminTaskFilterData {
	filterData := api.AdminTaskFilterData{
		AccountId: DefaultAccountId,
		OrgId:     DefaultOrgId,
		Status:    DefaultStatus,
	}
	err := echo.QueryParamsBinder(c).
		String("account_id", &filterData.AccountId).
		String("org_id", &filterData.OrgId).
		String("status", &filterData.Status).
		BindError()

	if err != nil {
		log.Error().Err(err).Msg("Error parsing filters")
	}

	return filterData
}
