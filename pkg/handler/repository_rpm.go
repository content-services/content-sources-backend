package handler

import (
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/labstack/echo/v4"
)

const (
	defaultSearchRpmLimit = 20
)

type RepositoryRpmHandler struct {
	Dao dao.RpmDao
}

type RepositoryRpmRequest struct {
	UUID string `param:"uuid"`
}

func RegisterRepositoryRpmRoutes(engine *echo.Group, rDao *dao.RpmDao) {
	rh := RepositoryRpmHandler{
		Dao: *rDao,
	}
	engine.GET("/repositories/:uuid/rpms", rh.listRepositoriesRpm)
	engine.POST("/rpms/names", rh.searchRpmByName)
}

// searchRpmByName godoc
// @Summary      Search RPMs
// @ID           searchRpm
// @Description  Search RPMs for a given list of repository URLs
// @Tags         repositories,rpms
// @Accept       json
// @Produce      json
// @Success      200 {object} api.SearchRpmRequest
// @Router       /rpms/names [post]
func (rh *RepositoryRpmHandler) searchRpmByName(c echo.Context) error {
	_, orgId, err := getAccountIdOrgId(c)
	if err != nil {
		return badIdentity(err)
	}
	dataInput := api.SearchRpmRequest{}
	if err = c.Bind(&dataInput); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Error binding params: "+err.Error())
	}

	limit := defaultSearchRpmLimit
	apiResponse, err := rh.Dao.Search(orgId, dataInput, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(200, apiResponse)
}

func (rh *RepositoryRpmHandler) searchRpmPreprocessInput(input *api.SearchRpmRequest) error {
	for i, url := range input.URLs {
		input.URLs[i] = strings.TrimSuffix(url, "/")
	}
	return nil
}

// listRepositoriesRpm godoc
// @Summary      List Repositories RPMs
// @ID           listRepositoriesRpms
// @Description  get repositories RPMs
// @Tags         repositories,rpms
// @Accept       json
// @Produce      json
// @Success      200 {object} api.RepositoryRpmCollectionResponse
// @Router       /repositories/:uuid/rpms [get]
func (rh *RepositoryRpmHandler) listRepositoriesRpm(c echo.Context) error {
	// Read input information
	var rpmInput RepositoryRpmRequest
	if err := (&echo.DefaultBinder{}).BindPathParams(c, &rpmInput); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	_, orgId, err := getAccountIdOrgId(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	page := ParsePagination(c)

	// Request record from database
	dao := dao.GetRpmDao(db.DB)
	apiResponse, total, err := dao.List(orgId, rpmInput.UUID, page.Limit, page.Offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&apiResponse, c, total))
}
