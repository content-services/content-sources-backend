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

type RepositoryPackageGroupHandler struct {
	Dao dao.DaoRegistry
}

func RegisterPackageGroupRoutes(engine *echo.Group, rDao *dao.DaoRegistry) {
	rh := RepositoryPackageGroupHandler{
		Dao: *rDao,
	}

	addRoute(engine, http.MethodGet, "/repositories/:uuid/package_groups", rh.listRepositoriesPackageGroups, rbac.RbacVerbRead)
	addRoute(engine, http.MethodPost, "/package_groups/names", rh.searchPackageGroupByName, rbac.RbacVerbRead)
	addRoute(engine, http.MethodPost, "/snapshots/package_groups/names", rh.searchSnapshotPackageGroups, rbac.RbacVerbRead)
}

// searchPackageGroupByName godoc
// @Summary      Search package groups
// @ID           searchPackageGroup
// @Description  This enables users to search for package groups in a given list of repositories.
// @Tags         repositories,packagegroups
// @Accept       json
// @Produce      json
// @Param        body  body   api.ContentUnitSearchRequest  true  "request body"
// @Success      200 {object} []api.SearchPackageGroupResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /package_groups/names [post]
func (rh *RepositoryPackageGroupHandler) searchPackageGroupByName(c echo.Context) error {
	_, orgId := getAccountIdOrgId(c)
	dataInput := api.ContentUnitSearchRequest{}
	if err := c.Bind(&dataInput); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	preprocessInput(&dataInput)

	apiResponse, err := rh.Dao.PackageGroup.Search(orgId, dataInput)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error searching package groups", err.Error())
	}

	return c.JSON(200, apiResponse)
}

// listRepositoriesPackageGroups godoc
// @Summary      List Repositories Package Groups
// @ID           listRepositoriesPackageGroups
// @Description  List package groups in a repository.
// @Tags         repositories,packagegroups
// @Accept       json
// @Produce      json
// @Param		 uuid	path string true "Repository ID."
// @Param		 limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Param		 offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param		 search query string false "Term to filter and retrieve items that match the specified search criteria. Search term can include name."
// @Param		 sort_by query string false "Sort the response based on specific repository parameters. Sort criteria can include `id`, `name`, `description`, and `package_list`."
// @Success      200 {object} api.RepositoryPackageGroupCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/package_groups [get]
func (rh *RepositoryPackageGroupHandler) listRepositoriesPackageGroups(c echo.Context) error {
	// Read input information
	packageGroupInput := api.ContentUnitListRequest{}
	if err := c.Bind(&packageGroupInput); err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error binding parameters", err.Error())
	}

	_, orgId := getAccountIdOrgId(c)
	page := ParsePagination(c)

	// Request record from database
	apiResponse, total, err := rh.Dao.PackageGroup.List(orgId, packageGroupInput.UUID, page.Limit, page.Offset, packageGroupInput.Search, packageGroupInput.SortBy)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing package groups", err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&apiResponse, c, total))
}

// searchSnapshotPackageGroups godoc
// @Summary      Search package groups within snapshots
// @ID           searchSnapshotPackageGroups
// @Description  This enables users to search for package groups in a given list of snapshots.
// @Tags         snapshots,environments
// @Accept       json
// @Produce      json
// @Param        body  body   api.SnapshotSearchRpmRequest  true  "request body"
// @Success      200 {object} []api.SearchPackageGroupResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /snapshots/package_groups/names [post]
func (rh *RepositoryPackageGroupHandler) searchSnapshotPackageGroups(c echo.Context) error {
	_, orgId := getAccountIdOrgId(c)
	dataInput := api.SnapshotSearchRpmRequest{}

	var err error
	err = CheckSnapshotAccessible(c.Request().Context())
	if err != nil {
		return err
	}

	if err = c.Bind(&dataInput); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	if dataInput.Limit == nil || *dataInput.Limit > api.SearchRpmRequestLimitDefault {
		dataInput.Limit = pointy.Pointer(api.SearchRpmRequestLimitDefault)
	}

	resp, err := rh.Dao.PackageGroup.SearchSnapshotPackageGroups(c.Request().Context(), orgId, dataInput)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error searching package groups", err.Error())
	}
	return c.JSON(200, resp)
}
