package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/labstack/echo/v4"
)

type RpmHandler struct {
	Dao dao.DaoRegistry
}

func RegisterRpmRoutes(engine *echo.Group, rDao *dao.DaoRegistry) {
	rh := RpmHandler{
		Dao: *rDao,
	}

	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/rpms", rh.listRepositoriesRpm, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodPost, "/rpms/names", rh.searchRpmByName, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/snapshots/:uuid/rpms", rh.listSnapshotRpm, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/snapshots/:uuid/errata", rh.listSnapshotErrata, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodPost, "/snapshots/rpms/names", rh.searchSnapshotRPMs, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodPost, "/rpms/presence", rh.detectRpmsPresence, rbac.RbacVerbRead)
	addTemplateRoute(engine, http.MethodGet, "/templates/:uuid/rpms", rh.listTemplateRpm, rbac.RbacVerbRead)
	addTemplateRoute(engine, http.MethodGet, "/templates/:uuid/errata", rh.listTemplateErrata, rbac.RbacVerbRead)
}

// searchRpmByName godoc
// @Summary      Search RPMs
// @ID           searchRpm
// @Description  This enables users to search for RPMs (Red Hat Package Manager) in a given list of repositories.
// @Tags         rpms
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

	apiResponse, err := rh.Dao.Rpm.Search(c.Request().Context(), orgId, dataInput)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error searching RPMs", err.Error())
	}

	return c.JSON(200, apiResponse)
}

// listRepositoriesRpm godoc
// @Summary      List Repositories RPMs
// @ID           listRepositoriesRpms
// @Description  List RPMs in a repository.
// @Tags         rpms
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
	apiResponse, total, err := rh.Dao.Rpm.List(c.Request().Context(), orgId, rpmInput.UUID, page.Limit, page.Offset, rpmInput.Search, rpmInput.SortBy)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing RPMs", err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&apiResponse, c, total))
}

// searchSnapshotRPMs godoc
// @Summary      Search RPMs within snapshots
// @ID           searchSnapshotRpms
// @Description  This enables users to search for RPMs (Red Hat Package Manager) in a given list of snapshots.
// @Tags         rpms
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

	err := CheckSnapshotAccessible(c.Request().Context())
	if err != nil {
		return err
	}

	if err = c.Bind(&dataInput); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	if dataInput.Limit == nil || *dataInput.Limit > api.SearchRpmRequestLimitDefault {
		dataInput.Limit = utils.Ptr(api.SearchRpmRequestLimitDefault)
	}

	resp, err := rh.Dao.Rpm.SearchSnapshotRpms(c.Request().Context(), orgId, dataInput)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error searching RPMs", err.Error())
	}
	return c.JSON(200, resp)
}

// listSnapshotRpm godoc
// @Summary      List Snapshot RPMs
// @ID           listSnapshotRpms
// @Description  List RPMs in a repository snapshot.
// @Tags         rpms
// @Accept       json
// @Produce      json
// @Param		 uuid	path string true "Snapshot ID."
// @Param		 limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Param		 offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param		 search query string false "Term to filter and retrieve items that match the specified search criteria. Search term can include name."
// @Success      200 {object} api.SnapshotRpmCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /snapshots/{uuid}/rpms [get]
func (rh *RpmHandler) listSnapshotRpm(c echo.Context) error {
	// Read input information
	rpmInput := api.ContentUnitListRequest{}
	if err := c.Bind(&rpmInput); err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error binding parameters", err.Error())
	}

	_, orgId := getAccountIdOrgId(c)
	page := ParsePagination(c)

	// Request record from database
	data, total, err := rh.Dao.Rpm.ListSnapshotRpms(c.Request().Context(), orgId, []string{rpmInput.UUID}, rpmInput.Search, page)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing RPMs", err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&api.SnapshotRpmCollectionResponse{Data: data}, c, int64(total)))
}

// detectRpmsPresence godoc
// @Summary      Detect RPMs presence
// @ID           detectRpm
// @Description  This enables users to detect presence of RPMs (Red Hat Package Manager) in a given list of repositories.
// @Tags         rpms
// @Accept       json
// @Produce      json
// @Param        body  body   api.DetectRpmsRequest  true  "request body"
// @Success      200 {object} api.DetectRpmsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /rpms/presence [post]
func (rh *RpmHandler) detectRpmsPresence(c echo.Context) error {
	_, orgId := getAccountIdOrgId(c)
	dataInput := api.DetectRpmsRequest{}
	if err := c.Bind(&dataInput); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	apiResponse, err := rh.Dao.Rpm.DetectRpms(c.Request().Context(), orgId, dataInput)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error detecting RPMs", err.Error())
	}

	return c.JSON(200, apiResponse)
}

