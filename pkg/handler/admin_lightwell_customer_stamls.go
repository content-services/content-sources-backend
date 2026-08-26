package handler

import (
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

type AdminLightwellCustomerStamlHandler struct {
	DaoRegistry dao.DaoRegistry
}

func checkAdminLightwellAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := CheckAdminLightwellAccessible(c.Request().Context()); err != nil {
			return err
		}
		return next(c)
	}
}

func RegisterAdminLightwellCustomerStamlRoutes(engine *echo.Group, daoReg *dao.DaoRegistry) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("daoReg is nil")
	}

	h := AdminLightwellCustomerStamlHandler{DaoRegistry: *daoReg}
	addRepoRoute(engine, http.MethodPost, "/admin/lightwell/customer-stamls/", h.create, rbac.RbacVerbWrite, checkAdminLightwellAccessible)
	addRepoRoute(engine, http.MethodDelete, "/admin/lightwell/customer-stamls/", h.delete, rbac.RbacVerbWrite, checkAdminLightwellAccessible)
}

func (h *AdminLightwellCustomerStamlHandler) create(c echo.Context) error {
	customerID, staml, err := bindLightwellCustomerStamlRequest(c)
	if err != nil {
		return err
	}

	resp, err := h.DaoRegistry.LightwellCustomerStaml.Create(c.Request().Context(), customerID, staml)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error creating STAML to CID mapping", err.Error())
	}
	return c.JSON(http.StatusCreated, resp)
}

func (h *AdminLightwellCustomerStamlHandler) delete(c echo.Context) error {
	customerID, staml, err := bindLightwellCustomerStamlRequest(c)
	if err != nil {
		return err
	}

	if err := h.DaoRegistry.LightwellCustomerStaml.Delete(c.Request().Context(), customerID, staml); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error deleting STAML to CID mapping", err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func bindLightwellCustomerStamlRequest(c echo.Context) (string, string, error) {
	var req api.LightwellCustomerStamlRequest
	if err := c.Bind(&req); err != nil {
		return "", "", ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	customerID := strings.TrimSpace(req.CustomerID)
	staml := strings.TrimSpace(req.Staml)
	if customerID == "" || staml == "" {
		return "", "", ce.NewErrorResponse(http.StatusBadRequest, "Missing required fields", "customer_id and staml are required")
	}
	return customerID, staml, nil
}
