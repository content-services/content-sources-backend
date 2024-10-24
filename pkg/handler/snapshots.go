package handler

import (
	"fmt"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

// SnapshotByDateQueryLimit - Max number of repository snapshots permitted to query at a time by date.
const SnapshotByDateQueryLimit = 1000

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
	addRepoRoute(group, http.MethodPost, "/snapshots/for_date/", sh.listSnapshotsByDate, rbac.RbacVerbRead)
	addRepoRoute(group, http.MethodGet, "/repositories/:uuid/snapshots/", sh.listSnapshotsForRepo, rbac.RbacVerbRead)
	addRepoRoute(group, http.MethodGet, "/repositories/:uuid/config.repo", sh.getLatestRepoConfigurationFile, rbac.RbacVerbRead)
	addRepoRoute(group, http.MethodGet, "/snapshots/:snapshot_uuid/config.repo", sh.getRepoConfigurationFile, rbac.RbacVerbRead)
	addRepoRoute(group, http.MethodGet, "/templates/:uuid/snapshots/", sh.listSnapshotsForTemplate, rbac.RbacVerbRead)
}

// Get Snapshots godoc
// @Summary      List snapshots for a template
// @ID           listSnapshotsForTemplate
// @Description  List snapshots for a template.
// @Tags         snapshots
// @Accept       json
// @Produce      json
// @Param  		 uuid 			   path  string true  "Template ID."
// @Param		 repository_search query string false "Search through snapshots by repository name."
// @Param		 sort_by query string false "Sort the response data based on specific snapshot parameters. Sort criteria can include `repository_name` or `created_at`."
// @Param		 offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param		 limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Success      200 {object} api.SnapshotCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /templates/{uuid}/snapshots/ [get]
func (sh *SnapshotHandler) listSnapshotsForTemplate(c echo.Context) error {
	uuid := c.Param("uuid")
	pageData := ParsePagination(c)
	_, orgID := getAccountIdOrgId(c)

	templateResponse, err := sh.DaoRegistry.Template.Fetch(c.Request().Context(), orgID, uuid, false)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching template", err.Error())
	}

	queryParams := c.QueryParams()
	repositorySearch := queryParams.Get("repository_search")
	snapshots, totalSnaps, err := sh.DaoRegistry.Snapshot.ListByTemplate(c.Request().Context(), orgID, templateResponse, repositorySearch, pageData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing snapshots for template", err.Error())
	}

	return c.JSON(http.StatusOK, setCollectionResponseMetadata(&snapshots, c, totalSnaps))
}

// Get Snapshots godoc
// @Summary      List snapshots of a repository
// @ID           listSnapshotsForRepo
// @Description  List snapshots of a repository.
// @Tags         snapshots
// @Accept       json
// @Produce      json
// @Param  uuid  path  string    true  "Repository ID."
// @Success      200   {object}  api.SnapshotCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/snapshots/ [get]
func (sh *SnapshotHandler) listSnapshotsForRepo(c echo.Context) error {
	uuid := c.Param("uuid")
	pageData := ParsePagination(c)
	filterData := ParseFilters(c)
	_, orgID := getAccountIdOrgId(c)

	snapshots, totalSnaps, err := sh.DaoRegistry.Snapshot.List(c.Request().Context(), orgID, uuid, pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing repository snapshots", err.Error())
	}
	return c.JSON(200, setCollectionResponseMetadata(&snapshots, c, totalSnaps))
}

// Get Snapshots godoc
// @Summary      Get latest configuration file for a repository
// @ID           getLatestRepoConfigurationFile
// @Tags         repositories
// @Accept       json
// @Produce      text/plain
// @Param  uuid  path  string    true  "Repository ID."
// @Success      200   {string} string
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/config.repo [get]
func (sh *SnapshotHandler) getLatestRepoConfigurationFile(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	repoUUID := c.Param("uuid")

	latestSnapshot, err := sh.DaoRegistry.Snapshot.FetchLatestSnapshot(c.Request().Context(), repoUUID)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching latest snapshot", err.Error())
	}

	repoConfigFile, err := sh.DaoRegistry.Snapshot.GetRepositoryConfigurationFile(c.Request().Context(), orgID, latestSnapshot.UUID, true)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error getting repository configuration file", err.Error())
	}

	return c.String(http.StatusOK, repoConfigFile)
}

// Post Snapshots godoc
// @Summary      Get nearest snapshot by date for a list of repositories.
// @ID           listSnapshotsByDate
// @Description  Get nearest snapshot by date for a list of repositories.
// @Tags         snapshots
// @Accept       json
// @Produce      json
// @Param        body  body    api.ListSnapshotByDateRequest  true  "request body"
// @Success      200 {object}  api.ListSnapshotByDateResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /snapshots/for_date/ [post]
func (sh *SnapshotHandler) listSnapshotsByDate(c echo.Context) error {
	var listSnapshotByDateParams api.ListSnapshotByDateRequest

	if err := c.Bind(&listSnapshotByDateParams); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	repoCount := len(listSnapshotByDateParams.RepositoryUUIDS)

	if SnapshotByDateQueryLimit < repoCount {
		limitErrMsg := fmt.Sprintf(
			"Cannot query more than %d repository_uuids at once, query contains %d repository_uuids",
			SnapshotByDateQueryLimit,
			repoCount,
		)
		return ce.NewErrorResponse(http.StatusRequestEntityTooLarge, "", limitErrMsg)
	} else if repoCount == 0 {
		badRequestMsg := fmt.Sprintf(
			"Query must contain between 1 and %d repository_uuids, query contains 0 repository_uuids",
			SnapshotByDateQueryLimit,
		)
		return ce.NewErrorResponse(http.StatusBadRequest, "", badRequestMsg)
	}

	_, orgID := getAccountIdOrgId(c)
	response, err := sh.DaoRegistry.Snapshot.FetchSnapshotsByDateAndRepository(c.Request().Context(), orgID, listSnapshotByDateParams)

	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching snapshots", err.Error())
	}

	return c.JSON(http.StatusOK, response)
}

// Get Snapshots godoc
// @Summary      Get configuration file of a repository
// @ID           getRepoConfigurationFile
// @Tags         repositories
// @Accept       json
// @Produce      text/plain
// @Param        snapshot_uuid  path  string    true  "Identifier of the snapshot"
// @Success      200   {string} string
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /snapshots/{snapshot_uuid}/config.repo [get]
func (sh *SnapshotHandler) getRepoConfigurationFile(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	snapshotUUID := c.Param("snapshot_uuid")

	repoConfigFile, err := sh.DaoRegistry.Snapshot.GetRepositoryConfigurationFile(c.Request().Context(), orgID, snapshotUUID, false)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error getting repository configuration file", err.Error())
	}

	return c.String(http.StatusOK, repoConfigFile)
}
