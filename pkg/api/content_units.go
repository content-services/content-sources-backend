package api

type ContentUnitListRequest struct {
	UUID   string `param:"uuid" validate:"required"`    // Identifier of the repository
	Search string `query:"search" validate:"required"`  // Search string based query to optionally filter-on
	SortBy string `query:"sort_by" validate:"required"` // SortBy sets the sort order of the result
}

type ContentUnitSearchRequest struct {
	URLs   []string `json:"urls,omitempty"`             // URLs of repositories to search
	UUIDs  []string `json:"uuids,omitempty"`            // List of repository UUIDs to search
	Search string   `json:"search" validate:"required"` // Search string to search content unit names
	Limit  *int     `json:"limit,omitempty"`            // Maximum number of records to return for the search
}

const ContentUnitSearchRequestLimitDefault int = 100
const ContentUnitSearchRequestLimitMaximum int = 500
