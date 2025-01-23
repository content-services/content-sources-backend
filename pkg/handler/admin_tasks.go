package handler

import (
	"net/http"
	"strconv"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	fs "github.com/content-services/content-sources-backend/pkg/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type AdminTaskHandler struct {
	DaoRegistry dao.DaoRegistry
	fsClient    fs.FeatureServiceClient
	cpClient    candlepin_client.CandlepinClient
}

func checkAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := CheckAdminTaskAccessible(c.Request().Context()); err != nil {
			return err
		}
		return next(c)
	}
}

func RegisterAdminTaskRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, fsClient *fs.FeatureServiceClient, cpClient *candlepin_client.CandlepinClient) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("taskInfoReg is nil")
	}
	if fsClient == nil {
		panic("adminClient is nil")
	}
	if cpClient == nil {
		panic("candlepinClient is nil")
	}

	adminTaskHandler := AdminTaskHandler{
		DaoRegistry: *daoReg,
		fsClient:    *fsClient,
		cpClient:    *cpClient,
	}
	addRepoRoute(engine, http.MethodGet, "/admin/tasks/", adminTaskHandler.listTasks, rbac.RbacVerbRead, checkAccessible)
	addRepoRoute(engine, http.MethodGet, "/admin/tasks/:uuid", adminTaskHandler.fetch, rbac.RbacVerbRead, checkAccessible)
	addRepoRoute(engine, http.MethodGet, "/admin/features/", adminTaskHandler.listFeatures, rbac.RbacVerbRead, checkAccessible)
	addRepoRoute(engine, http.MethodGet, "/admin/features/:name/content/", adminTaskHandler.listContentForFeature, rbac.RbacVerbRead, checkAccessible)
}

func (adminTaskHandler *AdminTaskHandler) listTasks(c echo.Context) error {
	pageData := ParsePagination(c)
	filterData := ParseAdminTaskFilters(c)

	tasks, totalTasks, err := adminTaskHandler.DaoRegistry.AdminTask.List(c.Request().Context(), pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing tasks", err.Error())
	}

	return c.JSON(http.StatusOK, setCollectionResponseMetadata(&tasks, c, totalTasks))
}

func (adminTaskHandler *AdminTaskHandler) fetch(c echo.Context) error {
	id := c.Param("uuid")

	response, err := adminTaskHandler.DaoRegistry.AdminTask.Fetch(c.Request().Context(), id)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching task", err.Error())
	}
	return c.JSON(http.StatusOK, response)
}

func (adminTaskHandler *AdminTaskHandler) listFeatures(c echo.Context) error {
	resp, statusCode, err := adminTaskHandler.fsClient.ListFeatures(c.Request().Context())
	if err != nil {
		return ce.NewErrorResponse(statusCode, "Error listing features", err.Error())
	}

	subsAsFeatResp := api.ListFeaturesResponse{}
	for _, content := range resp.Content {
		subsAsFeatResp.Features = append(subsAsFeatResp.Features, content.Name)
	}

	return c.JSON(http.StatusOK, subsAsFeatResp)
}

func (adminTaskHandler *AdminTaskHandler) listContentForFeature(c echo.Context) error {
	name := c.Param("name")
	_, orgID := getAccountIdOrgId(c)

	resp, statusCode, err := adminTaskHandler.fsClient.ListFeatures(c.Request().Context())
	if err != nil {
		return ce.NewErrorResponse(statusCode, "Error listing features", err.Error())
	}

	var found bool
	var engIDs []int
	for _, content := range resp.Content {
		if name == content.Name {
			found = true
			engIDs = content.Rules.MatchProducts[0].EngIDs
		}
	}
	if !found {
		return ce.NewErrorResponse(http.StatusNotFound, "Error listing content", "feature name not found")
	}

	var products []*caliri.ProductDTO
	for _, engID := range engIDs {
		product, err := adminTaskHandler.cpClient.FetchProduct(c.Request().Context(), candlepin_client.OwnerKey(orgID), strconv.Itoa(engID))
		if err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error fetching product", err.Error())
		}
		if product != nil {
			products = append(products, product)
		}
	}

	var contents []api.FeatureServiceContentResponse
	for _, product := range products {
		contents = append(contents, fs.ProductToRepoJSON(product, name)...)
	}
	return c.JSON(http.StatusOK, contents)
}

func ParseAdminTaskFilters(c echo.Context) api.AdminTaskFilterData {
	filterData := api.AdminTaskFilterData{
		AccountId: DefaultAccountId,
		OrgId:     DefaultOrgId,
		Status:    DefaultStatus,
	}
	err := echo.QueryParamsBinder(c).
		String("account_id", &filterData.AccountId).
		String("org_id", &filterData.OrgId).
		String("status", &filterData.Status).
		String("type", &filterData.Typename).
		BindError()

	if err != nil {
		log.Ctx(c.Request().Context()).Info().Err(err).Msg("error parsing filters")
	}

	return filterData
}
