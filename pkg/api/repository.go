package api

// RepositoryResponse holds data returned by a repositories API response
type RepositoryResponse struct {
	UUID                         string   `json:"uuid" readonly:"true"`                // UUID of the object
	Name                         string   `json:"name"`                                // Name of the remote yum repository
	URL                          string   `json:"url"`                                 // URL of the remote yum repository
	DistributionVersions         []string `json:"distribution_versions" example:"7,8"` // Versions to restrict client usage to
	DistributionArch             string   `json:"distribution_arch" example:"x86_64"`  // Architecture to restrict client usage to
	AccountID                    string   `json:"account_id" readonly:"true"`          // Account ID of the owner
	OrgID                        string   `json:"org_id" readonly:"true"`              // Organization ID of the owner
	LastIntrospectionTime        string   `json:"last_introspection_time"`             // Timestamp of last attempted introspection
	LastIntrospectionSuccessTime string   `json:"last_success_introspection_time"`     // Timestamp of last successful introspection
	LastIntrospectionUpdateTime  string   `json:"last_update_introspection_time"`      // Timestamp of last introspection that had updates
	LastIntrospectionError       string   `json:"last_introspection_error"`            // Error of last attempted introspection
	Status                       string   `json:"status"`                              // Status of repository introspection (Valid, Invalid, Unavailable, Pending)
	GpgKey                       string   `json:"gpg_key"`                             // GPG key for repository
	MetadataVerification         bool     `json:"metadata_verification"`               // Verify packages
}

// RepositoryRequest holds data received from request to create/update repository
type RepositoryRequest struct {
	UUID                 *string   `json:"uuid" readonly:"true" swaggerignore:"true"`
	Name                 *string   `json:"name"`                                            // Name of the remote yum repository
	URL                  *string   `json:"url"`                                             // URL of the remote yum repository
	DistributionVersions *[]string `json:"distribution_versions" example:"7,8"`             // Versions to restrict client usage to
	DistributionArch     *string   `json:"distribution_arch" example:"x86_64"`              // Architecture to restrict client usage to
	GpgKey               *string   `json:"gpg_key"`                                         // GPG key for repository
	MetadataVerification *bool     `json:"metadata_verification"`                           // Verify packages
	AccountID            *string   `json:"account_id" readonly:"true" swaggerignore:"true"` // Account ID of the owner
	OrgID                *string   `json:"org_id" readonly:"true" swaggerignore:"true"`     // Organization ID of the owner
}

type RepositoryBulkCreateResponse struct {
	ErrorMsg   string              `json:"error"`      // Error during creation
	Repository *RepositoryResponse `json:"repository"` // Repository object information
}

func (r *RepositoryRequest) FillDefaults() {
	// Fill in default values in case of PUT request, doesn't have to be valid, let the db validate that
	defaultName := ""
	defaultUrl := ""
	defaultVersions := []string{"any"}
	defaultArch := "any"
	defaultGpgKey := ""
	defaultMetadataVerification := false
	if r.Name == nil {
		r.Name = &defaultName
	}
	if r.URL == nil {
		r.URL = &defaultUrl
	}
	if r.DistributionVersions == nil {
		r.DistributionVersions = &defaultVersions
	}
	if r.DistributionArch == nil {
		r.DistributionArch = &defaultArch
	}
	if r.GpgKey == nil {
		r.GpgKey = &defaultGpgKey
	}
	if r.MetadataVerification == nil {
		r.MetadataVerification = &defaultMetadataVerification
	}
}

type RepositoryCollectionResponse struct {
	Data  []RepositoryResponse `json:"data"`  //Requested Data
	Meta  ResponseMetadata     `json:"meta"`  //Metadata about the request
	Links Links                `json:"links"` //Links to other pages of results
}

func (r *RepositoryCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
