package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
)

const BulkCreateLimit = 20
const BulkDeleteLimit = 100

type RepositoryHandler struct {
	DaoRegistry dao.DaoRegistry
	TaskClient  client.TaskClient
}

func RegisterRepositoryRoutes(engine *echo.Group, daoReg *dao.DaoRegistry,
	taskClient *client.TaskClient) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("daoReg is nil")
	}

	if taskClient == nil {
		panic("taskClient is nil")
	}
	rh := RepositoryHandler{
		DaoRegistry: *daoReg,
		TaskClient:  *taskClient,
	}

	addRoute(engine, http.MethodGet, "/repositories/", rh.listRepositories, rbac.RbacVerbRead)
	addRoute(engine, http.MethodGet, "/repositories/:uuid", rh.fetch, rbac.RbacVerbRead)
	addRoute(engine, http.MethodPut, "/repositories/:uuid", rh.fullUpdate, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodPatch, "/repositories/:uuid", rh.partialUpdate, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodDelete, "/repositories/:uuid", rh.deleteRepository, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodPost, "/repositories/bulk_delete/", rh.bulkDeleteRepositories, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodPost, "/repositories/", rh.createRepository, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodPost, "/repositories/bulk_create/", rh.bulkCreateRepositories, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodPost, "/repositories/:uuid/snapshot/", rh.createSnapshot, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodPost, "/repositories/:uuid/introspect/", rh.introspect, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodGet, "/repository_gpg_key/:uuid", rh.getGpgKeyFile, rbac.RbacVerbRead)
}

func getAccountIdOrgId(c echo.Context) (string, string) {
	data := identity.GetIdentity(c.Request().Context())
	return data.Identity.AccountNumber, data.Identity.Internal.OrgID
}

// ListRepositories godoc
// @Summary      List Repositories
// @ID           listRepositories
// @Description  This operation enables users to retrieve a list of repositories.
// @Tags         repositories
// @Param		 offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param		 limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Param		 version query string false "A comma separated list of release versions to filter on. For example, `1,2` would return repositories with versions 1 or 2 only."
// @Param		 arch query string false "A comma separated list of architectures or platforms for that you want to retrieve repositories. It controls responses where repositories support multiple architectures or platforms. For example, ‘x86_64,s390x' returns repositories with `x86_64` or `s390x` only."
// @Param		 available_for_version query string false "Filter repositories by supported release version. For example, `1` returns repositories with the version `1` or where version is not set."
// @Param		 available_for_arch query string false "Filter repositories by architecture. For example, `x86_64` returns repositories with the version `x86_64` or where architecture is not set."
// @Param		 search query string false "Term to filter and retrieve items that match the specified search criteria. Search term can include name or URL."
// @Param		 name query string false "Filter repositories by name."
// @Param		 url query string false "A comma separated list of URLs to control api response."
// @Param		 uuid query string false "A comma separated list of UUIDs to control api response."
// @Param		 sort_by query string false "Sort the response data based on specific repository parameters. Sort criteria can include `name`, `url`, `status`, and `package_count`."
// @Param        status query string false "A comma separated list of statuses to control api response. Statuses can include `pending`, `valid`, `invalid`, `unavailable`."
// @Param		 origin query string false "A comma separated list of origins to filter api response. Origins can include `red_hat` and `external`."
// @Param		 content_type query string false "content type of a repository to filter on (rpm)"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.RepositoryCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/ [get]
func (rh *RepositoryHandler) listRepositories(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	c.Logger().Infof("org_id: %s", orgID)
	pageData := ParsePagination(c)
	filterData := ParseFilters(c)

	repos, totalRepos, err := rh.DaoRegistry.RepositoryConfig.List(c.Request().Context(), orgID, pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing repositories", err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&repos, c, totalRepos))
}

// CreateRepository godoc
// @Summary      Create Repository
// @ID           createRepository
// @Description  This operation enables creating custom repositories based on user preferences.
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param        body  body     api.RepositoryRequest  true  "request body"
// @Success      201  {object}  api.RepositoryResponse
// @Header       201  {string}  Location "resource URL"
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/ [post]
func (rh *RepositoryHandler) createRepository(c echo.Context) error {
	var (
		newRepository api.RepositoryRequest
		err           error
	)
	if err = c.Bind(&newRepository); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding params", err.Error())
	}

	accountID, orgID := getAccountIdOrgId(c)
	newRepository.AccountID = &accountID
	newRepository.OrgID = &orgID
	newRepository.FillDefaults()

	if err = rh.CheckSnapshotForRepos(c, orgID, []api.RepositoryRequest{newRepository}); err != nil {
		return err
	}

	var response api.RepositoryResponse
	if response, err = rh.DaoRegistry.RepositoryConfig.Create(c.Request().Context(), newRepository); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error creating repository", err.Error())
	}
	if response.Snapshot {
		rh.enqueueSnapshotEvent(c, &response)
	}
	rh.enqueueIntrospectEvent(c, response, orgID)

	c.Response().Header().Set("Location", "/api/"+config.DefaultAppName+"/v1.0/repositories/"+response.UUID)
	return c.JSON(http.StatusCreated, &response)
}

