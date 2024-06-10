package handler

import (
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
)

type TemplateHandler struct {
	DaoRegistry dao.DaoRegistry
	TaskClient  client.TaskClient
}

func RegisterTemplateRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, taskClient *client.TaskClient) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("daoReg is nil")
	}

	if taskClient == nil {
		panic("taskClient is nil")
	}
	h := TemplateHandler{
		DaoRegistry: *daoReg,
		TaskClient:  *taskClient,
	}

	addTemplateRoute(engine, http.MethodGet, "/templates/", h.listTemplates, rbac.RbacVerbRead)
	addTemplateRoute(engine, http.MethodGet, "/templates/:uuid", h.fetch, rbac.RbacVerbRead)
	addTemplateRoute(engine, http.MethodPost, "/templates/", h.createTemplate, rbac.RbacVerbWrite)
	addTemplateRoute(engine, http.MethodDelete, "/templates/:uuid", h.deleteTemplate, rbac.RbacVerbWrite)
	addTemplateRoute(engine, http.MethodPut, "/templates/:uuid", h.fullUpdate, rbac.RbacVerbWrite)
	addTemplateRoute(engine, http.MethodPatch, "/templates/:uuid", h.partialUpdate, rbac.RbacVerbWrite)
}

// CreateRepository godoc
// @Summary      Create Template
// @ID           createTemplate
// @Description  This operation enables creating templates based on user preferences.
// @Tags         templates
// @Accept       json
// @Produce      json
// @Param        body  body     api.TemplateRequest  true  "request body"
// @Success      201  {object}  api.TemplateResponse
// @Header       201  {string}  Location "resource URL"
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /templates/ [post]
func (th *TemplateHandler) createTemplate(c echo.Context) error {
	var newTemplate api.TemplateRequest
	if err := c.Bind(&newTemplate); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding params", err.Error())
	}
	_, orgID := getAccountIdOrgId(c)
	newTemplate.OrgID = &orgID

	user := getUser(c)
	newTemplate.User = &user

	respTemplate, err := th.DaoRegistry.Template.Create(c.Request().Context(), newTemplate)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error creating template", err.Error())
	}

	if config.Get().Clients.Candlepin.Server != "" {
		th.enqueueUpdateTemplateContentEvent(c, respTemplate)
	}

	return c.JSON(http.StatusCreated, respTemplate)
}

