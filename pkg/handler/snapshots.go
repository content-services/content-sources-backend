package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/labstack/echo/v4"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
)

// SnapshotByDateQueryLimit - Max number of repository snapshots permitted to query at a time by date.
const SnapshotByDateQueryLimit = 1000

type SnapshotHandler struct {
	DaoRegistry dao.DaoRegistry
	TaskClient  client.TaskClient
}

func RegisterSnapshotRoutes(group *echo.Group, daoReg *dao.DaoRegistry, taskClient *client.TaskClient) {
	if group == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("daoReg is nil")
	}
	if taskClient == nil {
		panic("taskClient is nil")
	}

	sh := SnapshotHandler{
		DaoRegistry: *daoReg,
		TaskClient:  *taskClient,
	}

	addRepoRoute(group, http.MethodPost, "/snapshots/for_date/", sh.listSnapshotsByDate, rbac.RbacVerbRead)
	addRepoRoute(group, http.MethodGet, "/repositories/:uuid/snapshots/", sh.listSnapshotsForRepo, rbac.RbacVerbRead)
	addRepoRoute(group, http.MethodGet, "/repositories/:uuid/config.repo", sh.getLatestRepoConfigurationFile, rbac.RbacVerbRead)
	addRepoRoute(group, http.MethodGet, "/snapshots/:snapshot_uuid/config.repo", sh.getRepoConfigurationFile, rbac.RbacVerbRead)
	addRepoRoute(group, http.MethodGet, "/templates/:uuid/snapshots/", sh.listSnapshotsForTemplate, rbac.RbacVerbRead)
	addRepoRoute(group, http.MethodDelete, "/repositories/:repo_uuid/snapshots/:snapshot_uuid", sh.deleteSnapshot, rbac.RbacVerbWrite)
	addRepoRoute(group, http.MethodPost, "/repositories/:repo_uuid/snapshots/bulk_delete/", sh.bulkDeleteSnapshot, rbac.RbacVerbWrite)
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
// @Param        uuid path string true "Repository ID."
// @Param		 sort_by query string false "Sort the response data based on specific repository parameters. Sort criteria can include `created_at`."
// @Param  		 offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param		 limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Success      200 {object} api.SnapshotCollectionResponse
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = &ce.DaoError{NotFound: true, Message: "Could not find repository with UUID " + repoUUID}
		}
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

