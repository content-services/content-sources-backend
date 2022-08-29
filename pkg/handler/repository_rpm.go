package handler

import (
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
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
// @Param        body  body   api.SearchRpmRequest  true  "request body"
// @Success      200 {object} []api.SearchRpmResponse
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
	rh.searchRpmPreprocessInput(&dataInput)

	limit := defaultSearchRpmLimit
	apiResponse, err := rh.Dao.Search(orgId, dataInput, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(200, apiResponse)
}

func (rh *RepositoryRpmHandler) searchRpmPreprocessInput(input *api.SearchRpmRequest) {
	for i, url := range input.URLs {
		input.URLs[i] = strings.TrimSuffix(url, "/")
	}
}

// listRepositoriesRpm godoc
// @Summary      List Repositories RPMs
// @ID           listRepositoriesRpms
// @Description  list repositories RPMs
// @Tags         repositories,rpms
// @Accept       json
// @Produce      json
// @Param		 uuid	path string true "Identifier of the Repository"
// @Success      200 {object} api.RepositoryRpmCollectionResponse
// @Router       /repositories/{uuid}/rpms [get]
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
	dao := rh.Dao
	apiResponse, total, err := dao.List(orgId, rpmInput.UUID, page.Limit, page.Offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&apiResponse, c, total))
}
