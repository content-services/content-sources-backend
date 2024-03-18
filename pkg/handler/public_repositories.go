package handler

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
)

type PublicRepositoriesHandler struct {
	DaoRegistry dao.DaoRegistry
}

func RegisterPublicRepositoriesRoutes(engine *echo.Group, dao *dao.DaoRegistry) {
	rph := PublicRepositoriesHandler{DaoRegistry: *dao}
	addRoute(engine, http.MethodGet, "/public_repositories/", rph.listPublicRepositories, rbac.RbacVerbRead)
}

// ListPublicRepositories godoc
// @Summary      List Public Repositories
// @ID           listPublicRepositories
// @Description  Get public repositories.
// @Description  This enables listing a set of pre-created entries that represent a base set of RPMs needed for image building. These repositories are defined and made available to all user accounts, enabling them to perform RPM name searches using URLs as search criteria. These public repositories are not listed by the normal repositories API.
// @Description  It does not show up via the normal repositories API.
// @Tags         public_repositories
// @Param		 offset query int false "Starting point for retrieving a subset of results. Determines how many items to skip from the beginning of the result set. Default value:`0`."
// @Param		 limit query int false "Number of items to include in response. Use it to control the number of items, particularly when dealing with large datasets. Default value: `100`."
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PublicRepositoryCollectionResponse
// @Router       /public_repositories/ [get]
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
func (rh *PublicRepositoriesHandler) listPublicRepositories(c echo.Context) error {
	pageData := ParsePagination(c)
	filterData := ParseFilters(c)

	repos, totalRepos, err := rh.DaoRegistry.Repository.ListPublic(c.Request().Context(), pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing repositories", err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&repos, c, totalRepos))
}