// DeleteSnapshot godoc
// @summary 		Delete a snapshot
// @ID				deleteSnapshot
// @Description     This enables deleting a specific snapshot.
// @Tags			snapshots
// @Param  			repo_uuid path string true "Repository UUID."
// @Param  			snapshot_uuid path string true "Snapshot UUID."
// @Success			204 "Snapshot was successfully deleted"
// @Failure      	400 {object} ce.ErrorResponse
// @Failure     	401 {object} ce.ErrorResponse
// @Failure      	404 {object} ce.ErrorResponse
// @Failure      	500 {object} ce.ErrorResponse
// @Router			/repositories/{repo_uuid}/snapshots/{snapshot_uuid} [delete]
func (sh *SnapshotHandler) deleteSnapshot(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	repoUUID := c.Param("repo_uuid")
	snapshotUUID := c.Param("snapshot_uuid")

	err := sh.isDeleteAllowed(c, orgID, repoUUID, snapshotUUID)
	if err != nil {
		return err
	}

	err = sh.DaoRegistry.Snapshot.SoftDelete(c.Request().Context(), snapshotUUID)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error deleting snapshot", err.Error())
	}

	enqueueErr := sh.enqueueDeleteSnapshotsTask(c, orgID, repoUUID, snapshotUUID)
	if enqueueErr != nil {
		err = sh.DaoRegistry.Snapshot.ClearDeletedAt(c.Request().Context(), snapshotUUID)
		if err != nil {
			return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error clearing deleted_at field", err.Error())
		}
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(enqueueErr), "Error enqueueing task", enqueueErr.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// BulkDeleteSnapshots godoc
// @summary 		Bulk delete a snapshots
// @ID				bulkDeleteSnapshots
// @Description     This enables deleting specified snapshots from a repository.
// @Tags			snapshots
// @Param  			repo_uuid path string true "Repository UUID."
// @Param       	body body api.UUIDListRequest true "Identifiers of the snapshots"
// @Success			204 "Snapshots were successfully deleted"
// @Failure      	400 {object} ce.ErrorResponse
// @Failure     	401 {object} ce.ErrorResponse
// @Failure      	404 {object} ce.ErrorResponse
// @Failure      	500 {object} ce.ErrorResponse
// @Router			/repositories/{repo_uuid}/snapshots/bulk_delete/ [post]
func (sh *SnapshotHandler) bulkDeleteSnapshot(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	repoUUID := c.Param("repo_uuid")
	var body api.UUIDListRequest
	var snapshotUUIDs []string
	err := c.Bind(&body)
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	snapshotUUIDs = body.UUIDs

	if len(snapshotUUIDs) == 0 {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error deleting snapshots", "Request body must contain at least 1 snapshot UUID to delete.")
	}
	if BulkDeleteLimit < len(snapshotUUIDs) {
		limitErrMsg := fmt.Sprintf("Cannot delete more than %d snapshots at once.", BulkDeleteLimit)
		return ce.NewErrorResponse(http.StatusRequestEntityTooLarge, "Error deleting repositories", limitErrMsg)
	}

	err = sh.isDeleteAllowed(c, orgID, repoUUID, snapshotUUIDs...)
	if err != nil {
		return err
	}

	errs := sh.DaoRegistry.Snapshot.BulkDelete(c.Request().Context(), snapshotUUIDs)
	if len(errs) > 0 {
		return ce.NewErrorResponseFromError("Error deleting snapshots", errs...)
	}

	enqueueErr := sh.enqueueDeleteSnapshotsTask(c, orgID, repoUUID, snapshotUUIDs...)
	if enqueueErr != nil {
		hasErr := false
		errs = make([]error, len(snapshotUUIDs))
		for i := range snapshotUUIDs {
			err = sh.DaoRegistry.Snapshot.ClearDeletedAt(c.Request().Context(), snapshotUUIDs[i])
			if err != nil {
				hasErr = true
				errs[i] = err
			}
		}
		if hasErr {
			return ce.NewErrorResponseFromError("Error clearing snapshot deleted_at fields", errs...)
		}
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(enqueueErr), "Error enqueueing task", enqueueErr.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

func (sh *SnapshotHandler) enqueueDeleteSnapshotsTask(c echo.Context, orgID, repoUUID string, snapshotUUIDs ...string) error {
	accountID, _ := getAccountIdOrgId(c)
	payload := payloads.DeleteSnapshotsPayload{RepoUUID: repoUUID, SnapshotsUUIDs: snapshotUUIDs}
	task := queue.Task{
		Typename:   config.DeleteSnapshotsTask,
		Payload:    payload,
		OrgId:      orgID,
		AccountId:  accountID,
		ObjectUUID: utils.Ptr(repoUUID),
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		RequestID:  c.Response().Header().Get(config.HeaderRequestId),
	}

	taskID, err := sh.TaskClient.Enqueue(task)
	if err != nil {
		logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
		logger.Error().Msg("error enqueuing task")
		return err
	}

	return nil
}

func (sh *SnapshotHandler) isDeleteInProgress(c echo.Context, orgID, repoUUID string) error {
	inProgressTasks, err := sh.DaoRegistry.TaskInfo.
		FetchActiveTasks(c.Request().Context(), orgID, repoUUID, config.DeleteRepositorySnapshotsTask, config.DeleteSnapshotsTask)

	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching delete repository snapshots task", err.Error())
	}
	if len(inProgressTasks) >= 1 {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error deleting snapshot", "Delete is already in progress")
	}

	return nil
}

func (sh *SnapshotHandler) isDeleteAllowed(c echo.Context, orgID, repoUUID string, snapshotUUIDs ...string) error {
	repo, err := sh.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), orgID, repoUUID)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository config", err.Error())
	}

	if repo.OrgID == config.RedHatOrg {
		return ce.NewErrorResponse(http.StatusNotFound, "Error fetching repository config", "Could not find repository with UUID "+repoUUID)
	}

	err = sh.isDeleteInProgress(c, orgID, repoUUID)
	if err != nil {
		return err
	}

	repoSnaps, err := sh.DaoRegistry.Snapshot.FetchForRepoConfigUUID(c.Request().Context(), repoUUID)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching snapshots", err.Error())
	}

	hasErr := false
	errs := make([]error, len(snapshotUUIDs))
	for i := range snapshotUUIDs {
		err = sh.isSnapInRepo(repoSnaps, snapshotUUIDs[i])
		if err != nil {
			hasErr = true
			errs[i] = err
			continue
		}
	}
	if hasErr {
		return ce.NewErrorResponseFromError("Error deleting snapshots", errs...)
	}

	if len(repoSnaps) <= len(snapshotUUIDs) {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error deleting snapshots", "Can't delete all the snapshots in the repository")
	}

	return nil
}

func (sh *SnapshotHandler) isSnapInRepo(repoSnaps []models.Snapshot, uuid string) error {
	if slices.IndexFunc(repoSnaps, func(snapshot models.Snapshot) bool { return snapshot.UUID == uuid }) == -1 {
		return &ce.DaoError{
			NotFound: true,
			Err:      errors.New("snapshot with this UUID does not exist for the specified repository"),
		}
	}

	return nil
}
