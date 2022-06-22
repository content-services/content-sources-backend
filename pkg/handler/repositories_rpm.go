package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/labstack/echo/v4"
)

type RepositoryRpmRequest struct {
	UUID string `param:"uuid"`
}

func RegisterRepositoryRpmRoutes(engine *echo.Group /*, rDao *dao.RepositoryRpmDao */) {
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
	page := ParsePagination(c)

	// Request record from database
	dao := dao.GetRepositoryRpmDao(db.DB)
	apiResponse, total, err := dao.List(orgId, accountNumber, rpmInput.UUID, page.Limit, page.Offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&apiResponse, c, total))
}
