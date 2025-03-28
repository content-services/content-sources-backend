package api

type ContentUnitListRequest struct {
	UUID   string `param:"uuid"`    // Identifier of the repository
	Search string `query:"search"`  // Search string based query to optionally filter-on
	SortBy string `query:"sort_by"` // SortBy sets the sort order of the result
}

type ContentUnitSearchRequest struct {
	URLs                  []string `json:"urls,omitempty"`                    // URLs of repositories to search
	UUIDs                 []string `json:"uuids,omitempty"`                   // List of repository UUIDs to search
	Search                string   `json:"search"`                            // Search string to search content unit names
	ExactNames            []string `json:"exact_names,omitempty"`             // List of names to search using an exact match
	Limit                 *int     `json:"limit,omitempty"`                   // Maximum number of records to return for the search
	IncludePackageSources bool     `json:"include_package_sources,omitempty"` // Whether to include module information
}

const ContentUnitSearchRequestLimitDefault int = 100
const ContentUnitSearchRequestLimitMaximum int = 500
