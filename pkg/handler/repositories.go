package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/event/adapter"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/content-services/content-sources-backend/pkg/event/producer"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/rs/zerolog/log"
)

const BulkCreateLimit = 20
const BulkDeleteLimit = 100

type RepositoryHandler struct {
	DaoRegistry               dao.DaoRegistry
	IntrospectRequestProducer producer.IntrospectRequest
	TaskClient                client.TaskClient
}

func RegisterRepositoryRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, prod *producer.IntrospectRequest, taskClient *client.TaskClient) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("daoReg is nil")
	}

	if prod == nil {
		panic("prod is nil")
	}
	if taskClient == nil {
		panic("taskClient is nil")
	}
	rh := RepositoryHandler{
		DaoRegistry:               *daoReg,
		IntrospectRequestProducer: *prod,
		TaskClient:                *taskClient,
	}

	addRoute(engine, http.MethodGet, "/repositories/", rh.listRepositories, rbac.RbacVerbRead)
	addRoute(engine, http.MethodGet, "/repositories/:uuid", rh.fetch, rbac.RbacVerbRead)
	addRoute(engine, http.MethodPut, "/repositories/:uuid", rh.fullUpdate, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodPatch, "/repositories/:uuid", rh.partialUpdate, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodDelete, "/repositories/:uuid", rh.deleteRepository, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodPost, "/repositories/bulk_delete/", rh.bulkDeleteRepositories, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodPost, "/repositories/", rh.createRepository, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodPost, "/repositories/bulk_create/", rh.bulkCreateRepositories, rbac.RbacVerbWrite)
	addRoute(engine, http.MethodPost, "/repositories/:uuid/introspect/", rh.introspect, rbac.RbacVerbWrite)
}

func GetIdentity(c echo.Context) (identity.XRHID, error) {
	// This block is a bit defensive as the read of the XRHID structure from the
	// context does not check if the value is a nil and

	if value := c.Request().Context().Value(identity.Key); value == nil {
		return identity.XRHID{}, fmt.Errorf("cannot find identity into the request context")
	}
	output := identity.Get(c.Request().Context())
	return output, nil
}

func getAccountIdOrgId(c echo.Context) (string, string) {
	var (
		data identity.XRHID
		err  error
	)
	if data, err = GetIdentity(c); err != nil {
		return "", ""
	}
	return data.Identity.AccountNumber, data.Identity.Internal.OrgID
}

// ListRepositories godoc
// @Summary      List Repositories
// @ID           listRepositories
// @Description  list repositories
// @Tags         repositories
// @Param		 offset query int false "Offset into the list of results to return in the response"
// @Param		 limit query int false "Limit the number of items returned"
// @Param		 version query string false "Comma separated list of architecture to optionally filter-on (e.g. 'x86_64,s390x' would return Repositories with x86_64 or s390x only)"
// @Param		 arch query string false "Comma separated list of versions to optionally filter-on  (e.g. '7,8' would return Repositories with versions 7 or 8 only)"
// @Param		 available_for_version query string false "Filter by compatible arch (e.g. 'x86_64' would return Repositories with the 'x86_64' arch and Repositories where arch is not set)"
// @Param		 available_for_arch query string false "Filter by compatible version (e.g. 7 would return Repositories with the version 7 or where version is not set)"
// @Param		 search query string false "Search term for name and url."
// @Param		 name query string false "Filter repositories by name using an exact match"
// @Param		 url query string false "Filter repositories by name using an exact match"
// @Param		 sort_by query string false "Sets the sort order of the results"
// @Param        status query string false "Comma separated list of statuses to optionally filter on"
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
	repos, totalRepos, err := rh.DaoRegistry.RepositoryConfig.List(orgID, pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing repositories", err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&repos, c, totalRepos))
}

// CreateRepository godoc
// @Summary      Create Repository
// @ID           createRepository
// @Description  create a repository
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

	if err = CheckSnapshotForRepo(c, newRepository); err != nil {
		return err
	}

	var response api.RepositoryResponse
	if response, err = rh.DaoRegistry.RepositoryConfig.Create(newRepository); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error creating repository", err.Error())
	}
	if response.Snapshot {
		rh.enqueueSnapshotEvent(response.RepositoryUUID, orgID)
	}
	rh.enqueueIntrospectEvent(c, response, orgID)

	c.Response().Header().Set("Location", "/api/"+config.DefaultAppName+"/v1.0/repositories/"+response.UUID)
	return c.JSON(http.StatusCreated, response)
}

