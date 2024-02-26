package api

// PublicRepositoryResponse holds data returned by the public repositories API response
type PublicRepositoryResponse struct {
	URL                          string `json:"url"`                             // URL of the remote yum repository
	Status                       string `json:"status"`                          // Introspection status of the repository
	LastIntrospectionTime        string `json:"last_introspection_time"`         // Timestamp of last attempted introspection
	LastIntrospectionSuccessTime string `json:"last_success_introspection_time"` // Timestamp of last successful introspection
	LastIntrospectionUpdateTime  string `json:"last_update_introspection_time"`  // Timestamp of last introspection that had updates
	LastIntrospectionError       string `json:"last_introspection_error"`        // Error of last attempted introspection
	PackageCount                 int    `json:"package_count"`                   // Number of packages last read in the repository
}

type PublicRepositoryCollectionResponse struct {
	Data  []PublicRepositoryResponse `json:"data"`  //
	Meta  ResponseMetadata           `json:"meta"`  // Metadata about the request
	Links Links                      `json:"links"` // Links to other pages of results
}

func (r *PublicRepositoryCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
