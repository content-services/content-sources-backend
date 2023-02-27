package handler

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/labstack/echo/v4"
)

const RequestTimeout = time.Second * 3

type RepositoryParameterHandler struct {
	RepositoryDao dao.RepositoryConfigDao
}

func RegisterRepositoryParameterRoutes(
	engine *echo.Group,
	repoDao *dao.RepositoryConfigDao,
) {
	rph := RepositoryParameterHandler{RepositoryDao: *repoDao}
	engine.GET("/repository_parameters/", rph.listParameters)
	engine.POST("/repository_parameters/external_gpg_key/", rph.fetchGpgKey)
	engine.POST("/repository_parameters/validate/", rph.validate)
}

// FetchGpgKeys godoc
// @Summary      Fetch gpgkey from URL
// @ID           fetchGpgKey
// @Description  Fetch gpgkey from URL
// @Tags         gpgKey
// @Accept       json
// @Produce      json
// @Success      200 {object} api.FetchGPGKeyResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repository_parameters/external_gpg_key [post]
func (rh *RepositoryParameterHandler) fetchGpgKey(c echo.Context) error {
	var gpgKeyParams api.FetchGPGKeyRequest

	if err := c.Bind(&gpgKeyParams); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	transport := http.Transport{ResponseHeaderTimeout: RequestTimeout}
	client := http.Client{Timeout: RequestTimeout, Transport: &transport}
	gpgKey, _, err := yum.FetchGPGKey(gpgKeyParams.URL, &client)
	if err != nil {
		httpError := ce.NewErrorResponse(http.StatusNotAcceptable, "", "Received response was not a valid GPG Key")
		return httpError
	}

	return c.JSON(http.StatusOK, api.FetchGPGKeyResponse{
		GpgKey: *gpgKey,
	})
}

// ListRepositoryParameters godoc
// @Summary      List Repository Parameters
// @ID           listRepositoryParameters
// @Description  get repository parameters (Versions and Architectures)
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Success      200 {object} api.RepositoryParameterResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Router       /repository_parameters/ [get]
func (rh *RepositoryParameterHandler) listParameters(c echo.Context) error {
	return c.JSON(200, api.RepositoryParameterResponse{
		DistributionVersions: config.DistributionVersions[:],
		DistributionArches:   config.DistributionArches[:],
	})
}

// ValidateRepositoryParameters godoc
// @summary 		Validate parameters prior to creating a repository
// @Description  	Validate parameters prior to creating a repository, including checking if remote yum metadata is present
// @ID				validateRepositoryParameters
// @Tags         	repositories
// @Accept       	json
// @Produce      	json
// @Param       	body  body     []api.RepositoryValidationRequest  true  "request body"
// @Success      	200   {object}  []api.RepositoryValidationResponse
// @Failure         400 {object} ce.ErrorResponse
// @Failure      	401 {object} ce.ErrorResponse
// @Failure         404 {object} ce.ErrorResponse
// @Failure         500 {object} ce.ErrorResponse
// @Router			/repository_parameters/validate/ [post]
func (rph *RepositoryParameterHandler) validate(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)

	var validationParams []api.RepositoryValidationRequest

	if err := c.Bind(&validationParams); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	repoCount := len(validationParams)
	if BulkCreateLimit < repoCount {
		limitErrMsg := fmt.Sprintf("Cannot validate more than %d repositories at once.", BulkCreateLimit)
		return ce.NewErrorResponse(http.StatusRequestEntityTooLarge, "", limitErrMsg)
	}

	// Create arrays to hold results and errors
	validationResponse := make([]api.RepositoryValidationResponse, repoCount)
	errors := make([]error, repoCount)

	excludedUUIDs := []string{}
	for i := 0; i < repoCount; i++ {
		if validationParams[i].UUID != nil {
			excludedUUIDs = append(excludedUUIDs, *validationParams[i].UUID)
		}
	}

	// Use go routine here to reduce the api call time length.
	// Each url validation can take seconds to fail in case of a timeout.
	// This makes the call roughly take the amount of time as a single timeout at worst.
	var wg sync.WaitGroup
	wg.Add(len(validationParams))
	for i := 0; i < len(validationParams); i++ {
		rDao := rph.RepositoryDao
		go func(slot int, validationParam api.RepositoryValidationRequest) {
			response, err := rDao.ValidateParameters(orgID, validationParam, excludedUUIDs)
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
			return c.JSON(ce.HttpCodeForDaoError(errors[i]), errors[i].Error())
		}
	}

	return c.JSON(http.StatusOK, validationResponse)
}
