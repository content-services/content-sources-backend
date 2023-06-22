package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
)

type AdminTaskHandler struct {
	DaoRegistry dao.DaoRegistry
}

func enforceAllowedAccount() func(next echo.HandlerFunc) echo.HandlerFunc {
	allowedAccounts := make(map[string]bool)
	for i := range config.Get().AdminTasks.AllowedAccounts {
		allowedAccounts[config.Get().AdminTasks.AllowedAccounts[i]] = true
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			accountId, _ := getAccountIdOrgId(c)
			if !allowedAccounts[accountId] {
				return ce.NewErrorResponse(http.StatusUnauthorized, "Unauthorized account", "Account ID is not included in config")
			}
			return next(c)
		}
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
	allowedAccountMiddleware := enforceAllowedAccount()
	engine.GET("/admin/tasks/", adminTaskHandler.listTasks, allowedAccountMiddleware)
	engine.GET("/admin/tasks/:uuid", adminTaskHandler.fetch, allowedAccountMiddleware)
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
