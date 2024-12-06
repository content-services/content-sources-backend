package api

type SearchModuleStreamsRequest struct {
	UUIDs    []string `json:"uuids" validate:"required"`     // List of snapshot UUIDs to search
	RpmNames []string `json:"rpm_names" validate:"required"` // List of rpm names to search
	SortBy   string   `json:"sort_by"`                       // SortBy sets the sort order of the result
	Search   string   `json:"search"`                        // Search string to search rpm names
}

type Stream struct {
	Name        string              `json:"name"`        // Name of the module
	Stream      string              `json:"stream"`      // Module stream version
	Context     string              `json:"context"`     // Context of the module
	Arch        string              `json:"arch"`        // The Architecture of the rpm
	Version     string              `json:"cersion"`     // The version of the rpm
	Description string              `json:"description"` // Module description
	Profiles    map[string][]string `json:"profiles"`    // Module profile data
}

type SearchModuleStreams struct {
	ModuleName string   `json:"module_name"` // Module name
	Streams    []Stream `json:"streams"`     // A list of stream related information for the module
}

type SearchModuleStreamsCollectionResponse struct {
	Data  []SearchModuleStreams `json:"data"`  // Requested Data
	Meta  ResponseMetadata      `json:"meta"`  // Metadata about the request
	Links Links                 `json:"links"` // Links to other pages of results
}

// SetMetadata Map metadata to the collection.
// meta Metadata about the request.
// links Links to other pages of results.
func (r *SearchModuleStreamsCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
