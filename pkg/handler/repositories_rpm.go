package handler

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func RegisterRepositoryRpmRoutes(engine *echo.Group) {
	engine.GET("/repositories/:uuid/rpms", listRepositoriesRpm)
}

// listRepositoriesRpm godoc
// @Summary      List Repositories RPMS
// @ID           listRepositoriesRpms
// @Description  get repositories RPMS
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Success      200 {object} api.RepositoryRpmCollectionResponse
// @Router       /repositories/:uuid [get]
func listRepositoriesRpm(c echo.Context) error {
	var uuid uuid.UUID
	if err := (&echo.DefaultBinder{}).BindPathParams(c, &uuid); err != nil {
		return err
	}
	repoConfig := &models.RepositoryConfiguration{}
	db.DB.Find(repoConfig, "uuid = ?", uuid)
	if repoConfig == nil {
		return fmt.Errorf("repoConfig not found for uuid='%q'", uuid)
	}

	var total int64
	items := make([]models.RepositoryRpm, 0)
	page := ParsePagination(c)
	db.DB.Find(&items).Count(&total)
	db.DB.Limit(page.Limit).Offset(page.Offset).Find(&items)

	rpms := fromRepositoryRpm2Response(items)
	return c.JSON(200, collectionResponse(&api.RepositoryRpmCollectionResponse{Data: rpms}, c, total))
}

// fromRepositoryRpm2Response Converts the database model to our response object
func fromRepositoryRpm2Response(rpms []models.RepositoryRpm) []api.RepositoryRpm {
	items := make([]api.RepositoryRpm, len(rpms))
	for i := 0; i < len(rpms); i++ {
		items[i].FromRepositoryRpm(rpms[i])
	}
	return items
}
