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

type RpmHandler struct {
	Dao dao.DaoRegistry
}

func RepositoryRpmRoutes(engine *echo.Group, rDao *dao.DaoRegistry) {
	rh := RpmHandler{
		Dao: *rDao,
	}

	addRoute(engine, http.MethodGet, "/repositories/:uuid/rpms", rh.listRepositoriesRpm, rbac.RbacVerbRead)
	addRoute(engine, http.MethodPost, "/rpms/names", rh.searchRpmByName, rbac.RbacVerbRead)
	addRoute(engine, http.MethodPost, "/snapshots/rpms/names", rh.searchSnapshotRPMs, rbac.RbacVerbRead)
}

// searchRpmByName godoc
// @Summary      Search RPMs
// @ID           searchRpm
// @Description  This enables users to search for RPMs (Red Hat Package Manager) in a given list of repositories.
// @Tags         repositories,rpms
// @Accept       json
// @Produce      json
// @Param        body  body   api.ContentUnitSearchRequest  true  "request body"
// @Success      200 {object} []api.SearchRpmResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /rpms/names [post]
func (rh *RpmHandler) searchRpmByName(c echo.Context) error {
	_, orgId := getAccountIdOrgId(c)
	dataInput := api.ContentUnitSearchRequest{}
	if err := c.Bind(&dataInput); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	preprocessInput(&dataInput)

	apiResponse, err := rh.Dao.Rpm.Search(orgId, dataInput)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error searching RPMs", err.Error())
	}

	return c.JSON(200, apiResponse)
}

// listRepositoriesRpm godoc
// @Summary      List Repositories RPMs
// @ID           listRepositoriesRpms
// @Description  List RPMs in a repository.
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
func (rh *RpmHandler) listRepositoriesRpm(c echo.Context) error {
	// Read input information
	rpmInput := api.ContentUnitListRequest{}
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

// searchSnapshotRPMs godoc
// @Summary      Search RPMs within snapshots
// @ID           searchSnapshotRpms
// @Description  This enables users to search for RPMs (Red Hat Package Manager) in a given list of snapshots.
// @Tags         snapshots,rpms
// @Accept       json
// @Produce      json
// @Param        body  body   api.SnapshotSearchRpmRequest  true  "request body"
// @Success      200 {object} []api.SearchRpmResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /snapshots/rpms/names [post]
func (rh *RpmHandler) searchSnapshotRPMs(c echo.Context) error {
	_, orgId := getAccountIdOrgId(c)
	dataInput := api.SnapshotSearchRpmRequest{}
	if err := c.Bind(&dataInput); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	if dataInput.Limit == nil || *dataInput.Limit > api.SearchRpmRequestLimitDefault {
		dataInput.Limit = pointy.Pointer(api.SearchRpmRequestLimitDefault)
	}

	resp, err := rh.Dao.Rpm.SearchSnapshotRpms(c.Request().Context(), orgId, dataInput)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error searching RPMs", err.Error())
	}
	return c.JSON(200, resp)
}
