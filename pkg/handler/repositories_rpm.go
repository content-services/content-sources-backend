package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/labstack/echo/v4"
)

type RepositoryRpmRequest struct {
	UUID string `param:"uuid"`
}

func RegisterRepositoryRpmRoutes(engine *echo.Group, rDao *dao.RepositoryDao) {
	engine.GET("/repositories/:uuid/rpms", listRepositoriesRpm)
}

// listRepositoriesRpm godoc
// @Summary      List Repositories RPMS
// @ID           listRepositoriesRpms
// @Description  get repositories RPMS
// @Tags         repositories,rpms
// @Accept       json
// @Produce      json
// @Success      200 {object} api.RepositoryRpmCollectionResponse
// @Router       /repositories/:uuid/rpms [get]
//
func listRepositoriesRpm(c echo.Context) error {
	// Read input information
	var rpmInput RepositoryRpmRequest
	if err := (&echo.DefaultBinder{}).BindPathParams(c, &rpmInput); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	accountNumber, orgId, err := getAccountIdOrgId(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Request record from database
	repoConfig := &models.RepositoryConfiguration{}
	if err := db.DB.First(repoConfig,
		"uuid = ? AND org_id = ? AND account_id = ?",
		rpmInput.UUID,
		orgId,
		accountNumber,
	).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "repository_configurations:"+err.Error())
	}

	// Retrieve the linked repository record
	repo := &models.Repository{}
	if err := db.DB.First(repo,
		"refer_repo_config = ?", repoConfig.UUID,
	).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "repositories:"+err.Error())
	}

	// Set pagination information
	var total int64
	items := make([]models.RepositoryRpm, 0)
	page := ParsePagination(c)
	db.DB.Where("repo_refer like ?", repo.UUID).Count(&total)
	db.DB.Limit(page.Limit).Offset(page.Offset).Find(&items)

	// Return rpm collection
	rpms := fromRepositoryRpm2Response(items)
	return c.JSON(200, setCollectionResponseMetadata(&api.RepositoryRpmCollectionResponse{Data: rpms}, c, total))
}

// fromRepositoryRpm2Response Converts the database model to our response object
func fromRepositoryRpm2Response(rpms []models.RepositoryRpm) []api.RepositoryRpm {
	items := make([]api.RepositoryRpm, len(rpms))
	for i := 0; i < len(rpms); i++ {
		items[i].FromRepositoryRpm(rpms[i])
	}
	return items
}
