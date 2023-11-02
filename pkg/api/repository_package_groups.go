package api

type RepositoryPackageGroup struct {
	UUID        string   `json:"uuid"`        // Identifier of the package group
	Name        string   `json:"name"`        // The package group name
	Description string   `json:"description"` // The package group description
	PackageList []string `json:"packagelist"` // The list of packages in the package group
}

type RepositoryPackageGroupCollectionResponse struct {
	Data  []RepositoryPackageGroup `json:"data"`  // List of package groups
	Meta  ResponseMetadata         `json:"meta"`  // Metadata about the request
	Links Links                    `json:"links"` // Links to other pages of results
}

type RepositoryPackageGroupRequest struct {
	UUID   string `param:"uuid"`    // Identifier of the repository
	Search string `query:"search"`  // Search string based query to optionally filter-on
	SortBy string `query:"sort_by"` // SortBy sets the sort order of the result
}

type SearchPackageGroupRequest struct {
	URLs   []string `json:"urls,omitempty"`  // URLs of repositories to search
	UUIDs  []string `json:"uuids,omitempty"` // List of RepositoryConfig UUIDs to search
	Search string   `json:"search"`          // Search string to search package group names
	Limit  *int     `json:"limit,omitempty"` // Maximum number of records to return for the search
}

const SearchPackageGroupRequestLimitDefault int = 100
const SearchPackageGroupRequestLimitMaximum int = 500

type SearchPackageGroupResponse struct {
	PackageGroup string `json:"package_group_name"` // Package group found
	Description  string `json:"description"`        // Description of the package group found
}

// SetMetadata Map metadata to the collection.
// meta Metadata about the request.
// links Links to other pages of results.
func (r *RepositoryPackageGroupCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
