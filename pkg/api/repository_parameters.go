package api

import "github.com/content-services/content-sources-backend/pkg/config"

// RepositoryParameterResponse holds data returned by a repositories API response
type RepositoryParameterResponse struct {
	DistributionVersions []config.DistributionVersion `json:"distribution_versions" ` //Versions available for repository creation
	DistributionArches   []config.DistributionArch    `json:"distribution_arches"`    //Architectures available for repository creation
}

//Validation request/response
type RepositoryValidationRequest struct {
	Name *string `json:"name"`
	URL  *string `json:"url"` // URL of the remote yum repository
}

type RepositoryValidationResponse struct {
	Name GenericAttributeValidationResponse `json:"name"` //Validation response for repository name
	URL  UrlValidationResponse              `json:"url"`  //Validation response for repository url
}

type GenericAttributeValidationResponse struct {
	Skipped bool   `json:"skipped"` // Skipped if the attribute is not passed in for validation
	Valid   bool   `json:"valid"`
	Error   string `json:"error"`
}

type UrlValidationResponse struct {
	Skipped         bool   `json:"skipped"` // Skipped if the URL is not passed in for validation
	Valid           bool   `json:"valid"`
	Error           string `json:"error"`
	HTTPCode        int    `json:"http_code"`
	MetadataPresent bool   `json:"metadata_present"`
}
