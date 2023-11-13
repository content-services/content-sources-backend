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
	addRoute(group, http.MethodGet, "/repositories/:uuid/snapshots/:snapshot_uuid/config.repo", sh.getRepoConfigurationFile, rbac.RbacVerbRead)
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
	_, orgID := getAccountIdOrgId(c)

	snapshots, totalSnaps, err := sh.DaoRegistry.Snapshot.WithContext(c.Request().Context()).List(orgID, uuid, pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing repository snapshots", err.Error())
	}
	return c.JSON(200, setCollectionResponseMetadata(&snapshots, c, totalSnaps))
}

// Get Snapshots godoc
// @Summary      Get configuration file of a repository
// @ID           getRepoConfigurationFile
// @Tags         repositories
// @Accept       json
// @Produce      text/plain
// @Param  uuid           path  string    true  "Identifier of the repository"
// @Param  snapshot_uuid  path  string    true  "Identifier of the snapshot"
// @Success      200   {string} string
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/snapshots/{snapshot_uuid}/config.repo [get]
func (sh *SnapshotHandler) getRepoConfigurationFile(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	uuid := c.Param("uuid")
	snapshotUUID := c.Param("snapshot_uuid")
	var repoConfigFile string

	repoConfigFile, err := sh.DaoRegistry.Snapshot.WithContext(c.Request().Context()).GetRepositoryConfigurationFile(orgID, snapshotUUID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error getting repository configuration file", err.Error())
	}

	return c.String(http.StatusOK, repoConfigFile)
}
