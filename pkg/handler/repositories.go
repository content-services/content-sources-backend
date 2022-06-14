package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/identity"
)

type RepositoryHandler struct {
	RepositoryDao dao.RepositoryDao
}

func RegisterRepositoryRoutes(engine *echo.Group, rDao *dao.RepositoryDao) {
	rh := RepositoryHandler{RepositoryDao: *rDao}
	engine.GET("/repositories/", listRepositories)
	engine.GET("/repositories/:uuid", rh.fetch)
	engine.PUT("/repositories/:uuid", rh.fullUpdate)
	engine.PATCH("/repositories/:uuid", rh.partialUpdate)
	engine.DELETE("/repositories/:uuid", deleteRepository)
	engine.POST("/repositories/", rh.createRepository)
}

func getAccountIdOrgId(c echo.Context) (string, string, error) {
	decodedIdentity, err := base64.StdEncoding.DecodeString(c.Request().Header.Get("x-rh-identity"))
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
func listRepositories(c echo.Context) error {
	var total int64
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	page := ParsePagination(c)

	db.DB.Find(&repoConfigs).Count(&total)
	db.DB.Limit(page.Limit).Offset(page.Offset).Find(&repoConfigs)

	repos := convertToItems(repoConfigs)
	return c.JSON(200, collectionResponse(&api.RepositoryCollectionResponse{Data: repos}, c, total))
}

// CreateRepository godoc
// @Summary      Create Repository
// @ID           createRepository
// @Description  create a repository
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param  body       body    api.RepositoryRequest true  "request body"
// @Success      201
// @Router       /repositories/ [post]
func (rh *RepositoryHandler) createRepository(c echo.Context) error {
	newRepository := api.RepositoryRequest{}
	if err := c.Bind(&newRepository); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Error binding params: "+err.Error())
	}

	AccountID, OrgID, err := getAccountIdOrgId(c)
	if err != nil {
		return badIdentity(err)
	}
	newRepository.AccountID = &AccountID
	newRepository.OrgID = &OrgID

	newRepository.FillDefaults()

	if err := rh.RepositoryDao.Create(newRepository); err != nil {
		return echo.NewHTTPError(httpCodeForError(err), "Error creating repository: "+err.Error())
	}

	return c.String(http.StatusCreated, "Repository created.\n")
}

// Get RepositoryResponse godoc
// @Summary      Get Repository
// @ID           getRepository
// @Description  Get information about a Repository
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Param  uuid       path    string  true  "Identifier of the Repository"
// @Success      200
// @Router       /repositories/{uuid} [get]
func (rh *RepositoryHandler) fetch(c echo.Context) error {
	_, OrgID, err := getAccountIdOrgId(c)
	if err != nil {
		return badIdentity(err)
	}
	uuid := c.Param("uuid")

	response := rh.RepositoryDao.Fetch(OrgID, uuid)
	if response.UUID == "" {
		return echo.NewHTTPError(http.StatusNotFound, "Could not find RepositoryResponse with id "+uuid)
	} else {
		return c.JSON(200, response)
	}
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
	_, OrgID, err := getAccountIdOrgId(c)
	if err != nil {
		return badIdentity(err)
	}

	if err := c.Bind(&repoParams); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "error binding params: "+err.Error())
	}
	if fillDefaults {
		repoParams.FillDefaults()
	}
	err = rh.RepositoryDao.Update(OrgID, uuid, repoParams)
	if err != nil {
		return echo.NewHTTPError(httpCodeForError(err), err.Error())
	}
	return c.String(http.StatusOK, "RepositoryResponse Updated.\n")
}

// DeleteRepository godoc
// @summary 		Delete a repository
// @ID				deleteRepository
// @Tags			repositories
// @Param  			uuid       path    string  true  "Identifier of the Repository"
// @Success			200
// @Router			/repositories/:uuid [delete]
func deleteRepository(c echo.Context) error {
	repo := models.RepositoryConfiguration{}
	id := c.Param("uuid")
	db.DB.Find(&repo, "uuid = ?", id)
	if repo.UUID == "" {
		return echo.NewHTTPError(http.StatusNotFound, "Could not find RepositoryResponse with id "+id)
	} else {
		db.DB.Delete(&repo)
		return c.JSON(http.StatusNoContent, "")
	}
}

//Converts the database model to our response object
func convertToItems(repoConfigs []models.RepositoryConfiguration) []api.RepositoryResponse {
	repos := make([]api.RepositoryResponse, len(repoConfigs))
	for i := 0; i < len(repoConfigs); i++ {
		repos[i].FromRepositoryConfiguration(repoConfigs[i])
	}
	return repos
}
