package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type TaskInfoHandler struct {
	DaoRegistry dao.DaoRegistry
	TaskClient  client.TaskClient
}

func RegisterTaskInfoRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, taskClient *client.TaskClient) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("taskInfoReg is nil")
	}
	if taskClient == nil {
		panic("taskClient is nil")
	}

	taskInfoHandler := TaskInfoHandler{
		DaoRegistry: *daoReg,
		TaskClient:  *taskClient,
	}
	addRoute(engine, http.MethodGet, "/tasks/", taskInfoHandler.listTasks, rbac.RbacVerbRead)
	addRoute(engine, http.MethodGet, "/tasks/:uuid", taskInfoHandler.fetch, rbac.RbacVerbRead)
	addRoute(engine, http.MethodPost, "/tasks/:uuid/cancel/", taskInfoHandler.cancel, rbac.RbacVerbWrite)
}

// ListTasks godoc
// @Summary      List Tasks
// @ID           listTasks
// @Description  list tasks
// @Tags         tasks
// @Param		 offset query int false "Offset into the list of results to return in the response"
// @Param		 limit query int false "Limit the number of items returned"
// @Param		 status query string false "Filter tasks by status using an exact match"
// @Param 		 type query string false "Filter tasks by type using an exact match"
// @Param 		 repository_uuid query string false "Filter tasks by associated repository UUID using an exact match"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.TaskInfoCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /tasks/ [get]
func (t *TaskInfoHandler) listTasks(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	pageData := ParsePagination(c)
	filterData := ParseTaskInfoFilters(c)

	tasks, totalTasks, err := t.DaoRegistry.TaskInfo.List(orgID, pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing tasks", err.Error())
	}

	return c.JSON(http.StatusOK, setCollectionResponseMetadata(&tasks, c, totalTasks))
}

// Get TaskResponse godoc
// @Summary      Get Task
// @ID           getTask
// @Description  Get information about a Task
// @Tags         tasks
// @Accept       json
// @Produce      json
// @Param  uuid  path  string    true  "Identifier of the Task"
// @Success      200   {object}  api.TaskInfoResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /tasks/{uuid} [get]
func (t *TaskInfoHandler) fetch(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	id := c.Param("uuid")

	response, err := t.DaoRegistry.TaskInfo.Fetch(orgID, id)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching task", err.Error())
	}
	return c.JSON(http.StatusOK, response)
}

func (t *TaskInfoHandler) cancel(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	id := c.Param("uuid")

	_, err := t.DaoRegistry.TaskInfo.Fetch(orgID, id)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "error canceling task", err.Error())
	}

	err = t.TaskClient.SendCancelNotification(c.Request().Context(), id)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "error canceling task", err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func ParseTaskInfoFilters(c echo.Context) api.TaskInfoFilterData {
	filterData := api.TaskInfoFilterData{
		Status:         "",
		Typename:       "",
		RepoConfigUUID: "",
	}

	err := echo.QueryParamsBinder(c).
		String("status", &filterData.Status).
		String("type", &filterData.Typename).
		String("repository_uuid", &filterData.RepoConfigUUID).
		BindError()

	if err != nil {
		log.Error().Err(err).Msg("Error parsing filters")
	}

	return filterData
}
