package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type TaskInfoHandler struct {
	DaoRegistry dao.DaoRegistry
}

func RegisterTaskInfoRoutes(engine *echo.Group, daoReg *dao.DaoRegistry) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("taskInfoReg is nil")
	}

	taskInfoHandler := TaskInfoHandler{
		DaoRegistry: *daoReg,
	}
	engine.GET("/tasks/", taskInfoHandler.listTasks)
	engine.GET("/tasks/:uuid", taskInfoHandler.fetch)
}

// ListTasks godoc
// @Summary      List Tasks
// @ID           listTasks
// @Description  list tasks
// @Tags         tasks
// @Param		 offset query int false "Offset into the list of results to return in the response"
// @Param		 limit query int false "Limit the number of items returned"
// @Param		 status query string false "Filter tasks by status using an exact match"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.TaskInfoCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /tasks/ [get]
func (taskInfoHandler *TaskInfoHandler) listTasks(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	pageData := ParsePagination(c)
	var statusFilter string
	err := echo.QueryParamsBinder(c).String("status", &statusFilter).BindError()
	if err != nil {
		log.Error().Err(err).Msg("Error parsing filters")
	}

	tasks, totalTasks, err := taskInfoHandler.DaoRegistry.TaskInfo.List(orgID, pageData, statusFilter)
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
func (taskInfoHandler *TaskInfoHandler) fetch(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	id := c.Param("uuid")

	response, err := taskInfoHandler.DaoRegistry.TaskInfo.Fetch(orgID, id)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching task", err.Error())
	}
	return c.JSON(http.StatusOK, response)
}
