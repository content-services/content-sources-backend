package api

type ContentUnitListRequest struct {
	UUID   string `param:"uuid"`    // Identifier of the repository
	Search string `query:"search"`  // Search string based query to optionally filter-on
	SortBy string `query:"sort_by"` // SortBy sets the sort order of the result
}

type ContentUnitSearchRequest struct {
	URLs   []string `json:"urls,omitempty"`  // URLs of repositories to search
	UUIDs  []string `json:"uuids,omitempty"` // List of repository UUIDs to search
	Search string   `json:"search"`          // Search string to search content unit names
	Limit  *int     `json:"limit,omitempty"` // Maximum number of records to return for the search
}

type FruitSearchRequest struct {
	Search string `query:"search"` // Search string to search content fruit names
}

const ContentUnitSearchRequestLimitDefault int = 100
const ContentUnitSearchRequestLimitMaximum int = 500
