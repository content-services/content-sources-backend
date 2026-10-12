package handler

import (
	"fmt"
	"net/http"
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
	_, orgID := GetAccountIdOrgId(c)

	features, err := rh.FeatureServiceClient.GetEntitledFeatures(c.Request().Context(), orgID)
	if err != nil {
		log.Error().Err(err).Msg("error getting entitled features, proceeding with default")
	}

	featureProducts := make([]api.FeatureProduct, len(features))
	for i, feature := range features {
		featureProducts[i] = api.FeatureProduct{
			Name:  feature,
			Label: getFeatureLabel(feature),
		}
	}

	extendedReleaseStreams := buildExtendedReleaseStreams(features)

	return c.JSON(200, api.RepositoryParameterResponse{
		DistributionVersions:      config.DistributionVersions[:],
		DistributionMinorVersions: config.DistributionMinorVersions[:],
		DistributionArches:        config.DistributionArches[:],
		ExtendedReleaseStreams:    extendedReleaseStreams,
		FeatureProducts:           featureProducts,
	})
}

// buildExtendedReleaseStreams builds extended release streams with architecture entitlements
func buildExtendedReleaseStreams(entitledFeatures []string) []api.ExtendedReleaseStream {
	// Build a map of entitled features for quick lookup
	entitledMap := make(map[string]bool)
	for _, feature := range entitledFeatures {
		entitledMap[feature] = true
	}

	extendedReleaseArches := []string{config.X8664, config.AARCH64}

	var streams []api.ExtendedReleaseStream
	for streamLabel, streamName := range config.ExtendedReleaseStreamNames {
		stream := api.ExtendedReleaseStream{
			Name:  streamName,
			Label: streamLabel,
		}

		for _, arch := range extendedReleaseArches {
			featureName := config.ExtendedReleaseLabelToFeature[[2]string{streamLabel, arch}]
			entitled := entitledMap[featureName]

			stream.Architectures = append(stream.Architectures, api.ExtendedReleaseArchitecture{
				Name:     arch,
				Label:    arch,
				Entitled: entitled,
			})
		}

		streams = append(streams, stream)
	}

	return streams
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
	_, orgID := GetAccountIdOrgId(c)

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

func getFeatureLabel(featureName string) string {
	switch featureName {
	case "RHEL-OS-x86_64":
		return "Red Hat Enterprise Linux for x86_64"
	case "OPENSHIFT-OCP-x86_64":
		return "Red Hat OpenShift Container Platform"
	case "RHEL-HA-x86_64":
		return "Red Hat Enterprise Linux High Availability for x86_64"
	case "RHEL-EUS-x86_64":
		return "Red Hat Enterprise Linux for x86_64 - Extended Update Support"
	case "RHEL-EUS-aarch64":
		// Nothing returned from candlepin
		return "Red Hat Enterprise Linux for ARM 64 - Extended Update Support"
	case "RHEL-E4S-x86_64":
		return "Red Hat Enterprise Linux for x86_64 - Update Services for SAP Solutions"
	case "RHEL-EEUS-x86_64":
		// Nothing returned from candlepin
		return "Red Hat Enterprise Linux for x86_64 - Enhanced Extended Update Support"
	case "RHEL-EEUS-aarch64":
		// Nothing returned from candlepin
		return "Red Hat Enterprise Linux for ARM 64 - Enhanced Extended Update Support"
	case "RHEL-SAP_Solutions-x86_64":
		return "Red Hat Enterprise Linux for SAP Solutions for x86_64"
	case "RHEL-SAP-x86_64":
		return "Red Hat Enterprise Linux for SAP Applications for x86_64"

	case "lightwell-network":
		return "Lightwell Network"
	case "lightwell-clearinghouse":
		return "Lightwell Clearinghouse"
	case "lightwell-research-institutions":
		return "Lightwell Research Institutions"

	case "RHEL-OS-aarch64":
		return "Red Hat Enterprise Linux for ARM 64"

	default:
		return featureName
	}
}