// CreateRepository godoc
// @Summary      Bulk create repositories
// @ID           bulkCreateRepositories
// @Description  bulk create repositories
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
		if err := CheckSnapshotForRepo(c, newRepositories[i]); err != nil {
			return err
		}
		newRepositories[i].AccountID = &accountID
		newRepositories[i].OrgID = &orgID
		newRepositories[i].FillDefaults()
	}

	responses, errs := rh.DaoRegistry.RepositoryConfig.BulkCreate(newRepositories)
	if len(errs) > 0 {
		return ce.NewErrorResponseFromError("Error creating repository", errs...)
	}

	// Produce an event for each repository
	for _, repo := range responses {
		if repo.Snapshot {
			rh.enqueueSnapshotEvent(repo.RepositoryUUID, orgID)
		}

		rh.enqueueIntrospectEvent(c, repo, orgID)
		log.Info().Msgf("bulkCreateRepositories produced IntrospectRequest event")
	}

	return c.JSON(http.StatusCreated, responses)
}

// Get RepositoryResponse godoc
// @Summary      Get Repository
// @ID           getRepository
// @Description  Get information about a Repository
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param  uuid  path  string    true  "Identifier of the Repository"
// @Success      200   {object}  api.RepositoryResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid} [get]
func (rh *RepositoryHandler) fetch(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	uuid := c.Param("uuid")

	response, err := rh.DaoRegistry.RepositoryConfig.Fetch(orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}
	return c.JSON(http.StatusOK, response)
}

// FullUpdateRepository godoc
// @Summary      Update Repository
// @ID           fullUpdateRepository
// @Description  Fully update a repository
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param  uuid       path    string  true  "Identifier of the Repository"
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
// @Description  Partially Update a repository
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param  uuid       path    string  true  "Identifier of the Repository"
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
	if err := CheckSnapshotForRepo(c, repoParams); err != nil {
		return err
	}
	if fillDefaults {
		repoParams.FillDefaults()
	}

	repoConfig, err := rh.DaoRegistry.RepositoryConfig.Fetch(orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repoParams.URL != nil && repoConfig.URL != *repoParams.URL {
		snapInProgress, err := rh.DaoRegistry.TaskInfo.IsSnapshotInProgress(orgID, repoConfig.RepositoryUUID)
		if err != nil {
			return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error checking if snapshot is in progress", err.Error())
		}
		if snapInProgress {
			return ce.NewErrorResponse(http.StatusBadRequest, "Cannot update repository URL while snapshotting is in progress", "")
		}
	}

	urlUpdated, err := rh.DaoRegistry.RepositoryConfig.Update(orgID, uuid, repoParams)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error updating repository", err.Error())
	}

	response, err := rh.DaoRegistry.RepositoryConfig.Fetch(orgID, uuid)
	if urlUpdated && response.Snapshot {
		rh.enqueueSnapshotEvent(response.RepositoryUUID, orgID)
	}
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}
	rh.enqueueIntrospectEvent(c, response, orgID)

	return c.JSON(http.StatusOK, response)
}

// DeleteRepository godoc
// @summary 		Delete a repository
// @ID				deleteRepository
// @Tags			repositories
// @Param  			uuid       path    string  true  "Identifier of the Repository"
// @Success			204 "Repository was successfully deleted"
// @Failure      	400 {object} ce.ErrorResponse
// @Failure     	401 {object} ce.ErrorResponse
// @Failure      	404 {object} ce.ErrorResponse
// @Failure      	500 {object} ce.ErrorResponse
// @Router			/repositories/{uuid} [delete]
func (rh *RepositoryHandler) deleteRepository(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	uuid := c.Param("uuid")

	repoConfig, err := rh.DaoRegistry.RepositoryConfig.Fetch(orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	snapInProgress, err := rh.DaoRegistry.TaskInfo.IsSnapshotInProgress(orgID, repoConfig.RepositoryUUID)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error checking if snapshot is in progress", err.Error())
	}
	if snapInProgress {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot delete repository while snapshot is in progress", "")
	}
	if err := rh.DaoRegistry.RepositoryConfig.Delete(orgID, uuid); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error deleting repository", err.Error())
	}
	rh.enqueueSnapshotDeleteEvent(orgID, repoConfig)

	return c.NoContent(http.StatusNoContent)
}

