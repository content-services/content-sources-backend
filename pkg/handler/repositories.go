package handler

import (
	"fmt"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/event/adapter"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/content-services/content-sources-backend/pkg/event/producer"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/rs/zerolog/log"
)

const BulkCreateLimit = 20

type RepositoryHandler struct {
	RepositoryConfigDao       dao.RepositoryConfigDao
	IntrospectRequestProducer producer.IntrospectRequest
}

func RegisterRepositoryRoutes(engine *echo.Group, rDao *dao.RepositoryConfigDao, prod *producer.IntrospectRequest) {
	if engine == nil {
		panic("engine is nil")
	}
	if rDao == nil {
		panic("rDao is nil")
	}
	if prod == nil {
		panic("prod is nil")
	}
	rh := RepositoryHandler{
		RepositoryConfigDao:       *rDao,
		IntrospectRequestProducer: *prod,
	}
	engine.GET("/repositories/", rh.listRepositories)
	engine.GET("/repositories/:uuid", rh.fetch)
	engine.PUT("/repositories/:uuid", rh.fullUpdate)
	engine.PATCH("/repositories/:uuid", rh.partialUpdate)
	engine.DELETE("/repositories/:uuid", rh.deleteRepository)
	engine.POST("/repositories/", rh.createRepository)
	engine.POST("/repositories/bulk_create/", rh.bulkCreateRepositories)
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
	repos, totalRepos, err := rh.RepositoryConfigDao.List(orgID, pageData, filterData)
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
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/ [post]
func (rh *RepositoryHandler) createRepository(c echo.Context) error {
	var (
		newRepository api.RepositoryRequest
		msg           *message.IntrospectRequestMessage
		err           error
	)
	if err = c.Bind(&newRepository); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding params", err.Error())
	}

	accountID, orgID := getAccountIdOrgId(c)
	newRepository.AccountID = &accountID
	newRepository.OrgID = &orgID
	newRepository.FillDefaults()

	var response api.RepositoryResponse
	if response, err = rh.RepositoryConfigDao.Create(newRepository); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error creating repository", err.Error())
	}

	if msg, err = adapter.NewIntrospect().FromRepositoryResponse(&response); err != nil {
		log.Error().Msgf("error mapping to event message: %s", err.Error())
	}
	if err = rh.IntrospectRequestProducer.Produce(c, msg); err != nil {
		log.Warn().Msgf("error producing event message: %s", err.Error())
	}

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

	responses, errs := rh.RepositoryConfigDao.BulkCreate(newRepositories)
	if len(errs) > 0 {
		return ce.NewErrorResponseFromError("Error creating repository", errs...)
	}

	// Produce an event for each repository
	var msg *message.IntrospectRequestMessage
	var err error
	for _, repo := range responses {
		if msg, err = adapter.NewIntrospect().FromRepositoryResponse(&repo); err != nil {
			log.Error().Msgf("bulkCreateRepositories could not map to IntrospectRequest message: %s", err.Error())
			continue
		}
		if err = rh.IntrospectRequestProducer.Produce(c, msg); err != nil {
			log.Error().Msgf("bulkCreateRepositories returned an error: %s", err.Error())
			continue
		}
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

	response, err := rh.RepositoryConfigDao.Fetch(orgID, uuid)
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
	if fillDefaults {
		repoParams.FillDefaults()
	}
	if err := rh.RepositoryConfigDao.Update(orgID, uuid, repoParams); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error updating repository", err.Error())
	}
	if repoParams.URL != nil {
		// Produce IntrospectRequest event to introspect the updated url
		message, err := adapter.NewIntrospect().FromRepositoryRequest(&repoParams, uuid)
		if err != nil {
			log.Error().Msgf("Error adapting FromRepositoryRequest to message.IntrospectRequest: %s", err.Error())
		} else if err := rh.IntrospectRequestProducer.Produce(c, message); err != nil {
			// It prints out to the log, but does not change the response to
			// an error as the record was updated into the database
			log.Error().Msgf("Error producing event when Repository is updated: %s", err.Error())
		}
	}
	return rh.fetch(c)
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
	if err := rh.RepositoryConfigDao.Delete(orgID, uuid); err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error deleting repository", err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}
