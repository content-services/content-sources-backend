package handler

import (
	"fmt"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/identity"
)

const BulkCreateLimit = 20

type RepositoryHandler struct {
	RepositoryDao dao.RepositoryConfigDao
}

func RegisterRepositoryRoutes(engine *echo.Group, rDao *dao.RepositoryConfigDao) {
	rh := RepositoryHandler{RepositoryDao: *rDao}
	engine.GET("/repositories/", rh.listRepositories)
	engine.GET("/repositories/:uuid", rh.fetch)
	engine.PUT("/repositories/:uuid", rh.fullUpdate)
	engine.PATCH("/repositories/:uuid", rh.partialUpdate)
	engine.DELETE("/repositories/:uuid", rh.deleteRepository)
	engine.POST("/repositories/", rh.createRepository)
	engine.POST("/repositories/bulk_create/", rh.bulkCreateRepositories)
}

func getAccountIdOrgId(c echo.Context) (string, string) {
	// This block is a bit defensive as the read of the XRHID structure from the
	// context does not check if the value is a nil and
	if value := c.Request().Context().Value(identity.Key); value == nil {
		return "", ""
	}
	identityHeader := identity.Get(c.Request().Context())
	return identityHeader.Identity.AccountNumber, identityHeader.Identity.Internal.OrgID
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
// @Accept       json
// @Produce      json
// @Success      200 {object} api.RepositoryCollectionResponse
// @Router       /repositories/ [get]
func (rh *RepositoryHandler) listRepositories(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	c.Logger().Infof("org_id: %s", orgID)
	pageData := ParsePagination(c)
	filterData := ParseFilters(c)
	repos, totalRepos, err := rh.RepositoryDao.List(orgID, pageData, filterData)
	if err != nil {
		return echo.NewHTTPError(httpCodeForError(err), "Error listing repositories: "+err.Error())
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
// @Router       /repositories/ [post]
func (rh *RepositoryHandler) createRepository(c echo.Context) error {
	var (
		newRepository api.RepositoryRequest
		err           error
	)
	if err = c.Bind(&newRepository); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Error binding params: "+err.Error())
	}

	accountID, orgID := getAccountIdOrgId(c)
	newRepository.AccountID = &accountID
	newRepository.OrgID = &orgID
	newRepository.FillDefaults()

	var response api.RepositoryResponse
	if response, err = rh.RepositoryDao.Create(newRepository); err != nil {
		return echo.NewHTTPError(httpCodeForError(err), "Error creating repository: "+err.Error())
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
// @Success      201  {object}  []api.RepositoryBulkCreateResponse
// @Header       201  {string}  Location "resource URL"
// @Router       /repositories/bulk_create/ [post]
func (rh *RepositoryHandler) bulkCreateRepositories(c echo.Context) error {
	var newRepositories []api.RepositoryRequest
	if err := c.Bind(&newRepositories); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Error binding params: "+err.Error())
	}

	if BulkCreateLimit < len(newRepositories) {
		limitErrMsg := fmt.Sprintf("Cannot create more than %d repositories at once.", BulkCreateLimit)
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, limitErrMsg)
	}

	accountID, orgID := getAccountIdOrgId(c)

	for i := 0; i < len(newRepositories); i++ {
		newRepositories[i].AccountID = &accountID
		newRepositories[i].OrgID = &orgID
		newRepositories[i].FillDefaults()
	}

	response, err := rh.RepositoryDao.BulkCreate(newRepositories)
	if err != nil {
		return c.JSON(httpCodeForError(err), response)
	}

	return c.JSON(http.StatusCreated, response)
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
// @Router       /repositories/{uuid} [get]
func (rh *RepositoryHandler) fetch(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	uuid := c.Param("uuid")

	response, err := rh.RepositoryDao.Fetch(orgID, uuid)
	if err != nil {
		return echo.NewHTTPError(httpCodeForError(err), err.Error())
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
// @Success      200 {string}  string    "OK"
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
// @Success      200 {string}  string    "OK"
// @Router       /repositories/{uuid} [patch]
func (rh *RepositoryHandler) partialUpdate(c echo.Context) error {
	return rh.update(c, false)
}

func (rh *RepositoryHandler) update(c echo.Context, fillDefaults bool) error {
	uuid := c.Param("uuid")
	repoParams := api.RepositoryRequest{}
	_, orgID := getAccountIdOrgId(c)

	if err := c.Bind(&repoParams); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "error binding params: "+err.Error())
	}
	if fillDefaults {
		repoParams.FillDefaults()
	}
	if err := rh.RepositoryDao.Update(orgID, uuid, repoParams); err != nil {
		return echo.NewHTTPError(httpCodeForError(err), err.Error())
	}
	return c.String(http.StatusOK, "Repository Updated.\n")
}

// DeleteRepository godoc
// @summary 		Delete a repository
// @ID				deleteRepository
// @Tags			repositories
// @Param  			uuid       path    string  true  "Identifier of the Repository"
// @Success			204 "Repository was successfully deleted"
// @Router			/repositories/{uuid} [delete]
func (rh *RepositoryHandler) deleteRepository(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)
	uuid := c.Param("uuid")
	if err := rh.RepositoryDao.Delete(orgID, uuid); err != nil {
		return echo.NewHTTPError(httpCodeForError(err), err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}
