package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
)

const (
	BulkCreateLimit = 20
	BulkDeleteLimit = 100
)

type RepositoryHandler struct {
	DaoRegistry          dao.DaoRegistry
	TaskClient           client.TaskClient
	FeatureServiceClient feature_service_client.FeatureServiceClient
}

func RegisterRepositoryRoutes(engine *echo.Group, daoReg *dao.DaoRegistry,
	taskClient *client.TaskClient, fsClient *feature_service_client.FeatureServiceClient,
) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("daoReg is nil")
	}
	if taskClient == nil {
		panic("taskClient is nil")
	}
	if fsClient == nil {
		panic("fsClient is nil")
	}
	rh := RepositoryHandler{
		DaoRegistry:          *daoReg,
		TaskClient:           *taskClient,
		FeatureServiceClient: *fsClient,
	}

	addRepoRoute(engine, http.MethodGet, "/repositories/", rh.listRepositories, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid", rh.fetch, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodPut, "/repositories/:uuid", rh.fullUpdate, rbac.RbacVerbWrite)
	addRepoRoute(engine, http.MethodPatch, "/repositories/:uuid", rh.partialUpdate, rbac.RbacVerbWrite)
	addRepoRoute(engine, http.MethodDelete, "/repositories/:uuid", rh.deleteRepository, rbac.RbacVerbWrite)
	addRepoRoute(engine, http.MethodPost, "/repositories/:uuid/add_uploads/", rh.addUploads, rbac.RbacVerbUpload)
	addRepoRoute(engine, http.MethodPost, "/repositories/uploads/", rh.createUpload, rbac.RbacVerbUpload)
	addRepoRoute(engine, http.MethodPost, "/repositories/uploads/:upload_uuid/upload_chunk/", rh.uploadChunk, rbac.RbacVerbUpload)
	addRepoRoute(engine, http.MethodPost, "/repositories/bulk_delete/", rh.bulkDeleteRepositories, rbac.RbacVerbWrite)
	addRepoRoute(engine, http.MethodPost, "/repositories/", rh.createRepository, rbac.RbacVerbWrite)
	addRepoRoute(engine, http.MethodPost, "/repositories/bulk_create/", rh.bulkCreateRepositories, rbac.RbacVerbWrite)
	addRepoRoute(engine, http.MethodPost, "/repositories/:uuid/snapshot/", rh.createSnapshot, rbac.RbacVerbWrite)
	addRepoRoute(engine, http.MethodPost, "/repositories/:uuid/introspect/", rh.introspect, rbac.RbacVerbWrite)
	addRepoRoute(engine, http.MethodGet, "/repository_gpg_key/:uuid", rh.getGpgKeyFile, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodPost, "/repositories/bulk_export/", rh.bulkExportRepositories, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodPost, "/repositories/bulk_import/", rh.bulkImportRepositories, rbac.RbacVerbWrite)
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
// @Param        status query string false "A comma separated list of statuses to control api response. Statuses can include `Pending`, `Valid`, `Invalid`, `Unavailable`."
// @Param		 origin query string false "A comma separated list of origins to filter api response. Origins can include `red_hat` and `external`."
// @Param		 content_type query string false "content type of a repository to filter on (rpm)"
// @Param		 extended_release query string false "A comma separated list of extended release types to filter on (eus, e4s), or 'none' to filter out extended release repositories"
// @Param		 extended_release_version query string false "A comma separated list of extended release versions to filter on (e.g. 9.4,9.6)"
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
	newRepository.FillDefaults(&accountID, &orgID)

	if err = rh.CheckSnapshotForRepo(c, newRepository.Snapshot); err != nil {
		return err
	}

	var response api.RepositoryResponse
	if response, err = rh.DaoRegistry.RepositoryConfig.Create(c.Request().Context(), newRepository); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error creating repository", err.Error())
	}

	if response.Snapshot {
		rh.enqueueSnapshotEvent(c, &response)
	}
	if response.Introspectable() {
		rh.enqueueIntrospectEvent(c, response, orgID)
	}

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
		newRepositories[i].FillDefaults(&accountID, &orgID)
	}

	if err := rh.CheckSnapshotForRepos(c, newRepositories); err != nil {
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
		if repo.Introspectable() {
			rh.enqueueIntrospectEvent(c, repo, orgID)
		}
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

	features, err := rh.FeatureServiceClient.GetEntitledFeatures(c.Request().Context(), orgID)
	if err != nil {
		log.Error().Err(err).Msg("error getting entitled features, proceeding with default")
	}

	response, err := rh.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if response.OrgID == config.RedHatOrg && !utils.Contains(features, response.FeatureName) {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", "Account does not have access to this repository")
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
// @Param        body       body    api.RepositoryUpdateRequest true  "request body"
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
	repoParams := api.RepositoryUpdateRequest{}
	_, orgID := getAccountIdOrgId(c)

	if err := c.Bind(&repoParams); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	if err := rh.CheckSnapshotForRepo(c, repoParams.Snapshot); err != nil {
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
		err := rh.cancelIntrospectAndSnapshot(c, orgID, repoConfig)
		if err != nil {
			return err
		}
	}

	response, err := rh.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	snapshottingNowEnabled := !repoConfig.Snapshot && response.Snapshot

	if (urlUpdated && response.Snapshot) || snapshottingNowEnabled {
		rh.enqueueSnapshotEvent(c, &response)
	}
	if urlUpdated || snapshottingNowEnabled {
		rh.enqueueIntrospectEvent(c, response, orgID)
	}

	rh.enqueueUpdateEvent(c, response, orgID)

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

	err = rh.cancelIntrospectAndSnapshot(c, orgID, repoConfig)
	if err != nil {
		return err
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

		err = rh.cancelIntrospectAndSnapshot(c, orgID, repoConfig)
		if err != nil {
			hasErr = true
			errs[i] = err
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
// @Success			200 {object} api.TaskInfoResponse
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

	if response.Origin == config.OriginUpload {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot snapshot this repository", "Upload repositories cannot be snapshotted.  To create a new snapshot, upload more content")
	}

	taskIDs, err := rh.DaoRegistry.TaskInfo.FetchActiveTasks(c.Request().Context(), orgID, response.RepositoryUUID, config.RepositorySnapshotTask)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error checking snapshot task", err.Error())
	}

	if len(taskIDs) > 0 {
		return ce.NewErrorResponse(http.StatusConflict, "Error snapshotting repository", "This repository is currently being snapshotted.")
	}

	if !response.Snapshot {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error snapshotting repository", "Snapshotting not yet enabled for this repository.")
	}

	taskID := rh.enqueueSnapshotEvent(c, &response)

	var resp api.TaskInfoResponse
	if resp, err = rh.DaoRegistry.TaskInfo.Fetch(c.Request().Context(), orgID, taskID); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching task info", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// IntrospectRepository godoc
// @summary 		introspect a repository
// @ID				introspect
// @Description     Check for repository updates.
// @Tags			repositories
// @Param  			uuid            path    string                          true   "Repository ID."
// @Param			body            body    api.RepositoryIntrospectRequest false  "request body"
// @Success			200 {object} api.TaskInfoResponse
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

	if response.Origin == config.OriginUpload {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot introspect this repository", "upload repositories cannot be introspected")
	}

	repo, err := rh.DaoRegistry.Repository.FetchForUrl(c.Request().Context(), response.URL, &response.Origin)
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
	lastIntrospectionStatus := config.StatusPending
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

	taskID := rh.enqueueIntrospectEvent(c, response, orgID)

	var resp api.TaskInfoResponse
	if resp, err = rh.DaoRegistry.TaskInfo.Fetch(c.Request().Context(), orgID, taskID); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "error fetching task info", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// CreateUploads godoc
// @summary         Create an upload
// @ID              createUpload
// @Description     Create an upload.
// @Tags            repositories
// @Accept          json
// @Produce         json
// @Param           body            body    api.CreateUploadRequest			true  "request body"
// @Success         200 {object} api.UploadResponse
// @Failure         400 {object} ce.ErrorResponse
// @Failure         404 {object} ce.ErrorResponse
// @Failure         500 {object} ce.ErrorResponse
// @Router          /repositories/uploads/ [post]
func (rh *RepositoryHandler) createUpload(c echo.Context) error {
	_, orgId := getAccountIdOrgId(c)
	ph := &PulpHandler{
		DaoRegistry: rh.DaoRegistry,
	}

	var req api.CreateUploadRequest

	if err := c.Bind(&req); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	if req.ChunkSize <= 0 {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error creating upload", "Chunk size must be greater than 0")
	}

	if req.Size <= 0 {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error creating upload", "Size must be greater than 0")
	}

	if req.Resumable {
		// resumable=true returns the existing UUID, if there has been a previous upload matching the same Sha256 and ChunkSize

		domainName, err := ph.DaoRegistry.Domain.FetchOrCreateDomain(c.Request().Context(), orgId)
		if err != nil {
			return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "error fetching or creating domain", err.Error())
		}

		pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

		artifactHref, err := pulpClient.LookupArtifact(c.Request().Context(), req.Sha256)

		if err != nil && len(strings.Split(err.Error(), "Not Found")) == 1 {
			return err
		}

		existingUUID, completedChunks, err := ph.DaoRegistry.Uploads.GetExistingUploadIDAndCompletedChunks(c.Request().Context(), orgId, req.Sha256, req.ChunkSize, req.Size)
		if err != nil {
			return err
		}

		// pulp artifact has already been created and uploaded content is being reused
		if artifactHref != nil && existingUUID != "" {
			resp := &api.UploadResponse{
				UploadUuid:         &existingUUID,
				ArtifactHref:       artifactHref,
				Size:               req.Size,
				CompletedChecksums: completedChunks,
			}
			return c.JSON(http.StatusCreated, resp)
		}

		// pulp artifact has not been created yet, but uploaded content has been saved in our db and can be reused
		if existingUUID != "" {
			resp := &api.UploadResponse{
				UploadUuid:         &existingUUID,
				Size:               req.Size,
				CompletedChecksums: completedChunks,
			}
			return c.JSON(http.StatusCreated, resp)
		}
	}
	// resumable=nil or resumable=false or first time upload, returns new UUID

	pulpResp, err := ph.createUploadInternal(c, req)
	if err != nil {
		return err
	}

	uploadUuid := ""
	if pulpResp != nil && pulpResp.PulpHref != nil {
		uploadUuid = extractUploadUuid(*pulpResp.PulpHref)
	}

	// new content to upload
	resp := &api.UploadResponse{
		UploadUuid:  &uploadUuid,
		Created:     pulpResp.PulpCreated,
		LastUpdated: pulpResp.PulpLastUpdated,
		Size:        pulpResp.Size,
		Completed:   pulpResp.Completed,
	}

	return c.JSON(http.StatusCreated, resp)
}

// UploadChunk godoc
// @summary         Upload a file chunk
// @ID              uploadChunk
// @Description     Upload a file chunk.
// @Tags            repositories
// @Accept          multipart/form-data
// @Param           upload_uuid            path     string true  "Upload ID."
// @Param           file                   formData file   true  "file chunk"
// @Param           sha256                 formData string true  "sha256"
// @Param           Content-Range          header   string true  "Content-Range header"
// @Success         200 {object} api.UploadResponse
// @Failure         400 {object} ce.ErrorResponse
// @Failure         404 {object} ce.ErrorResponse
// @Failure         500 {object} ce.ErrorResponse
// @Router          /repositories/uploads/{upload_uuid}/upload_chunk/ [post]
func (rh *RepositoryHandler) uploadChunk(c echo.Context) error {
	var req api.UploadChunkRequest

	_, orgId := getAccountIdOrgId(c)
	uploadUuid := c.Param("upload_uuid")

	if err := c.Bind(&req); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	domainName, err := rh.DaoRegistry.Domain.Fetch(c.Request().Context(), orgId)
	if err != nil {
		return err
	}

	c.SetParamNames("upload_href")
	c.SetParamValues(fmt.Sprintf("/api/pulp/%s/api/v3/uploads/%s/", domainName, uploadUuid))

	ph := &PulpHandler{
		DaoRegistry: rh.DaoRegistry,
	}

	pulpResp, err := ph.uploadChunkInternal(c)
	if err != nil {
		return err
	}
	if pulpResp != nil && pulpResp.PulpHref != nil {
		uploadUuid = extractUploadUuid(*pulpResp.PulpHref)
	}

	resp := &api.UploadResponse{
		UploadUuid:  &uploadUuid,
		Created:     pulpResp.PulpCreated,
		LastUpdated: pulpResp.PulpLastUpdated,
		Size:        pulpResp.Size,
		Completed:   pulpResp.Completed,
	}

	err = ph.DaoRegistry.Uploads.StoreChunkUpload(c.Request().Context(), orgId, uploadUuid, req.Sha256)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, resp)
}

// AddUploadsToRepository godoc
// @summary 		Add uploads to a repository
// @ID				add_upload
// @Description     Add uploads to a repository.
// @Tags			repositories
// @Accept          json
// @Param  			uuid            path    string                          true   "Repository ID."
// @Param			body            body    api.AddUploadsRequest			true  "request body"
// @Success			200 {object} api.TaskInfoResponse
// @Failure      	400 {object} ce.ErrorResponse
// @Failure      	404 {object} ce.ErrorResponse
// @Failure      	500 {object} ce.ErrorResponse
// @Router			/repositories/{uuid}/add_uploads/ [post]
func (rh *RepositoryHandler) addUploads(c echo.Context) error {
	var req api.AddUploadsRequest

	_, orgID := getAccountIdOrgId(c)
	uuid := c.Param("uuid")

	if err := c.Bind(&req); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	response, err := rh.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository uuid", err.Error())
	}

	if response.Origin != config.OriginUpload {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot add uploads to this repository", "Can only add them to repositories of type 'upload'")
	}
	taskID := rh.enqueueAddUploadsEvent(c, response, orgID, req)
	var resp api.TaskInfoResponse
	if resp, err = rh.DaoRegistry.TaskInfo.Fetch(c.Request().Context(), orgID, taskID); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "error fetching task info", err.Error())
	}

	return c.JSON(http.StatusCreated, resp)
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

	resp, err := rh.DaoRegistry.RepositoryConfig.FetchWithoutOrgID(c.Request().Context(), uuid, false)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}
	if resp.GpgKey == "" {
		errMsg := fmt.Errorf("no GPG key found for this repository")
		return ce.NewErrorResponse(http.StatusNotFound, "Error fetching gpg key", errMsg.Error())
	}
	return c.String(http.StatusOK, resp.GpgKey)
}

