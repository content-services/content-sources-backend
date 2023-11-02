package api

type SharedRepositoryEntityRequest struct {
	UUID   string `param:"uuid"`    // Identifier of the repository
	Search string `query:"search"`  // Search string based query to optionally filter-on
	SortBy string `query:"sort_by"` // SortBy sets the sort order of the result
}

type SearchSharedRepositoryEntityRequest struct {
	URLs   []string `json:"urls,omitempty"`  // URLs of repositories to search
	UUIDs  []string `json:"uuids,omitempty"` // List of RepositoryConfig UUIDs to search
	Search string   `json:"search"`          // Search string to search repository entity names
	Limit  *int     `json:"limit,omitempty"` // Maximum number of records to return for the search
}

const SearchSharedRepositoryEntityRequestLimitDefault int = 100
const SearchSharedRepositoryEntityRequestLimitMaximum int = 500
