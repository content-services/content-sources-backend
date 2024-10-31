package api

import "github.com/content-services/content-sources-backend/pkg/config"

type FetchGPGKeyResponse struct {
	GpgKey string `json:"gpg_key" ` // The downloaded GPG Keys from the provided url.
}

type FetchGPGKeyRequest struct {
	URL string `json:"url" validate:"required"` // The url from which to download the GPG Key.
}

// RepositoryParameterResponse holds data returned by a repositories API response
type RepositoryParameterResponse struct {
	DistributionVersions []config.DistributionVersion `json:"distribution_versions" ` // Versions available for repository creation
	DistributionArches   []config.DistributionArch    `json:"distribution_arches"`    // Architectures available for repository creation
}

type RepositoryValidationRequest struct {
	Name                 *string `json:"name"`                  // Name of the remote yum repository
	URL                  *string `json:"url"`                   // URL of the remote yum repository
	GPGKey               *string `json:"gpg_key"`               // GPGKey of the remote yum repository
	UUID                 *string `json:"uuid"`                  // If set, this is an "Update" validation
	MetadataVerification bool    `json:"metadata_verification"` // If set, attempt to validate the yum metadata with the specified GPG Key
}

type RepositoryValidationResponse struct {
	Name   GenericAttributeValidationResponse `json:"name"`    // Validation response for repository name
	URL    UrlValidationResponse              `json:"url"`     // Validation response for repository url
	GPGKey GenericAttributeValidationResponse `json:"gpg_key"` // Validation response for the GPG Key
}

type GenericAttributeValidationResponse struct {
	Skipped bool   `json:"skipped"` // Skipped if the attribute is not passed in for validation
	Valid   bool   `json:"valid"`   // Valid if not skipped and the provided attribute is valid
	Error   string `json:"error"`   // Error message if the attribute is not valid
}

type UrlValidationResponse struct {
	Skipped                  bool   `json:"skipped"`                    // Skipped if the URL is not passed in for validation
	Valid                    bool   `json:"valid"`                      // Valid if not skipped and the provided attribute is valid
	Error                    string `json:"error"`                      // Error message if the attribute is not valid
	HTTPCode                 int    `json:"http_code"`                  // If the metadata cannot be fetched successfully, the http code that is returned if the http request was completed
	MetadataPresent          bool   `json:"metadata_present"`           // True if the metadata can be fetched successfully
	MetadataSignaturePresent bool   `json:"metadata_signature_present"` // True if a repomd.xml.sig file was found in the repository
}