// listSnapshotErrata godoc
// @Summary      List Snapshot Errata
// @ID           listSnapshotErrata
// @Description  List errata in a repository snapshot.
// @Tags         rpms
// @Accept       json
// @Produce      json
// @Param        uuid path string true "Snapshot ID."
// @Param        limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Param        offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param        search query string false "Term to filter and retrieve items that match the specified search criteria. Search term can include name."
// @Param        type query string false "A comma separated list of types to control api response. Type can include `security`, `enhancement`, `bugfix`, and `other`."
// @Param        severity query string false "A comma separated list of severities to control api response. Severity can include `Important`, `Critical`, `Moderate`, `Low`, and `Unknown`."
// @Param        sort_by query string false "Sort the response based on specific parameters. Sort criteria can include `issued_date`, `updated_date`, `type`, and `severity`."
// @Success      200 {object} api.SnapshotErrataCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /snapshots/{uuid}/errata [get]
func (rh *RpmHandler) listSnapshotErrata(c echo.Context) error {
	// Read input information
	snapshotErrataRequest := api.SnapshotErrataListRequest{}
	if err := c.Bind(&snapshotErrataRequest); err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error binding parameters", err.Error())
	}

	_, orgId := getAccountIdOrgId(c)
	page := ParsePagination(c)

	// Request record from database
	data, total, err := rh.Dao.Rpm.ListSnapshotErrata(
		c.Request().Context(),
		orgId,
		[]string{snapshotErrataRequest.UUID},
		tangy.ErrataListFilters{Search: snapshotErrataRequest.Search, Type: snapshotErrataRequest.Type, Severity: snapshotErrataRequest.Severity},
		page,
	)

	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing Errata", err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&api.SnapshotErrataCollectionResponse{Data: data}, c, int64(total)))
}

// listTemplateRpm godoc
// @Summary      List Template RPMs
// @ID           listTemplateRpms
// @Description  List RPMs in a content template.
// @Tags         rpms
// @Accept       json
// @Produce      json
// @Param		 uuid	path string true "Template ID."
// @Param		 limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Param		 offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param		 search query string false "Term to filter and retrieve items that match the specified search criteria. Search term can include name."
// @Success      200 {object} api.SnapshotRpmCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /templates/{uuid}/rpms [get]
func (rh *RpmHandler) listTemplateRpm(c echo.Context) error {
	// Read input information
	rpmInput := api.ContentUnitListRequest{}
	if err := c.Bind(&rpmInput); err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error binding parameters", err.Error())
	}

	_, orgId := getAccountIdOrgId(c)
	page := ParsePagination(c)

	// Request record from database
	data, total, err := rh.Dao.Rpm.ListTemplateRpms(c.Request().Context(), orgId, rpmInput.UUID, rpmInput.Search, page)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing RPMs", err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&api.SnapshotRpmCollectionResponse{Data: data}, c, int64(total)))
}

// listTemplateErrata godoc
// @Summary      List Template Errata
// @ID           listTemplateErrata
// @Description  List errata in a content template.
// @Tags         templates
// @Accept       json
// @Produce      json
// @Param        uuid path string true "Template ID."
// @Param        limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Param        offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param        search query string false "Term to filter and retrieve items that match the specified search criteria. Search term can include name."
// @Param        type query string false "A comma separated list of types to control api response. Type can include `security`, `enhancement`, `bugfix`, and `other`."
// @Param        severity query string false "A comma separated list of severities to control api response. Severity can include `Important`, `Critical`, `Moderate`, `Low`, and `Unknown`."
// @Param        sort_by query string false "Sort the response based on specific parameters. Sort criteria can include `issued_date`, `updated_date`, `type`, and `severity`."
// @Success      200 {object} api.SnapshotErrataCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /templates/{uuid}/errata [get]
func (rh *RpmHandler) listTemplateErrata(c echo.Context) error {
	// Read input information
	snapshotErrataRequest := api.SnapshotErrataListRequest{}
	if err := c.Bind(&snapshotErrataRequest); err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error binding parameters", err.Error())
	}

	_, orgId := getAccountIdOrgId(c)
	page := ParsePagination(c)

	// Request record from database
	data, total, err := rh.Dao.Rpm.ListTemplateErrata(
		c.Request().Context(),
		orgId,
		snapshotErrataRequest.UUID,
		tangy.ErrataListFilters{Search: snapshotErrataRequest.Search, Type: snapshotErrataRequest.Type, Severity: snapshotErrataRequest.Severity},
		page,
	)

	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing Errata", err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&api.SnapshotErrataCollectionResponse{Data: data}, c, int64(total)))
}
