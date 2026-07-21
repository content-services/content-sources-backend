package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

type AdminRepositoriesHandler struct {
	DaoRegistry dao.DaoRegistry
}

func checkAdminPartnerReposAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := CheckAdminPartnerRepositoriesAccessible(c.Request().Context()); err != nil {
			return err
		}
		return next(c)
	}
}

func RegisterAdminRepositoriesRoutes(engine *echo.Group, daoReg *dao.DaoRegistry) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("daoReg is nil")
	}

	adminRepositoriesHandler := AdminRepositoriesHandler{
		DaoRegistry: *daoReg,
	}

	addRepoRoute(engine, http.MethodPatch, "/admin/repositories/:uuid/partner", adminRepositoriesHandler.toggleAsPartner, rbac.RbacVerbWrite, checkAdminPartnerReposAccessible)
}

func (arh *AdminRepositoriesHandler) toggleAsPartner(c echo.Context) error {
	uuid := c.Param("uuid")
	var req api.SetPartnerRepositoryRequest
	if err := c.Bind(&req); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	if req.Partner == nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Partner field is required and cannot be empty", "Missing required field: partner")
	}
	if err := arh.DaoRegistry.RepositoryConfig.SetPartnerRepo(c.Request().Context(), uuid, *req.Partner); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error updating partner status", err.Error())
	}
	response, err := arh.DaoRegistry.RepositoryConfig.FetchWithoutOrgID(c.Request().Context(), uuid, false)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}
	return c.JSON(http.StatusOK, response)
}
