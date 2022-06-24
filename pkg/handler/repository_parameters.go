package handler

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/labstack/echo/v4"
)

type RepositoryParameterHandler struct {
}

func RegisterRepositoryParameterRoutes(engine *echo.Group) {
	rph := RepositoryParameterHandler{}
	engine.GET("/repository_parameters/", rph.listParameters)
}

// ListRepositoryParameters godoc
// @Summary      List Repository Parameters
// @ID           listRepositoryParameters
// @Description  get repository parameters (Versions and Architectures)
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Success      200 {object} api.RepositoryParameterResponse
// @Router       /repository_parameters/ [get]
func (rh *RepositoryParameterHandler) listParameters(c echo.Context) error {
	return c.JSON(200, api.RepositoryParameterResponse{
		DistributionVersions: config.DistributionVersions[:],
		DistributionArches:   config.DistributionArches[:],
	})
}
