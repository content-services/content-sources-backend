package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

type ModuleStreamsHandler struct {
	Dao dao.DaoRegistry
}

func RegisterModuleStreamsRoutes(engine *echo.Group, rDao *dao.DaoRegistry) {
	rh := ModuleStreamsHandler{
		Dao: *rDao,
	}

	addRepoRoute(engine, http.MethodPost, "/snapshots/module_streams/search", rh.searchSnapshotModuleStreams, rbac.RbacVerbRead)

}

// searchSnapshotModuleStreams godoc
// @Summary      List modules and their streams for snapshots
// @ID           searchSnapshotModuleStreams
// @Description  List modules and their streams for snapshots
// @Tags         module_streams
// @Accept       json
// @Produce      json
// @Param        body  body   api.SearchSnapshotModuleStreamsRequest  true  "request body"
// @Success      200   {object}  []api.SearchModuleStreams
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /snapshots/module_streams/search [post]
func (rh *ModuleStreamsHandler) searchSnapshotModuleStreams(c echo.Context) error {
	_, orgId := getAccountIdOrgId(c)

	dataInput := api.SearchSnapshotModuleStreamsRequest{}

	if err := c.Bind(&dataInput); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	apiResponse, err := rh.Dao.ModuleStreams.SearchSnapshotModuleStreams(c.Request().Context(), orgId, dataInput)

	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error searching modules streams", err.Error())
	}

	return c.JSON(200, apiResponse)
}