// ExportRepository godoc
// @Summary      Bulk export repositories
// @ID           bulkExportRepositories
// @Description  Export multiple repositories.
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param        body  body     api.RepositoryExportRequest  true  "request body"
// @Success      201  {object}  []api.RepositoryExportResponse
// @Header       201  {string}  Location "resource URL"
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/bulk_export/ [post]
func (rh *RepositoryHandler) bulkExportRepositories(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	var reposToExport api.RepositoryExportRequest
	if err := c.Bind(&reposToExport); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	resp, err := rh.DaoRegistry.RepositoryConfig.BulkExport(c.Request().Context(), orgID, reposToExport)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error exporting repositories", err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// ImportRepository godoc
// @Summary      Bulk import repositories
// @ID           bulkImportRepositories
// @Description  Import multiple repositories.
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param        body  body     []api.RepositoryRequest  true  "request body"
// @Success      201  {object}  []api.RepositoryImportResponse
// @Header       201  {string}  Location "resource URL"
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/bulk_import/ [post]
func (rh *RepositoryHandler) bulkImportRepositories(c echo.Context) error {
	accountID, orgID := getAccountIdOrgId(c)
	var reposToImport []api.RepositoryRequest
	if err := c.Bind(&reposToImport); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	if BulkCreateLimit < len(reposToImport) {
		limitErrMsg := fmt.Sprintf("Cannot import more than %d repositories at once.", BulkCreateLimit)
		return ce.NewErrorResponse(http.StatusRequestEntityTooLarge, "Error importing repositories", limitErrMsg)
	}

	for i := 0; i < len(reposToImport); i++ {
		reposToImport[i].FillDefaults(&accountID, &orgID)
	}

	if err := rh.CheckSnapshotForRepos(c, reposToImport); err != nil {
		return err
	}

	responses, errs := rh.DaoRegistry.RepositoryConfig.BulkImport(c.Request().Context(), reposToImport)
	if len(errs) > 0 {
		return ce.NewErrorResponseFromError("Error importing repositories", errs...)
	}

	// Produce an event for each repository if there are no existing repos with the same URL
	for index, repo := range responses {
		if repo.Origin == config.OriginCommunity {
			continue
		}
		if repo.Origin == config.OriginUpload && repo.Warnings[0]["description"] == dao.UploadRepositoryWarning {
			rh.enqueueSnapshotEvent(c, &responses[index].RepositoryResponse)
		}
		if len(repo.Warnings) == 0 && repo.Snapshot {
			rh.enqueueSnapshotEvent(c, &responses[index].RepositoryResponse)
		}
		if len(repo.Warnings) == 0 && repo.Introspectable() {
			rh.enqueueIntrospectEvent(c, repo.RepositoryResponse, orgID)
		}
	}

	return c.JSON(http.StatusCreated, responses)
}

// enqueueSnapshotEvent queues up a snapshot for a given repository uuid (not repository config) and org.
func (rh *RepositoryHandler) enqueueSnapshotEvent(c echo.Context, response *api.RepositoryResponse) string {
	if config.PulpConfigured() {
		task := queue.Task{
			Typename:   config.RepositorySnapshotTask,
			Payload:    payloads.SnapshotPayload{},
			OrgId:      response.OrgID,
			ObjectUUID: &response.RepositoryUUID,
			ObjectType: utils.Ptr(config.ObjectTypeRepository),
			RequestID:  c.Response().Header().Get(config.HeaderRequestId),
			AccountId:  response.AccountID,
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
			rh.enqueueUpdateLatestSnapshotEvent(c, response.OrgID, taskID, *response)
		}

		return taskID.String()
	}
	return ""
}

func (rh *RepositoryHandler) enqueueSnapshotDeleteEvent(c echo.Context, orgID string, repo api.RepositoryResponse) {
	payload := tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repo.UUID}
	task := queue.Task{
		Typename:   config.DeleteRepositorySnapshotsTask,
		Payload:    payload,
		OrgId:      orgID,
		AccountId:  repo.AccountID,
		ObjectUUID: &repo.RepositoryUUID,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		RequestID:  c.Response().Header().Get(config.HeaderRequestId),
	}
	taskID, err := rh.TaskClient.Enqueue(task)
	if err != nil {
		logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
		logger.Error().Msg("error enqueuing task")
	}
}

