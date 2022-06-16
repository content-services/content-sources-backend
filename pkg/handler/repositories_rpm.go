package handler

import (
	"encoding/json"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/labstack/echo/v4"
)

type RPMRequest struct {
	Identity string `header:"x-rh-identity"`
	UUID     string `param:"uuid"`
}

type XRHIdentity struct {
	account_number string `json:"identity.account_number"`
	org_id         string `json:"identity.internal.org_id"`
}

func RegisterRepositoryRpmRoutes(engine *echo.Group, rDao *dao.RepositoryDao) {
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
// @Router       /repositories/:uuid/rpms [get]
//
func listRepositoriesRpm(c echo.Context) error {
	var rpmInput RPMRequest
	if err := (&echo.DefaultBinder{}).BindPathParams(c, &rpmInput); err != nil {
		return err
	}
	var rhIdentity XRHIdentity
	if err := (&echo.DefaultBinder{}).BindHeaders(c, &rhIdentity); err != nil {
		return err
	}
	json.Unmarshal([]byte(rpmInput.Identity), &rpmInput)
	repoConfig := &models.RepositoryConfiguration{}
	db.DB.First(repoConfig,
		"uuid = ?", rpmInput.UUID,
		"org_id = ?", rhIdentity.org_id,
		"account_id = ?", rhIdentity.account_number,
	)
	repo := &models.Repository{}
	db.DB.First(repo,
		"refer_repo_config = ?", repoConfig.UUID,
	)
	var total int64
	items := make([]models.RepositoryRpm, 0)
	page := ParsePagination(c)
	db.DB.Where("repo_refer like ?", repo.UUID).Count(&total)
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
