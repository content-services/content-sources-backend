package api

type RepositoryRpm struct {
	UUID     string `json:"uuid"`     // Identifier of the rpm
	Name     string `json:"name"`     // The rpm package name
	Arch     string `json:"arch"`     // The Architecture of the rpm
	Version  string `json:"version"`  // The version of the  rpm
	Release  string `json:"release"`  // The release of the rpm
	Epoch    int32  `json:"epoch"`    // The epoch of the rpm
	Summary  string `json:"summary"`  // The summary of the rpm
	Checksum string `json:"checksum"` // The checksum of the rpm
}

type RepositoryRpmCollectionResponse struct {
	Data  []RepositoryRpm  `json:"data"`  // List of rpms
	Meta  ResponseMetadata `json:"meta"`  // Metadata about the request
	Links Links            `json:"links"` // Links to other pages of results
}

type SearchRpmResponse struct {
	PackageName string `json:"package_name"` // Package name found
	Summary     string `json:"summary"`      // Summary of the package found
}

// SetMetadata Map metadata to the collection.
// meta Metadata about the request.
// links Links to other pages of results.
func (r *RepositoryRpmCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
