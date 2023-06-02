package handler

import (
	"embed"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

//go:embed "popular_repositories.json"

var fs embed.FS

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
// @Description  Get popular repositories
// @Tags         popular_repositories
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PopularRepositoriesCollectionResponse
// @Router       /popular_repositories/ [get]
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
func (rh *PopularRepositoriesHandler) listPopularRepositories(c echo.Context) error {
	jsonConfig, err := fs.ReadFile("popular_repositories.json")

	if err != nil {
		return ce.NewErrorResponseFromError("Could not read popular_repositories.json", err)
	}

	configData := []api.PopularRepositoryResponse{}

	err = json.Unmarshal([]byte(jsonConfig), &configData)
	if err != nil {
		return ce.NewErrorResponseFromError("Could not read popular_repositories.json", err)
	}

	filters := ParseFilters(c)

	filteredData := filterBySearchQuery(configData, filters.Search)

	// We should likely call the db directly here to reduce this down to one query if this list get's larger.
	for i := 0; i < len(filteredData); i++ {
		err := rh.updateIfExists(c, &filteredData[i])

		if err != nil {
			return ce.NewErrorResponseFromError("Could not get repository list", err)
		}
	}

	return c.JSON(200, setCollectionResponseMetadata(&api.PopularRepositoriesCollectionResponse{Data: filteredData}, c, int64(len(filteredData))))
}

func (rh *PopularRepositoriesHandler) updateIfExists(c echo.Context, repo *api.PopularRepositoryResponse) error {
	_, orgID := getAccountIdOrgId(c)
	// Go get the records for this URL
	repos, _, err := rh.Dao.RepositoryConfig.List(orgID, api.PaginationData{}, api.FilterData{Search: repo.URL})
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