func (rh *RepositoryHandler) enqueueAddUploadsEvent(c echo.Context, response api.RepositoryResponse, orgID string, req api.AddUploadsRequest) string {
	var err error
	task := queue.Task{
		Typename: config.AddUploadsTask,
		Payload: tasks.AddUploadsPayload{
			RepositoryConfigUUID: response.UUID,
			Artifacts:            req.Artifacts,
			Uploads:              req.Uploads,
		},
		OrgId:      orgID,
		AccountId:  response.AccountID,
		ObjectUUID: &response.RepositoryUUID,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		RequestID:  c.Response().Header().Get(config.HeaderRequestId),
	}
	taskID, err := rh.TaskClient.Enqueue(task)
	logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
	if err != nil {
		logger.Error().Msg("error enqueuing add uploads task")
	} else {
		if err := rh.DaoRegistry.RepositoryConfig.UpdateLastSnapshotTask(c.Request().Context(), taskID.String(), response.OrgID, response.RepositoryUUID); err != nil {
			logger.Error().Err(err).Msgf("error UpdatingLastSnapshotTask task for AddUploads")
		}
		rh.enqueueUpdateLatestSnapshotEvent(c, response.OrgID, taskID, response)
	}

	return taskID.String()
}

func (rh *RepositoryHandler) enqueueIntrospectEvent(c echo.Context, response api.RepositoryResponse, orgID string) string {
	var err error
	task := queue.Task{
		Typename:   payloads.Introspect,
		Payload:    payloads.IntrospectPayload{Url: response.URL, Force: true, Origin: &response.Origin},
		OrgId:      orgID,
		AccountId:  response.AccountID,
		ObjectUUID: &response.RepositoryUUID,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		RequestID:  c.Response().Header().Get(config.HeaderRequestId),
	}
	taskID, err := rh.TaskClient.Enqueue(task)
	if err != nil {
		logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
		logger.Error().Msg("error enqueuing task")
	}

	return taskID.String()
}

