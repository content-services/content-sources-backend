package api

import "github.com/content-services/content-sources-backend/pkg/config"

// RepositoryParameterResponse holds data returned by a repositories API response
type RepositoryParameterResponse struct {
	DistributionVersions []config.DistributionVersion `json:"distribution_versions" ` //Versions available for repository creation
	DistributionArches   []config.DistributionArch    `json:"distribution_arches"`    //Architectures available for repository creation
}

type RepositoryValidationRequest struct {
	Name *string `json:"name"` //Name of the remote yum repository
	URL  *string `json:"url"`  //URL of the remote yum repository
}

type RepositoryValidationResponse struct {
	Name GenericAttributeValidationResponse `json:"name"` //Validation response for repository name
	URL  UrlValidationResponse              `json:"url"`  //Validation response for repository url
}

type GenericAttributeValidationResponse struct {
	Skipped bool   `json:"skipped"` //Skipped if the attribute is not passed in for validation
	Valid   bool   `json:"valid"`   //Valid if not skipped and provided attribute is valid to be saved
	Error   string `json:"error"`   //Error message if the attribute is not valid
}

type UrlValidationResponse struct {
	Skipped         bool   `json:"skipped"`          // Skipped if the URL is not passed in for validation
	Valid           bool   `json:"valid"`            //Valid if not skipped and provided attribute is valid to be saved
	Error           string `json:"error"`            //Error message if the attribute is not valid
	HTTPCode        int    `json:"http_code"`        // If the metadata cannot be fetched successfully, the http code that is returned if the http request was completed
	MetadataPresent bool   `json:"metadata_present"` //True if the metadata can be fetched successfully
}
