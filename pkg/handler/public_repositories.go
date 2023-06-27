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
// @Description  Get public repositories
// @Tags         public_repositories
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
