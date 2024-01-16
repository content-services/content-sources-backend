package api

type RepositoryEnvironment struct {
	UUID        string `json:"uuid"`        // Identifier of the environment
	ID          string `json:"id"`          // The environment ID
	Name        string `json:"name"`        // The environment name
	Description string `json:"description"` // The environment description
}

type RepositoryEnvironmentCollectionResponse struct {
	Data  []RepositoryEnvironment `json:"data"`  // List of environments
	Meta  ResponseMetadata        `json:"meta"`  // Metadata about the request
	Links Links                   `json:"links"` // Links to other pages of results
}

type SearchEnvironmentResponse struct {
	EnvironmentName string `json:"environment_name"` // Environment found
	Description     string `json:"description"`      // Description of the environment found
	ID              string `json:"id"`               // ID of the environment found
}

// SetMetadata Map metadata to the collection.
// meta Metadata about the request.
// links Links to other pages of results.
func (r *RepositoryEnvironmentCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
