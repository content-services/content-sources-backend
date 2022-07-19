package api

import "github.com/content-services/content-sources-backend/pkg/config"

// RepositoryParameterResponse holds data returned by a repositories API response
type RepositoryParameterResponse struct {
	DistributionVersions []config.DistributionVersion `json:"distribution_versions" ` //Versions available for repository creation
	DistributionArches   []config.DistributionArch    `json:"distribution_arches"`    //Architectures available for repository creation
}
