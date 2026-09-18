package lightwell

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type RepositoryHandler struct {
	DaoRegistry dao.DaoRegistry
}

func RegisterLightwellRepositoryRoutes(engine *echo.Group, daoReg *dao.DaoRegistry) {
	h := RepositoryHandler{
		DaoRegistry: *daoReg,
	}
	addLightwellRoute(engine, http.MethodGet, "/repositories", h.listRepositories, rbac.RbacVerbRead)
}

// listLightwellRepositories godoc
// @Summary      List Lightwell Repositories
// @ID           listLightwellRepositories
// @Description  List repositories filtered to the Lightwell origin.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        offset                   query  int     false  "Starting point for retrieving a subset of results. Default value: `0`."
// @Param        limit                    query  int     false  "Number of items to include in response. Default value: `100`."
// @Param        search                   query  string  false  "Search term to filter by name or URL."
// @Param        name                     query  string  false  "Filter by repository name."
// @Param        content_type             query  string  false  "Filter by content type (e.g. maven, python, npm)."
// @Param        sort_by                  query  string  false  "Sort the response (e.g. name, url, status)."
// @Success      200 {object} api.RepositoryCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories [get]
func (h *RepositoryHandler) listRepositories(c echo.Context) error {
	_, orgID := handler.GetAccountIdOrgId(c)
	pageData := handler.ParsePagination(c)
	filterData := handler.ParseFilters(c)
	filterData.Origin = config.OriginLightwell

	repos, totalRepos, err := h.DaoRegistry.RepositoryConfig.List(c.Request().Context(), orgID, pageData, filterData)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error listing repositories", err.Error())
	}

	h.enrichAdvisoryCounts(c, &repos)

	return c.JSON(200, handler.SetCollectionResponseMetadata(&repos, c, totalRepos))
}

func (h *RepositoryHandler) enrichAdvisoryCounts(c echo.Context, repos *api.RepositoryCollectionResponse) {
	for i := range repos.Data {
		repo := &repos.Data[i]
		repoUUID, err := uuid.Parse(repo.UUID)
		if err != nil {
			log.Ctx(c.Request().Context()).Warn().Err(err).Str("uuid", repo.UUID).Msg("invalid UUID for advisory count")
			continue
		}
		count, err := h.DaoRegistry.LightwellAdvisory.CountAdvisoriesByRepo(c.Request().Context(), repoUUID)
		if err != nil {
			log.Ctx(c.Request().Context()).Warn().Err(err).Str("uuid", repo.UUID).Msg("failed to count advisories")
			continue
		}
		advCount := int(count)
		repo.AdvisoryCount = &advCount
	}
}