func (rh *RepositoryHandler) cancelIntrospectAndSnapshot(c echo.Context, orgID string, repoConfig api.RepositoryResponse) error {
	taskIDs, err := rh.DaoRegistry.TaskInfo.FetchActiveTasks(c.Request().Context(), orgID, repoConfig.RepositoryUUID, config.RepositorySnapshotTask, config.IntrospectTask)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error checking if introspect is in progress", err.Error())
	}

	for _, taskID := range taskIDs {
		err = rh.TaskClient.Cancel(c.Request().Context(), taskID)
		if err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error canceling introspect", err.Error())
		}
	}
	return nil
}

func (rh *RepositoryHandler) enqueueUpdateEvent(c echo.Context, response api.RepositoryResponse, orgID string) {
	var err error
	task := queue.Task{
		Typename:   config.UpdateRepositoryTask,
		Payload:    tasks.UpdateRepositoryPayload{RepositoryConfigUUID: response.UUID},
		OrgId:      orgID,
		AccountId:  response.AccountID,
		ObjectUUID: &response.RepositoryUUID,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		RequestID:  c.Response().Header().Get(config.HeaderRequestId),
		Priority:   1,
	}
	taskID, err := rh.TaskClient.Enqueue(task)
	if err != nil {
		logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
		logger.Error().Msg("error enqueuing task")
	}
}

