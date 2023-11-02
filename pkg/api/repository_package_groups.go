package api

import "github.com/lib/pq"

type RepositoryPackageGroup struct {
	UUID        string   `json:"uuid"`        // Identifier of the package group
	ID          string   `json:"id"`          // The package group ID
	Name        string   `json:"name"`        // The package group name
	Description string   `json:"description"` // The package group description
	PackageList []string `json:"packagelist"` // The list of packages in the package group
}

type RepositoryPackageGroupCollectionResponse struct {
	Data  []RepositoryPackageGroup `json:"data"`  // List of package groups
	Meta  ResponseMetadata         `json:"meta"`  // Metadata about the request
	Links Links                    `json:"links"` // Links to other pages of results
}

type SearchPackageGroupResponse struct {
	PackageGroupName string         `json:"package_group_name"`            // Package group found
	Description      string         `json:"description"`                   // Description of the package group found
	PackageList      pq.StringArray `json:"package_list" gorm:"type:text"` // Package list of the package group found
}

// SetMetadata Map metadata to the collection.
// meta Metadata about the request.
// links Links to other pages of results.
func (r *RepositoryPackageGroupCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