// CreateRepository godoc
// @Summary      Bulk create repositories
// @ID           bulkCreateRepositories
// @Description  This enables creating multiple repositories in a single API. If a user encounters any error, none of the repositories will be created. The applicable error message will be returned.
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param        body  body     []api.RepositoryRequest  true  "request body"
// @Success      201  {object}  []api.RepositoryResponse
// @Header       201  {string}  Location "resource URL"
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/bulk_create/ [post]
func (rh *RepositoryHandler) bulkCreateRepositories(c echo.Context) error {
	var newRepositories []api.RepositoryRequest
	if err := c.Bind(&newRepositories); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	if BulkCreateLimit < len(newRepositories) {
		limitErrMsg := fmt.Sprintf("Cannot create more than %d repositories at once.", BulkCreateLimit)
		return ce.NewErrorResponse(http.StatusRequestEntityTooLarge, "Error creating repositories", limitErrMsg)
	}

	accountID, orgID := getAccountIdOrgId(c)
	for i := 0; i < len(newRepositories); i++ {
		newRepositories[i].AccountID = &accountID
		newRepositories[i].OrgID = &orgID
		newRepositories[i].FillDefaults()
	}

	if err := rh.CheckSnapshotForRepos(c, orgID, newRepositories); err != nil {
		return err
	}

	responses, errs := rh.DaoRegistry.RepositoryConfig.BulkCreate(c.Request().Context(), newRepositories)
	if len(errs) > 0 {
		return ce.NewErrorResponseFromError("Error creating repository", errs...)
	}

	// Produce an event for each repository
	for index, repo := range responses {
		if repo.Snapshot {
			rh.enqueueSnapshotEvent(c, &responses[index])
		}

		rh.enqueueIntrospectEvent(c, repo, orgID)
		log.Info().Msgf("bulkCreateRepositories produced IntrospectRequest event")
	}

	return c.JSON(http.StatusCreated, responses)
}

// Get RepositoryResponse godoc
// @Summary      Get Repository
// @ID           getRepository
// @Description  Get repository information.
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param  uuid  path  string    true  "Repository ID."
// @Success      200   {object}  api.RepositoryResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid} [get]
func (rh *RepositoryHandler) fetch(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	uuid := c.Param("uuid")

	response, err := rh.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}
	return c.JSON(http.StatusOK, response)
}

// FullUpdateRepository godoc
// @Summary      Update Repository
// @ID           fullUpdateRepository
// @Description  Update a repository.
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param  uuid       path    string  true  "Repository ID."
// @Param  		 body body    api.RepositoryRequest true  "request body"
// @Success      200 {object}  api.RepositoryResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid} [put]
func (rh *RepositoryHandler) fullUpdate(c echo.Context) error {
	return rh.update(c, true)
}

// Update godoc
// @Summary      Partial Update Repository
// @ID           partialUpdateRepository
// @Description  Partially update a repository.
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param  uuid       path    string  true  "Repository ID."
// @Param        body       body    api.RepositoryRequest true  "request body"
// @Success      200 {object}  api.RepositoryResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid} [patch]
func (rh *RepositoryHandler) partialUpdate(c echo.Context) error {
	return rh.update(c, false)
}

