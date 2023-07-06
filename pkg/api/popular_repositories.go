package api

// PopularRepositoryResponse holds data returned by the popular repositories API response
type PopularRepositoryResponse struct {
	UUID                 string   `json:"uuid"`                                // UUID of the repository if it exists for the user
	ExistingName         string   `json:"existing_name"`                       // Existing reference name for repository
	SuggestedName        string   `json:"suggested_name"`                      // Suggested name of the popular repository
	URL                  string   `json:"url"`                                 // URL of the remote yum repository
	DistributionVersions []string `json:"distribution_versions" example:"7,8"` // Versions to restrict client usage to
	DistributionArch     string   `json:"distribution_arch" example:"x86_64"`  // Architecture to restrict client usage to
	GpgKey               string   `json:"gpg_key"`                             // GPG key for repository
	MetadataVerification bool     `json:"metadata_verification"`               // Verify packages
}

type PopularRepositoriesCollectionResponse struct {
	Data  []PopularRepositoryResponse `json:"data"`  //
	Meta  ResponseMetadata            `json:"meta"`  // Metadata about the request
	Links Links                       `json:"links"` // Links to other pages of results
}

func (r *PopularRepositoriesCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
