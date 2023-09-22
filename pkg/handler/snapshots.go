package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

type SnapshotHandler struct {
	DaoRegistry dao.DaoRegistry
}

func RegisterSnapshotRoutes(group *echo.Group, daoReg *dao.DaoRegistry) {
	if group == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("daoReg is nil")
	}

	sh := SnapshotHandler{DaoRegistry: *daoReg}
	addRoute(group, http.MethodGet, "/repositories/:uuid/snapshots/", sh.listSnapshots, rbac.RbacVerbRead)
}

// Get Snapshots godoc
// @Summary      List snapshots of a repository
// @ID           listSnapshots
// @Description  List snapshots of a repository.
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param  uuid  path  string    true  "Repository ID."
// @Success      200   {object}  api.SnapshotCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/snapshots/ [get]
func (sh *SnapshotHandler) listSnapshots(c echo.Context) error {
	uuid := c.Param("uuid")
	pageData := ParsePagination(c)
	filterData := ParseFilters(c)
	snapshots, totalSnaps, err := sh.DaoRegistry.Snapshot.List(uuid, pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing repository snapshots", err.Error())
	}
	return c.JSON(200, setCollectionResponseMetadata(&snapshots, c, totalSnaps))
}
