package api

type RepositoryRpm struct {
	// RPM id
	UUID string `json:"uuid"`
	// The rpm package name
	Name string `json:"name"`
	// The architecture that this package belong to
	Arch string `json:"arch"`
	// The version for this package
	Version string `json:"version"`
	// The release for this package
	Release string `json:"release"`
	// Epoch is a way to define weighted dependencies based
	// on version numbers. It's default value is 0 and this
	// is assumed if an Epoch directive is not listed in the RPM SPEC file.
	// https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html/packaging_and_distributing_software/advanced-topics#packaging-epoch_epoch-scriplets-and-triggers
	Epoch    int32  `json:"epoch"`
	Summary  string `json:"summary"`
	Checksum string `json:"checksum"`
}

type RepositoryRpmCollectionResponse struct {
	// Requested Data
	Data []RepositoryRpm `json:"data"`
	// Metadata about the request
	Meta ResponseMetadata `json:"meta"`
	// Links to other pages of results
	Links Links `json:"links"`
}

type SearchRpmRequest struct {
	URLs  []string `json:"urls"`
	Query string   `json:"query"`
}

type SearchRpmResponse struct {
	// List of suggested package names
	PackageNames []string `json:"package_names"`
}

// SetMetadata Map metadata to the collection.
// meta Metadata about the request.
// links Links to other pages of results.
func (r *RepositoryRpmCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