// Get RepositoryResponse godoc
// @Summary      Get Template
// @ID           getTemplate
// @Description  Get template information.
// @Tags         templates
// @Accept       json
// @Produce      json
// @Param  uuid  path  string    true  "Template ID."
// @Success      200   {object}  api.TemplateResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /templates/{uuid} [get]
func (th *TemplateHandler) fetch(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	uuid := c.Param("uuid")

	resp, err := th.DaoRegistry.Template.Fetch(c.Request().Context(), orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching template", err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// ListTemplates godoc
// @Summary      List Templates
// @ID           listTemplates
// @Description  This operation enables users to retrieve a list of templates.
// @Tags         templates
// @Param		 offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param		 limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Param		 version query string false "Filter templates by version."
// @Param		 arch query string false "Filter templates by architecture."
// @Param		 name query string false "Filter templates by name."
// @Param		 repository_uuids query string false "Filter templates by associated repositories using a comma separated list of repository UUIDs"
// @Param		 sort_by query string false "Sort the response data based on specific parameters. Sort criteria can include `name`, `arch`, and `version`."
// @Accept       json
// @Produce      json
// @Success      200 {object} api.TemplateCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /templates/ [get]
func (th *TemplateHandler) listTemplates(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	pageData := ParsePagination(c)
	filterData := ParseTemplateFilters(c)

	templates, total, err := th.DaoRegistry.Template.List(c.Request().Context(), orgID, pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing templates", err.Error())
	}
	return c.JSON(http.StatusOK, setCollectionResponseMetadata(&templates, c, total))
}

// FullUpdateTemplate godoc
// @Summary      Fully update all attributes of a Template
// @ID           fullUpdateTemplate
// @Description  This operation enables updating all attributes of a template
// @Tags         templates
// @Accept       json
// @Produce      json
// @Param        uuid  path  string    true  "Template ID."
// @Param        body  body     api.TemplateUpdateRequest  true  "request body"
// @Success      201  {object}  api.TemplateResponse
// @Header       201  {string}  Location "resource URL"
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /templates/{uuid} [put]
func (th *TemplateHandler) fullUpdate(c echo.Context) error {
	return th.update(c, true)
}

// PartiallyUpdateTemplate godoc
// @Summary      Update some attributes of a Template
// @ID           partialUpdateTemplate
// @Description  This operation enables updating some subset of attributes of a template
// @Tags         templates
// @Accept       json
// @Produce      json
// @Param        uuid  path  string    true  "Template ID."
// @Param        body  body     api.TemplateUpdateRequest  true  "request body"
// @Success      201  {object}  api.TemplateResponse
// @Header       201  {string}  Location "resource URL"
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /templates/{uuid} [patch]
func (th *TemplateHandler) partialUpdate(c echo.Context) error {
	return th.update(c, false)
}

func (th *TemplateHandler) update(c echo.Context, fillDefaults bool) error {
	uuid := c.Param("uuid")
	tempParams := api.TemplateUpdateRequest{}
	_, orgID := getAccountIdOrgId(c)

	user := getUser(c)
	tempParams.User = &user

	if err := c.Bind(&tempParams); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	if fillDefaults {
		tempParams.FillDefaults()
	}
	respTemplate, err := th.DaoRegistry.Template.Update(c.Request().Context(), orgID, uuid, tempParams)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error updating template", err.Error())
	}

	if config.Get().Clients.Candlepin.Server != "" {
		th.enqueueUpdateTemplateContentEvent(c, respTemplate)
	}

	return c.JSON(http.StatusOK, respTemplate)
}

func ParseTemplateFilters(c echo.Context) api.TemplateFilterData {
	filterData := api.TemplateFilterData{
		Name:    "",
		Version: "",
		Arch:    "",
		Search:  "",
	}
	repositoryUUIDs := ""
	err := echo.QueryParamsBinder(c).
		String("name", &filterData.Name).
		String("version", &filterData.Version).
		String("arch", &filterData.Arch).
		String("search", &filterData.Search).
		String("repository_uuids", &repositoryUUIDs).
		BindError()

	if err != nil {
		log.Error().Err(err).Msg("Error parsing filters")
	}
	if repositoryUUIDs != "" {
		filterData.RepositoryUUIDs = strings.Split(repositoryUUIDs, ",")
	}

	return filterData
}

// DeleteTemplate godoc
// @summary 		Delete a template
// @ID				deleteTemplate
// @Description     This enables deleting a specific template.
// @Tags			templates
// @Param  			uuid       path    string  true  "Template ID."
// @Success			204 "Template was successfully deleted"
// @Failure      	400 {object} ce.ErrorResponse
// @Failure     	401 {object} ce.ErrorResponse
// @Failure      	404 {object} ce.ErrorResponse
// @Failure      	500 {object} ce.ErrorResponse
// @Router			/templates/{uuid} [delete]
func (th *TemplateHandler) deleteTemplate(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	uuid := c.Param("uuid")

	template, err := th.DaoRegistry.Template.Fetch(c.Request().Context(), orgID, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching template", err.Error())
	}
	if err := th.DaoRegistry.Template.SoftDelete(c.Request().Context(), orgID, uuid); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error deleting template", err.Error())
	}
	enqueueErr := th.enqueueTemplateDeleteEvent(c, orgID, template)
	if enqueueErr != nil {
		if err = th.DaoRegistry.Template.ClearDeletedAt(c.Request().Context(), orgID, uuid); err != nil {
			return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error clearing deleted_at field", err.Error())
		}
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(enqueueErr), "Error enqueueing task", enqueueErr.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

func (th *TemplateHandler) enqueueTemplateDeleteEvent(c echo.Context, orgID string, template api.TemplateResponse) error {
	accountID, _ := getAccountIdOrgId(c)
	payload := tasks.DeleteTemplatesPayload{TemplateUUID: template.UUID, RepoConfigUUIDs: template.RepositoryUUIDS}
	task := queue.Task{
		Typename:  config.DeleteTemplatesTask,
		Payload:   payload,
		OrgId:     orgID,
		AccountId: accountID,
		RequestID: c.Response().Header().Get(config.HeaderRequestId),
	}
	taskID, err := th.TaskClient.Enqueue(task)
	if err != nil {
		logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
		logger.Error().Msg("error enqueuing task")
		return err
	}

	return nil
}

func (th *TemplateHandler) enqueueUpdateTemplateContentEvent(c echo.Context, template api.TemplateResponse) uuid.UUID {
	accountID, orgID := getAccountIdOrgId(c)
	payload := payloads.UpdateTemplateContentPayload{TemplateUUID: template.UUID, RepoConfigUUIDs: template.RepositoryUUIDS}
	task := queue.Task{
		Typename:  config.UpdateTemplateContentTask,
		Payload:   payload,
		OrgId:     orgID,
		AccountId: accountID,
		RequestID: c.Response().Header().Get(config.HeaderRequestId),
		Priority:  1,
	}
	taskID, err := th.TaskClient.Enqueue(task)
	if err != nil {
		logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
		logger.Error().Msg("error enqueuing task")
	}
	return taskID
}

func getUser(c echo.Context) string {
	id := identity.Get(c.Request().Context())
	if id.Identity.User != nil {
		return id.Identity.User.Username
	}
	return ""
}
