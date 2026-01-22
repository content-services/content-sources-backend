package handler

import (
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

const RequestTimeout = time.Second * 3

type RepositoryParameterHandler struct {
	dao                  dao.DaoRegistry
	FeatureServiceClient feature_service_client.FeatureServiceClient
}

func RegisterRepositoryParameterRoutes(engine *echo.Group, dao *dao.DaoRegistry, fsClient *feature_service_client.FeatureServiceClient) {
	rph := RepositoryParameterHandler{
		dao:                  *dao,
		FeatureServiceClient: *fsClient,
	}

	addRepoRoute(engine, http.MethodGet, "/repository_parameters/", rph.listParameters, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodPost, "/repository_parameters/external_gpg_key/", rph.fetchGpgKey, rbac.RbacVerbWrite)
	addRepoRoute(engine, http.MethodPost, "/repository_parameters/validate/", rph.validate, rbac.RbacVerbWrite)
}

// FetchGpgKeys godoc
// @Summary      Fetch gpgkey from URL
// @ID           fetchGpgKey
// @Description  Fetch a gpgkey from a remote repo.
// @Tags         gpgKey
// @Accept       json
// @Produce      json
// @Param        body  body     api.FetchGPGKeyRequest  true  "request body"
// @Success      200 {object} api.FetchGPGKeyResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      415 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repository_parameters/external_gpg_key/ [post]
func (rh *RepositoryParameterHandler) fetchGpgKey(c echo.Context) error {
	var gpgKeyParams api.FetchGPGKeyRequest

	if err := c.Bind(&gpgKeyParams); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	transport := http.Transport{ResponseHeaderTimeout: RequestTimeout}
	client := http.Client{Timeout: RequestTimeout, Transport: &transport}
	gpgKey, _, err := yum.FetchGPGKey(c.Request().Context(), gpgKeyParams.URL, &client)
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
// @Description  List repository parameters.
// @Tags         repositories
// @Accept       json
// @Produce      json
// @Success      200 {object} api.RepositoryParameterResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Router       /repository_parameters/ [get]
func (rh *RepositoryParameterHandler) listParameters(c echo.Context) error {
	_, orgID := getAccountIdOrgId(c)

	features, err := rh.FeatureServiceClient.GetEntitledFeatures(c.Request().Context(), orgID)
	if err != nil {
		log.Error().Err(err).Msg("error getting entitled features, proceeding with default")
	}

	filteredMinorVersions := filterMinorVersionsByFeatures(features)
	filteredExtendedReleaseFeatures := filterExtendedReleaseFeatures(features)

	return c.JSON(200, api.RepositoryParameterResponse{
		DistributionVersions:      config.DistributionVersions[:],
		DistributionMinorVersions: filteredMinorVersions[:],
		DistributionArches:        config.DistributionArches[:],
		ExtendedReleaseFeatures:   filteredExtendedReleaseFeatures[:],
	})
}

// filterMinorVersionsByFeatures filters minor versions based on entitled features
func filterMinorVersionsByFeatures(entitledFeatures []string) []config.DistributionMinorVersion {
	var filtered []config.DistributionMinorVersion
	for _, minorVersion := range config.DistributionMinorVersions[:] {
		for _, feature := range entitledFeatures {
			if slices.Contains(minorVersion.FeatureNames, feature) {
				filtered = append(filtered, minorVersion)
				break
			}
		}
	}
	return filtered
}

// filterExtendedReleaseFeatures filters extended release features based on entitled features
func filterExtendedReleaseFeatures(entitledFeatures []string) []config.ExtendedReleaseFeature {
	var filtered []config.ExtendedReleaseFeature
	for _, feature := range config.ExtendedReleaseFeatures[:] {
		if slices.Contains(entitledFeatures, feature.Label) {
			filtered = append(filtered, feature)
		}
	}
	return filtered
}

// ValidateRepositoryParameters godoc
// @summary 		Validate parameters prior to creating a repository
// @Description  	This validates the parameters before creating a repository. It provides a way to ensure the accuracy and validity of the provided parameters, including a check for the presence of remote yum metadata. Users can perform necessary checks before proceeding with the creation of a repository.
// @ID				validateRepositoryParameters
// @Tags         	repositories
// @Accept       	json
// @Produce      	json
// @Param       	body  body     []api.RepositoryValidationRequest  true  "request body"
// @Success      	200   {object}  []api.RepositoryValidationResponse
// @Failure         400 {object} ce.ErrorResponse
// @Failure      	401 {object} ce.ErrorResponse
// @Failure         404 {object} ce.ErrorResponse
// @Failure      	415 {object} ce.ErrorResponse
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
		go func(slot int, validationParam api.RepositoryValidationRequest) {
			defer wg.Done()
			response, err := rph.dao.RepositoryConfig.ValidateParameters(c.Request().Context(), orgID, validationParam, excludedUUIDs)
			if err == nil {
				validationResponse[slot] = response
			} else {
				errors[slot] = err
			}
		}(i, validationParams[i])
	}
	wg.Wait()

	// Check for any errors and return the first one.  Errors are fatal, not errors retrieving metadata.
	for i := 0; i < len(errors); i++ {
		if errors[i] != nil {
			return ce.NewErrorResponse(ce.HttpCodeForDaoError(errors[i]), "Error validating repository", errors[i].Error())
		}
	}

	return c.JSON(http.StatusOK, validationResponse)
}
