package handler

import (
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
)

const (
	defaultSearchRpmLimit = 20
)

type RepositoryRpmHandler struct {
	Dao dao.RpmDao
}

type RepositoryRpmRequest struct {
	UUID   string `param:"uuid"`
	Search string `query:"search"`
	SortBy string `query:"sort_by"`
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
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /rpms/names [post]
func (rh *RepositoryRpmHandler) searchRpmByName(c echo.Context) error {
	_, orgId := getAccountIdOrgId(c)
	dataInput := api.SearchRpmRequest{}
	if err := c.Bind(&dataInput); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	rh.searchRpmPreprocessInput(&dataInput)

	limit := defaultSearchRpmLimit
	apiResponse, err := rh.Dao.Search(orgId, dataInput, limit)
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error searching RPMs", err.Error())
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
// @Param		 limit query int false "Limit the number of items returned"
// @Param		 offset query int false "Offset into the list of results to return in the response"
// @Param		 search query string false "Search term for name."
// @Param		 sort_by query string false "Sets the sort order of the results."
// @Success      200 {object} api.RepositoryRpmCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/rpms [get]
func (rh *RepositoryRpmHandler) listRepositoriesRpm(c echo.Context) error {
	// Read input information
	rpmInput := api.RepositoryRpmRequest{}
	if err := c.Bind(&rpmInput); err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error binding parameters", err.Error())
	}

	_, orgId := getAccountIdOrgId(c)
	page := ParsePagination(c)

	// Request record from database
	dao := rh.Dao
	apiResponse, total, err := dao.List(orgId, rpmInput.UUID, page.Limit, page.Offset, rpmInput.Search, rpmInput.SortBy)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error listing RPMs", err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&apiResponse, c, total))
}
