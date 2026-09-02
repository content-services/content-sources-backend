package api

// LightwellPackageResponse represents a package found across Lightwell repositories.
type LightwellPackageResponse struct {
	Name           string        `json:"name"`
	Group          string        `json:"group,omitempty"`
	ContentType    string        `json:"content_type"`
	Repository     string        `json:"repository"`
	RepositoryUUID string        `json:"repository_uuid"`
	Versions       []string      `json:"versions"`
	LatestReleases []ReleaseInfo `json:"latest_releases"`
}

// LightwellPackageCollectionResponse is a paginated collection of cross-repo packages.
type LightwellPackageCollectionResponse struct {
	Data  []LightwellPackageResponse `json:"data"`
	Meta  ResponseMetadata           `json:"meta"`
	Links Links                      `json:"links"`
}

func (r *LightwellPackageCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}

// LightwellPackageVersionResponse represents a single package version across Lightwell repositories.
type LightwellPackageVersionResponse struct {
	Name           string `json:"name"`
	Group          string `json:"group,omitempty"`
	Version        string `json:"version"`
	ContentType    string `json:"content_type"`
	Repository     string `json:"repository"`
	RepositoryUUID string `json:"repository_uuid"`
	Release        string `json:"release,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	Purl           string `json:"purl"`
	Coordinates    string `json:"coordinates"`
}

// LightwellPackageVersionCollectionResponse is a paginated collection of cross-repo package versions.
type LightwellPackageVersionCollectionResponse struct {
	Data  []LightwellPackageVersionResponse `json:"data"`
	Meta  ResponseMetadata                  `json:"meta"`
	Links Links                             `json:"links"`
}

func (r *LightwellPackageVersionCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}

// LightwellPackageFilterData holds query-parameter filters for the cross-repo packages endpoint.
type LightwellPackageFilterData struct {
	ContentType   string `query:"content_type"`
	Name          string `query:"name"`
	Repository    string `query:"repository"`
	SecurityLevel string `query:"security_level"`
}

// LightwellPackageVersionFilterData holds query-parameter filters for the cross-repo package_versions endpoint.
type LightwellPackageVersionFilterData struct {
	ContentType       string `query:"content_type"`
	Name              string `query:"name"`
	SecurityLevel     string `query:"security_level"`
	Repository        string `query:"repository"`
	ResolvesCveID     string `query:"resolves_cve_id"`
	VulnerableToCveID string `query:"vulnerable_to_cve_id"`
}
