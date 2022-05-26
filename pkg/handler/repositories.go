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

func RegisterRepositoryRoutes(engine *echo.Group) {
	engine.GET("/repositories/", listRepositories)
	engine.DELETE("/repositories/:uuid", deleteRepository)
	engine.POST("/repositories/", createRepository)
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
// @Router       /repositories [get]
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
// @Param  body       body    api.CreateRepository true  "request body"
// @Param  org_id     header  string         	   true  "organization id"
// @Param  account_id header  string               true  "account number"
// @Success      201
// @Router       /repositories [post]
func createRepository(c echo.Context) error {
	newRepository := api.CreateRepository{}
	if err := c.Bind(&newRepository); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "error binding params: "+err.Error())
	}

	AccountID, OrgID, err := getAccountIdOrgId(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "error parsing identity: "+err.Error())
	}
	newRepository.AccountID = AccountID
	newRepository.OrgID = OrgID

	repositoryDao := dao.GetRepositoryDao()
	if err := repositoryDao.Create(newRepository); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "error creating repository: "+err.Error())
	}

	return c.String(http.StatusCreated, "Repository created.\n")
}

// DeleteRepository godoc
// @summary 		Delete a repository
// @ID				deleteRepository
// @Tags			repositories
// @Success			200
// @Router			/repositories/:uuid [delete]
func deleteRepository(c echo.Context) error {
	repo := models.RepositoryConfiguration{}
	id := c.Param("uuid")
	db.DB.Find(&repo, "uuid = ?", id)
	if repo.UUID == "" {
		return echo.NewHTTPError(http.StatusNotFound, "Could not find Repository with id "+id)
	} else {
		db.DB.Delete(&repo)
		return c.JSON(http.StatusNoContent, "")
	}
}

//Converts the database model to our response object
func convertToItems(repoConfigs []models.RepositoryConfiguration) []api.Repository {
	repos := make([]api.Repository, len(repoConfigs))
	for i := 0; i < len(repoConfigs); i++ {
		repos[i].FromRepositoryConfiguration(repoConfigs[i])
	}
	return repos
}
