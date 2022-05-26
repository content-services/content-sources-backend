package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/labstack/echo/v4"
)

type RepositoryItem struct {
	UUID                string `json:"uuid"`
	Name                string `json:"name"`
	Url                 string `json:"url"`                                //URL of the remote yum repository
	DistributionVersion string `json:"distribution_version" example:"7"`   //Version to restrict client usage to
	DistributionArch    string `json:"distribution_arch" example:"x86_64"` //Architecture to restrict client usage to
	AccountId           string `json:"account_id"`                         //Account Id of the owner
	OrgId               string `json:"org_id"`                             //Organization Id of the owner
}

func (r *RepositoryItem) FromRepositoryConfiguration(repoConfig models.RepositoryConfiguration) {
	r.UUID = repoConfig.UUID
	r.Name = repoConfig.Name
	r.Url = repoConfig.URL
	r.DistributionVersion = repoConfig.Version
	r.DistributionArch = repoConfig.Arch
	r.AccountId = repoConfig.AccountID
	r.OrgId = repoConfig.OrgID
}

type RepositoryCollectionResponse struct {
	Data  []RepositoryItem `json:"data"`  //Requested Data
	Meta  ResponseMetadata `json:"meta"`  //Metadata about the request
	Links Links            `json:"links"` //Links to other pages of results
}

func (r *RepositoryCollectionResponse) setMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}

func RegisterRepositoryRoutes(engine *echo.Group) {
	engine.GET("/repositories/", listRepositories)
	engine.DELETE("/repositories/:uuid", deleteRepository)
}

// ListRepositories godoc
// @Summary      List Repositories
// @ID           listRepositories
// @Description  get repositories
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Success      200 {object} RepositoryCollectionResponse
// @Router       /repositories [get]
func listRepositories(c echo.Context) error {
	var total int64
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	page := ParsePagination(c)

	db.DB.Find(&repoConfigs).Count(&total)
	db.DB.Limit(page.Limit).Offset(page.Offset).Find(&repoConfigs)

	repos := convertToItems(repoConfigs)
	return c.JSON(200, collectionResponse(&RepositoryCollectionResponse{Data: repos}, c, total))
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
func convertToItems(repoConfigs []models.RepositoryConfiguration) []RepositoryItem {
	repos := make([]RepositoryItem, len(repoConfigs))
	for i := 0; i < len(repoConfigs); i++ {
		repos[i].FromRepositoryConfiguration(repoConfigs[i])
	}
	return repos
}
