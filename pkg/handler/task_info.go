package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
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
// @Description  Get the list of tasks.
// @Tags         tasks
// @Param		 offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param		 limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Param		 status query string false "A comma separated list of statuses to control response. Statuses can include `running`, `completed`, `failed`."
// @Param 		 type query string false "Filter results based on a specific task types. Helps to narrow down the results to a specific type. Task types can be `snapshot` or `introspect`. "
// @Param 		 repository_uuid query string false "A unique identifier of a repository to filter the results."
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

	tasks, totalTasks, err := t.DaoRegistry.TaskInfo.List(c.Request().Context(), orgID, pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing tasks", err.Error())
	}

	return c.JSON(http.StatusOK, setCollectionResponseMetadata(&tasks, c, totalTasks))
}

// Get TaskResponse godoc
// @Summary      Get Task
// @ID           getTask
// @Description  Get information about a specific task.
// @Tags         tasks
// @Accept       json
// @Produce      json
// @Param  uuid  path  string    true  "Task ID."
// @Success      200   {object}  api.TaskInfoResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /tasks/{uuid} [get]
func (t *TaskInfoHandler) fetch(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	id := c.Param("uuid")

	response, err := t.DaoRegistry.TaskInfo.Fetch(c.Request().Context(), orgID, id)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching task", err.Error())
	}
	return c.JSON(http.StatusOK, response)
}

func (t *TaskInfoHandler) cancel(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	id := c.Param("uuid")

	task, err := t.DaoRegistry.TaskInfo.Fetch(c.Request().Context(), orgID, id)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "error canceling task", err.Error())
	}
	if task.OrgId == config.RedHatOrg {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot cancel a Red Hat Task", err.Error())
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