func (rh *RepositoryHandler) update(c echo.Context, fillDefaults bool) error {
	uuid := c.Param("uuid")
	repoParams := api.RepositoryRequest{}
	_, orgID := getAccountIdOrgId(c)

	if err := c.Bind(&repoParams); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	if err := rh.CheckSnapshotForRepos(c, orgID, []api.RepositoryRequest{repoParams}); err != nil {
		return err
	}
	if fillDefaults {
		repoParams.FillDefaults()
	}

	repoConfig, err := rh.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	urlUpdated, err := rh.DaoRegistry.RepositoryConfig.Update(c.Request().Context(), orgID, uuid, repoParams)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error updating repository", err.Error())
	}

	if urlUpdated {
		snapInProgress, err := rh.DaoRegistry.TaskInfo.IsSnapshotInProgress(c.Request().Context(), orgID, repoConfig.RepositoryUUID)
		if err != nil {
			return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error checking if snapshot is in progress", err.Error())
		}
		if snapInProgress {
			err = rh.TaskClient.SendCancelNotification(c.Request().Context(), repoConfig.LastSnapshotTaskUUID)
			if err != nil {
				return ce.NewErrorResponse(http.StatusInternalServerError, "Error canceling previous snapshot", err.Error())
			}
		}
	}

	response, err := rh.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), orgID, uuid)
	if urlUpdated && response.Snapshot {
		rh.enqueueSnapshotEvent(c, &response)
	}

	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if urlUpdated {
		rh.enqueueIntrospectEvent(c, response, orgID)
	}

	return c.JSON(http.StatusOK, response)
}

// DeleteRepository godoc
// @summary 		Delete a repository
// @ID				deleteRepository
// @Description     This enables deleting a specific repository.
// @Tags			repositories
// @Param  			uuid       path    string  true  "Repository ID."
// @Success			204 "Repository was successfully deleted"
// @Failure      	400 {object} ce.ErrorResponse
// @Failure     	401 {object} ce.ErrorResponse
// @Failure      	404 {object} ce.ErrorResponse
// @Failure      	500 {object} ce.ErrorResponse
// @Router			/repositories/{uuid} [delete]
func (rh *RepositoryHandler) deleteRepository(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	uuid := c.Param("uuid")

	repoConfig, err := rh.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	snapInProgress, err := rh.DaoRegistry.TaskInfo.IsSnapshotInProgress(c.Request().Context(), orgID, repoConfig.RepositoryUUID)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error checking if snapshot is in progress", err.Error())
	}
	if snapInProgress {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot delete repository while snapshot is in progress", "")
	}
	if err := rh.DaoRegistry.RepositoryConfig.SoftDelete(c.Request().Context(), orgID, uuid); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error deleting repository", err.Error())
	}
	rh.enqueueSnapshotDeleteEvent(c, orgID, repoConfig)

	return c.NoContent(http.StatusNoContent)
}

// BulkDeleteRepositories godoc
// @Summary      Bulk delete repositories
// @ID           bulkDeleteRepositories
// @Description  This enables deleting multiple repositories.
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param        body  body     api.UUIDListRequest  true  "Identifiers of the repositories"
// @Success			 204 "Repositories were successfully deleted"
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/bulk_delete/ [post]
func (rh *RepositoryHandler) bulkDeleteRepositories(c echo.Context) error {
	var body api.UUIDListRequest
	if err := c.Bind(&body); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	uuids := body.UUIDs

	if len(uuids) == 0 {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error deleting repositories", "Request body must contain at least 1 repository UUID to delete.")
	}

	if BulkDeleteLimit < len(uuids) {
		limitErrMsg := fmt.Sprintf("Cannot delete more than %d repositories at once.", BulkDeleteLimit)
		return ce.NewErrorResponse(http.StatusRequestEntityTooLarge, "Error deleting repositories", limitErrMsg)
	}

	_, orgID := getAccountIdOrgId(c)

	responses := make([]api.RepositoryResponse, len(uuids))
	hasErr := false
	errs := make([]error, len(uuids))
	for i := range uuids {
		repoConfig, err := rh.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), orgID, uuids[i])
		responses[i] = repoConfig
		if err != nil {
			hasErr = true
			errs[i] = err
			continue
		}

		snapInProgress, err := rh.DaoRegistry.TaskInfo.IsSnapshotInProgress(c.Request().Context(), orgID, repoConfig.RepositoryUUID)
		if err != nil {
			hasErr = true
			errs[i] = err
			continue
		}
		if snapInProgress {
			hasErr = true
			// To get status code 400
			errs[i] = &ce.DaoError{
				BadValidation: true,
				Message:       "Cannot delete repository while snapshot is in progress",
			}
			continue
		}
	}
	if hasErr {
		return ce.NewErrorResponseFromError("Error deleting repositories", errs...)
	}

	errs = rh.DaoRegistry.RepositoryConfig.BulkDelete(c.Request().Context(), orgID, uuids)
	if len(errs) > 0 {
		return ce.NewErrorResponseFromError("Error deleting repositories", errs...)
	}

	for i := range responses {
		rh.enqueueSnapshotDeleteEvent(c, orgID, responses[i])
	}

	return c.NoContent(http.StatusNoContent)
}

