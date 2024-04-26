package handler

import (
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

type PopularRepositoriesHandler struct {
	Dao dao.DaoRegistry
}

func RegisterPopularRepositoriesRoutes(engine *echo.Group, dao *dao.DaoRegistry) {
	rph := PopularRepositoriesHandler{Dao: *dao}
	addRoute(engine, http.MethodGet, "/popular_repositories/", rph.listPopularRepositories, rbac.RbacVerbRead)
}

// ListPopularRepositories godoc
// @Summary      List Popular Repositories
// @ID           listPopularRepositories
// @Description  This operation enables retrieving a paginated list of repository suggestions that are commonly used.
// @Tags         popular_repositories
// @Param        offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param		     limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Param		     search query string false "Term to filter and retrieve items that match the specified search criteria. Search term can include name or URL."
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PopularRepositoriesCollectionResponse
// @Router       /popular_repositories/ [get]
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
func (rh *PopularRepositoriesHandler) listPopularRepositories(c echo.Context) error {
	filters := ParseFilters(c)
	pageData := ParsePagination(c)

	var popRepos []api.PopularRepositoryResponse
	for _, popRepo := range config.PopularRepos {
		popRepos = append(popRepos, api.PopularRepositoryResponse(popRepo))
	}

	filteredData, totalCount := filterPopularRepositories(popRepos, filters, pageData)

	// We should likely call the db directly here to reduce this down to one query if this list get's larger.
	for i := 0; i < len(filteredData.Data); i++ {
		err := rh.updateIfExists(c, &filteredData.Data[i])

		if err != nil {
			return ce.NewErrorResponseFromError("Could not get repository list", err)
		}
	}

	return c.JSON(200, setCollectionResponseMetadata(&filteredData, c, totalCount))
}

func filterPopularRepositories(configData []api.PopularRepositoryResponse, filters api.FilterData, pageData api.PaginationData) (api.PopularRepositoriesCollectionResponse, int64) {
	filteredData := filterBySearchQuery(configData, filters.Search)

	totalCount := len(filteredData)

	if pageData.Offset < 0 || pageData.Offset >= totalCount {
		return api.PopularRepositoriesCollectionResponse{Data: []api.PopularRepositoryResponse{}}, int64(totalCount)
	} else if pageData.Offset+pageData.Limit > totalCount {
		filteredData = filteredData[pageData.Offset:]
	} else {
		filteredData = filteredData[pageData.Offset : pageData.Offset+pageData.Limit]
	}

	return api.PopularRepositoriesCollectionResponse{Data: filteredData}, int64(totalCount)
}

func (rh *PopularRepositoriesHandler) updateIfExists(c echo.Context, repo *api.PopularRepositoryResponse) error {
	_, orgID := getAccountIdOrgId(c)

	// Go get the records for this URL
	repos, _, err := rh.Dao.RepositoryConfig.List(c.Request().Context(), orgID, api.PaginationData{Limit: 1}, api.FilterData{Search: repo.URL})
	if err != nil {
		return ce.NewErrorResponseFromError("Could not get repository list", err)
	}

	// If the URL exists update the "existingName" field
	if len(repos.Data) > 0 && repos.Data[0].Name != "" {
		repo.ExistingName = repos.Data[0].Name
		repo.UUID = repos.Data[0].UUID
	}

	return nil
}

func filterBySearchQuery(data []api.PopularRepositoryResponse, searchQuery string) []api.PopularRepositoryResponse {
	filteredData := make([]api.PopularRepositoryResponse, 0)

	for _, item := range data {
		if strings.Contains(item.URL+item.SuggestedName, searchQuery) {
			filteredData = append(filteredData, item)
		}
	}

	return filteredData
}
