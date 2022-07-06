package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/identity"
)

type RepositoryHandler struct {
	RepositoryDao dao.RepositoryDao
}

func RegisterRepositoryRoutes(engine *echo.Group, rDao *dao.RepositoryDao) {
	rh := RepositoryHandler{RepositoryDao: *rDao}
	engine.GET("/repositories/", rh.listRepositories)
	engine.GET("/repositories/:uuid", rh.fetch)
	engine.PUT("/repositories/:uuid", rh.fullUpdate)
	engine.PATCH("/repositories/:uuid", rh.partialUpdate)
	engine.DELETE("/repositories/:uuid", rh.deleteRepository)
	engine.POST("/repositories/", rh.createRepository)
}

func getAccountIdOrgId(c echo.Context) (string, string, error) {
	decodedIdentity, err := base64.StdEncoding.DecodeString(c.Request().Header.Get(api.IdentityHeader))
	if err != nil {
		return "", "", err
	}

	var identityHeader identity.XRHID
	if err := json.Unmarshal(decodedIdentity, &identityHeader); err != nil {
		return "", "", err
	}
	return identityHeader.Identity.AccountNumber, identityHeader.Identity.Internal.OrgID, nil
}

// ListRepositories godoc
// @Summary      List Repositories
// @ID           listRepositories
// @Description  get repositories
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Success      200 {object} api.RepositoryCollectionResponse
// @Router       /repositories/ [get]
func (rh *RepositoryHandler) listRepositories(c echo.Context) error {
	_, orgID, err := getAccountIdOrgId(c)
	if err != nil {
		return badIdentity(err)
	}
	pageData := ParsePagination(c)
	filterData := ParseFilters(c)
	repos, totalRepos, _ :=
		rh.RepositoryDao.List(orgID, pageData, filterData)

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
	var newRepository api.RepositoryRequest
	if err := c.Bind(&newRepository); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Error binding params: "+err.Error())
	}

	accountID, orgID, err := getAccountIdOrgId(c)
	if err != nil {
		return badIdentity(err)
	}
	newRepository.AccountID = &accountID
	newRepository.OrgID = &orgID
	newRepository.FillDefaults()

	var response api.RepositoryResponse
	if response, err = rh.RepositoryDao.Create(newRepository); err != nil {
		return echo.NewHTTPError(httpCodeForError(err), "Error creating repository: "+err.Error())
	}

	c.Response().Header().Set("Location", "/api/content_sources/v1.0/repositories/"+response.UUID)
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
	_, orgID, err := getAccountIdOrgId(c)
	if err != nil {
		return badIdentity(err)
	}
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
// @Success      200
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
// @Success      200
// @Router       /repositories/{uuid} [patch]
func (rh *RepositoryHandler) partialUpdate(c echo.Context) error {
	return rh.update(c, false)
}

func (rh *RepositoryHandler) update(c echo.Context, fillDefaults bool) error {
	uuid := c.Param("uuid")
	repoParams := api.RepositoryRequest{}
	_, orgID, err := getAccountIdOrgId(c)
	if err != nil {
		return badIdentity(err)
	}

	if err := c.Bind(&repoParams); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "error binding params: "+err.Error())
	}
	if fillDefaults {
		repoParams.FillDefaults()
	}
	err = rh.RepositoryDao.Update(orgID, uuid, repoParams)
	if err != nil {
		return echo.NewHTTPError(httpCodeForError(err), err.Error())
	}
	return c.String(http.StatusOK, "Repository Updated.\n")
}

// DeleteRepository godoc
// @summary 		Delete a repository
// @ID				deleteRepository
// @Tags			repositories
// @Param  			uuid       path    string  true  "Identifier of the Repository"
// @Success			204
// @Router			/repositories/{uuid} [delete]
func (rh *RepositoryHandler) deleteRepository(c echo.Context) error {
	_, orgID, err := getAccountIdOrgId(c)
	if err != nil {
		return badIdentity(err)
	}
	uuid := c.Param("uuid")
	err = rh.RepositoryDao.Delete(orgID, uuid)
	if err != nil {
		return echo.NewHTTPError(httpCodeForError(err), err.Error())
	}
	return c.JSON(http.StatusNoContent, "")
}