// SnapshotRepository godoc
// @summary 		snapshot a repository
// @ID				createSnapshot
// @Description     Snapshot a repository if not already snapshotting
// @Tags			repositories
// @Param  			uuid            path    string                          true   "Repository ID."
// @Success			204 "Snapshot was successfully queued"
// @Failure      	400 {object} ce.ErrorResponse
// @Failure      	404 {object} ce.ErrorResponse
// @Failure      	500 {object} ce.ErrorResponse
// @Router			/repositories/{uuid}/snapshot/ [post]
func (rh *RepositoryHandler) createSnapshot(c echo.Context) error {
	if err := CheckSnapshotAccessible(c.Request().Context()); err != nil {
		return err
	}
	uuid := c.Param("uuid")
	_, orgID := getAccountIdOrgId(c)
	response, err := rh.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	inProgress, err := rh.DaoRegistry.TaskInfo.IsSnapshotInProgress(c.Request().Context(), orgID, response.RepositoryUUID)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error checking snapshot task", err.Error())
	}

	if inProgress {
		return ce.NewErrorResponse(http.StatusConflict, "Error snapshotting repository", "This repository is currently being snapshotted.")
	}

	if !response.Snapshot {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error snapshotting repository", "Snapshotting not yet enabled for this repository.")
	}

	rh.enqueueSnapshotEvent(c, &response)

	return c.NoContent(http.StatusNoContent)
}

// IntrospectRepository godoc
// @summary 		introspect a repository
// @ID				introspect
// @Description     Check for repository updates.
// @Tags			repositories
// @Param  			uuid            path    string                          true   "Repository ID."
// @Param			body            body    api.RepositoryIntrospectRequest false  "request body"
// @Success			204 "Introspection was successfully queued"
// @Failure      	400 {object} ce.ErrorResponse
// @Failure      	404 {object} ce.ErrorResponse
// @Failure      	500 {object} ce.ErrorResponse
// @Router			/repositories/{uuid}/introspect/ [post]
func (rh *RepositoryHandler) introspect(c echo.Context) error {
	var req api.RepositoryIntrospectRequest

	_, orgID := getAccountIdOrgId(c)
	uuid := c.Param("uuid")

	if err := c.Bind(&req); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	response, err := rh.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	repo, err := rh.DaoRegistry.Repository.FetchForUrl(c.Request().Context(), response.URL)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository uuid", err.Error())
	}

	if repo.LastIntrospectionTime != nil {
		limit := time.Second * time.Duration(config.Get().Options.IntrospectApiTimeLimitSec)
		since := time.Since(*repo.LastIntrospectionTime)
		if since < limit {
			detail := fmt.Sprintf("This repository has been introspected recently. Try again in %v", (limit - since).Truncate(time.Second))
			return ce.NewErrorResponse(http.StatusBadRequest, "Error introspecting repository", detail)
		}
	}

	if repo.FailedIntrospectionsCount >= config.FailedIntrospectionsLimit+1 && !req.ResetCount {
		return ce.NewErrorResponse(http.StatusBadRequest, "Too many failed introspections",
			fmt.Sprintf("This repository has failed introspecting %v times.", repo.FailedIntrospectionsCount))
	}

	var repoUpdate dao.RepositoryUpdate
	count := 0
	lastIntrospectionStatus := "Pending"
	if req.ResetCount {
		repoUpdate = dao.RepositoryUpdate{
			UUID:                      repo.UUID,
			FailedIntrospectionsCount: &count,
			LastIntrospectionStatus:   &lastIntrospectionStatus,
		}
	} else {
		repoUpdate = dao.RepositoryUpdate{
			UUID:                    repo.UUID,
			LastIntrospectionStatus: &lastIntrospectionStatus,
		}
	}

	if err := rh.DaoRegistry.Repository.Update(c.Request().Context(), repoUpdate); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error resetting failed introspections count", err.Error())
	}

	rh.enqueueIntrospectEvent(c, response, orgID)

	return c.NoContent(http.StatusNoContent)
}

