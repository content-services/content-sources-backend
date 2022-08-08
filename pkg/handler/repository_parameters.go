package handler

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/labstack/echo/v4"
)

type RepositoryParameterHandler struct {
	RepositoryDao dao.RepositoryDao
}

func RegisterRepositoryParameterRoutes(engine *echo.Group, repoDao *dao.RepositoryDao) {
	rph := RepositoryParameterHandler{RepositoryDao: *repoDao}
	engine.GET("/repository_parameters/", rph.listParameters)
	engine.POST("/repository_parameters/validate/", rph.validate)
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

// ValidateRepositoryParameters godoc
// @summary 		Validate parameters prior to creating a repository
// @ID				validateRepositoryParameters
// @Tags			repositories
// @Param       	body  body     []api.RepositoryValidationRequest  true  "request body"
// @Success      	200   {object}  []api.RepositoryValidationResponse
// @Router			/repository_parameters/validate/ [post]
func (rph *RepositoryParameterHandler) validate(c echo.Context) error {
	_, orgID, err := getAccountIdOrgId(c)
	if err != nil {
		return badIdentity(err)
	}

	var validationParams []api.RepositoryValidationRequest

	if err := c.Bind(&validationParams); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "error binding params: "+err.Error())
	}

	repoCount := len(validationParams)
	if BulkCreateLimit < repoCount {
		limitErrMsg := fmt.Sprintf("Cannot validate more than %d repositories at once.", BulkCreateLimit)
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, limitErrMsg)
	}

	// Create arrays to hold results and errors
	validationResponse := make([]api.RepositoryValidationResponse, repoCount)
	errors := make([]error, repoCount)

	//Use go routine here to reduce the api call time length.
	// Each url validation can take seconds to fail in case of a timeout.
	// So this makes the call roughly take the amount of time as a single timeout at worst case
	var wg sync.WaitGroup
	wg.Add(len(validationParams))
	for i := 0; i < len(validationParams); i++ {
		go func(slot int, validationParam api.RepositoryValidationRequest) {
			response, err := rph.RepositoryDao.ValidateParameters(orgID, validationParam)
			if err == nil {
				validationResponse[slot] = response
			} else {
				errors[slot] = err
			}
			wg.Done()
		}(i, validationParams[i])
	}
	wg.Wait()

	//Check for any errors and return the first one.  Errors are fatal, not errors retrieving metadata.
	for i := 0; i < len(errors); i++ {
		if errors[i] != nil {
			return c.JSON(httpCodeForError(errors[i]), errors[i].Error())
		}
	}

	return c.JSON(http.StatusOK, validationResponse)
}