func (rh *RepositoryHandler) enqueueUpdateLatestSnapshotEvent(c echo.Context, orgID string, snapshotTaskID uuid.UUID, response api.RepositoryResponse) {
	if config.PulpConfigured() {
		var err error
		task := queue.Task{
			Typename:     config.UpdateLatestSnapshotTask,
			Payload:      tasks.UpdateLatestSnapshotPayload{RepositoryConfigUUID: response.UUID},
			OrgId:        orgID,
			AccountId:    response.AccountID,
			ObjectUUID:   &response.RepositoryUUID,
			ObjectType:   utils.Ptr(config.ObjectTypeRepository),
			RequestID:    c.Response().Header().Get(config.HeaderRequestId),
			Dependencies: []uuid.UUID{snapshotTaskID},
		}
		taskID, err := rh.TaskClient.Enqueue(task)
		if err != nil {
			logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
			logger.Error().Msg("error enqueuing task")
		}
	}
}

func (rh *RepositoryHandler) CheckSnapshotForRepo(c echo.Context, snapshotParam *bool) error {
	if snapshotParam != nil && *snapshotParam {
		if err := CheckSnapshotAccessible(c.Request().Context()); err != nil {
			return err
		}
	}
	return nil
}

// CheckSnapshotForRepos checks if for a given RepositoryRequest, snapshotting can be done
func (rh *RepositoryHandler) CheckSnapshotForRepos(c echo.Context, repos []api.RepositoryRequest) error {
	for _, repo := range repos {
		return rh.CheckSnapshotForRepo(c, repo.Snapshot)
	}
	return nil
}
