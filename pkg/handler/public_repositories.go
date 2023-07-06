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
// @Description  A public repository is a defined repository that is available to all accounts for the purposes of searching for rpm names by URL.
// @Description  It does not show up via the normal repositories API.
// @Tags         public_repositories
// @Param		 offset query int false "Offset into the list of results to return in the response"
// @Param		 limit query int false "Limit the number of items returned"
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

	repos, totalRepos, err := rh.DaoRegistry.Repository.ListPublic(pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing repositories", err.Error())
	}

	return c.JSON(200, setCollectionResponseMetadata(&repos, c, totalRepos))
}