// Update godoc
// @Summary      Get the GPG key file for a repository
// @ID           getGpgKeyFile
// @Description  Get the GPG key file for a repository.
// @Tags         repositories
// @Accept       json
// @Produce      text/plain
// @Param  uuid       path    string  true  "Repository ID."
// @Success      200 {object}  string
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repository_gpg_key/{uuid} [get]
func (rh *RepositoryHandler) getGpgKeyFile(c echo.Context) error {
	uuid := c.Param("uuid")

	resp, err := rh.DaoRegistry.RepositoryConfig.FetchWithoutOrgID(c.Request().Context(), uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}
	if resp.GpgKey == "" {
		errMsg := fmt.Errorf("no GPG key found for this repository")
		return ce.NewErrorResponse(http.StatusNotFound, "Error fetching gpg key", errMsg.Error())
	}
	return c.String(http.StatusOK, resp.GpgKey)
}

// enqueueSnapshotEvent queues up a snapshot for a given repository uuid (not repository config) and org.
func (rh *RepositoryHandler) enqueueSnapshotEvent(c echo.Context, response *api.RepositoryResponse) {
	if config.PulpConfigured() {
		task := queue.Task{
			Typename:       config.RepositorySnapshotTask,
			Payload:        payloads.SnapshotPayload{},
			OrgId:          response.OrgID,
			RepositoryUUID: &response.RepositoryUUID,
			RequestID:      c.Response().Header().Get(config.HeaderRequestId),
			AccountId:      response.AccountID,
		}
		taskID, err := rh.TaskClient.Enqueue(task)
		logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
		if err != nil {
			logger.Error().Msg("error enqueuing task")
		}
		if err == nil {
			if err := rh.DaoRegistry.RepositoryConfig.UpdateLastSnapshotTask(c.Request().Context(), taskID.String(), response.OrgID, response.RepositoryUUID); err != nil {
				logger.Error().Err(err).Msgf("error UpdatingLastSnapshotTask task")
			} else {
				response.LastSnapshotTaskUUID = taskID.String()
			}
		}
	}
}

func (rh *RepositoryHandler) enqueueSnapshotDeleteEvent(c echo.Context, orgID string, repo api.RepositoryResponse) {
	payload := tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repo.UUID}
	task := queue.Task{
		Typename:       config.DeleteRepositorySnapshotsTask,
		Payload:        payload,
		OrgId:          orgID,
		AccountId:      repo.AccountID,
		RepositoryUUID: &repo.RepositoryUUID,
		RequestID:      c.Response().Header().Get(config.HeaderRequestId),
	}
	taskID, err := rh.TaskClient.Enqueue(task)
	if err != nil {
		logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
		logger.Error().Msg("error enqueuing task")
	}
}

func (rh *RepositoryHandler) enqueueIntrospectEvent(c echo.Context, response api.RepositoryResponse, orgID string) {
	var err error
	task := queue.Task{
		Typename:       payloads.Introspect,
		Payload:        payloads.IntrospectPayload{Url: response.URL, Force: true},
		OrgId:          orgID,
		AccountId:      response.AccountID,
		RepositoryUUID: &response.RepositoryUUID,
		RequestID:      c.Response().Header().Get(config.HeaderRequestId),
	}
	taskID, err := rh.TaskClient.Enqueue(task)
	if err != nil {
		logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
		logger.Error().Msg("error enqueuing task")
	}
}

// CheckSnapshotForRepos checks if for a given RepositoryRequest, snapshotting can be done
func (rh *RepositoryHandler) CheckSnapshotForRepos(c echo.Context, orgId string, repos []api.RepositoryRequest) error {
	for _, repo := range repos {
		if repo.Snapshot != nil && *repo.Snapshot {
			if err := CheckSnapshotAccessible(c.Request().Context()); err != nil {
				return err
			}
		}
	}
	return nil
}
