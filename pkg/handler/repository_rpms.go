package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/openlyinc/pointy"
)

type RepositoryRpmHandler struct {
	Dao dao.DaoRegistry
}

func RegisterRepositoryRpmRoutes(engine *echo.Group, rDao *dao.DaoRegistry) {
	rh := RepositoryRpmHandler{
		Dao: *rDao,
	}

	addRoute(engine, http.MethodGet, "/repositories/:uuid/rpms", rh.listRepositoriesRpm, rbac.RbacVerbRead)
	addRoute(engine, http.MethodPost, "/rpms/names", rh.searchRpmByName, rbac.RbacVerbRead)
}

// searchRpmByName godoc
// @Summary      Search RPMs
// @ID           searchRpm
// @Description  Search RPMs for a given list of repositories as URLs or UUIDs
// @Tags         repositories,rpms
// @Accept       json
// @Produce      json
// @Param        body  body   api.SearchRpmRequest  true  "request body"
// @Success      200 {object} []api.SearchRpmResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /rpms/names [post]
func (rh *RepositoryRpmHandler) searchRpmByName(c echo.Context) error {
	_, orgId := getAccountIdOrgId(c)
	dataInput := api.SearchRpmRequest{}
	if err := c.Bind(&dataInput); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	rh.searchRpmPreprocessInput(&dataInput)

	apiResponse, err := rh.Dao.Rpm.Search(orgId, dataInput)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error searching RPMs", err.Error())
	}

	return c.JSON(200, apiResponse)
}

func (rh *RepositoryRpmHandler) searchRpmPreprocessInput(input *api.SearchRpmRequest) {
	if input == nil {
		return
	}
	for i, url := range input.URLs {
		input.URLs[i] = removeEndSuffix(url, "/")
	}
	if input.Limit == nil {
		input.Limit = pointy.Int(api.SearchRpmRequestLimitDefault)
	}
	if *input.Limit > api.SearchRpmRequestLimitMaximum {
		*input.Limit = api.SearchRpmRequestLimitMaximum
	}
}

// listRepositoriesRpm godoc
// @Summary      List Repositories RPMs
// @ID           listRepositoriesRpms
// @Description  list repositories RPMs
// @Tags         repositories,rpms
// @Accept       json
// @Produce      json
// @Param		 uuid	path string true "Repository ID."
// @Param		 limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Param		 offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param		 search query string false "Term to filter and retrieve items that match the specified search criteria. Search term can include name."
// @Param		 sort_by query string false "Sort the response based on specific repository parameters. Sort criteria can include `name`, `url`, `status`, and `package_count`."
// @Success      200 {object} api.RepositoryRpmCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
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
	apiResponse, total, err := rh.Dao.Rpm.List(orgId, rpmInput.UUID, page.Limit, page.Offset, rpmInput.Search, rpmInput.SortBy)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing RPMs", err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&apiResponse, c, total))
}