// BulkDeleteRepositories godoc
// @Summary      Bulk delete repositories
// @ID           bulkDeleteRepositories
// @Description  bulk delete repositories
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
		repoConfig, err := rh.DaoRegistry.RepositoryConfig.Fetch(orgID, uuids[i])
		responses[i] = repoConfig
		if err != nil {
			hasErr = true
			errs[i] = err
			continue
		}

		snapInProgress, err := rh.DaoRegistry.TaskInfo.IsSnapshotInProgress(orgID, repoConfig.RepositoryUUID)
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

	errs = rh.DaoRegistry.RepositoryConfig.BulkDelete(orgID, uuids)
	if len(errs) > 0 {
		return ce.NewErrorResponseFromError("Error deleting repositories", errs...)
	}

	for i := range responses {
		rh.enqueueSnapshotDeleteEvent(orgID, responses[i])
	}

	return c.NoContent(http.StatusNoContent)
}

// IntrospectRepository godoc
// @summary 		introspect a repository
// @ID				introspect
// @Tags			repositories
// @Param  			uuid            path    string                          true   "Identifier of the Repository"
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

	response, err := rh.DaoRegistry.RepositoryConfig.Fetch(orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	repo, err := rh.DaoRegistry.Repository.FetchForUrl(response.URL)
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

	var repoUpdate dao.RepositoryUpdate
	count := 0
	status := "Pending"
	if req.ResetCount {
		repoUpdate = dao.RepositoryUpdate{
			UUID:                      repo.UUID,
			FailedIntrospectionsCount: &count,
			Status:                    &status,
		}
	} else {
		repoUpdate = dao.RepositoryUpdate{
			UUID:   repo.UUID,
			Status: &status,
		}
	}

	if err := rh.DaoRegistry.Repository.Update(repoUpdate); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error resetting failed introspections count", err.Error())
	}

	rh.enqueueIntrospectEvent(c, response, orgID)

	return c.NoContent(http.StatusNoContent)
}

// enqueueSnapshotEvent queues up a snapshot for a given repository uuid (not repository config) and org.
func (rh *RepositoryHandler) enqueueSnapshotEvent(repositoryUuid string, orgID string) {
	if config.Get().NewTaskingSystem && config.PulpConfigured() {
		task := queue.Task{Typename: config.RepositorySnapshotTask, Payload: payloads.SnapshotPayload{}, OrgId: orgID, RepositoryUUID: repositoryUuid}
		_, err := rh.TaskClient.Enqueue(task)
		if err != nil {
			log.Error().Err(err).Msgf("error enqueuing task for orgId %v, repository %v", orgID, repositoryUuid)
		}
	}
}

func (rh *RepositoryHandler) enqueueSnapshotDeleteEvent(orgID string, repo api.RepositoryResponse) {
	if config.Get().NewTaskingSystem && config.PulpConfigured() {
		payload := tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repo.UUID}
		task := queue.Task{
			Typename:       config.DeleteRepositorySnapshotsTask,
			Payload:        payload,
			OrgId:          orgID,
			RepositoryUUID: repo.RepositoryUUID,
		}
		_, err := rh.TaskClient.Enqueue(task)
		if err != nil {
			log.Error().Err(err).Msgf("error enqueuing task")
		}
	}
}

func (rh *RepositoryHandler) enqueueIntrospectEvent(c echo.Context, response api.RepositoryResponse, orgID string) {
	var msg *message.IntrospectRequestMessage
	var err error
	if config.Get().NewTaskingSystem {
		task := queue.Task{Typename: payloads.Introspect, Payload: payloads.IntrospectPayload{Url: response.URL, Force: true}, OrgId: orgID, RepositoryUUID: response.RepositoryUUID}
		_, err := rh.TaskClient.Enqueue(task)
		if err != nil {
			log.Error().Err(err).Msgf("error enqueuing tasks")
		}
	} else {
		if msg, err = adapter.NewIntrospect().FromRepositoryResponse(&response); err != nil {
			log.Error().Msgf("error mapping to event message: %s", err.Error())
		}
		if err = rh.IntrospectRequestProducer.Produce(c, msg); err != nil {
			log.Warn().Msgf("error producing event message: %s", err.Error())
		}
	}
}

// CheckSnapshotForRepo checks if for a given RepositoryRequest, snapshotting can be done
func CheckSnapshotForRepo(c echo.Context, repo api.RepositoryRequest) error {
	if repo.Snapshot != nil && *repo.Snapshot {
		if err := CheckSnapshotAccessible(c.Request().Context()); err != nil {
			return err
		}
	}
	return nil
}
