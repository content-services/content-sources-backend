package api

import (
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/openlyinc/pointy"
)

// RepositoryResponse holds data returned by a repositories API response
type RepositoryResponse struct {
	UUID                         string            `json:"uuid" readonly:"true"`                // UUID of the object
	Name                         string            `json:"name"`                                // Name of the remote yum repository
	Label                        string            `json:"label"`                               // Label used to configure the yum repository on clients
	URL                          string            `json:"url"`                                 // URL of the remote yum repository
	Origin                       string            `json:"origin" `                             // Origin of the repository
	ContentType                  string            `json:"content_type" `                       // Content Type (rpm) of the repository
	DistributionVersions         []string          `json:"distribution_versions" example:"7,8"` // Versions to restrict client usage to
	DistributionArch             string            `json:"distribution_arch" example:"x86_64"`  // Architecture to restrict client usage to
	AccountID                    string            `json:"account_id" readonly:"true"`          // Account ID of the owner
	OrgID                        string            `json:"org_id" readonly:"true"`              // Organization ID of the owner
	LastIntrospectionTime        string            `json:"last_introspection_time"`             // Timestamp of last attempted introspection
	LastIntrospectionSuccessTime string            `json:"last_success_introspection_time"`     // Timestamp of last successful introspection
	LastIntrospectionUpdateTime  string            `json:"last_update_introspection_time"`      // Timestamp of last introspection that had updates
	LastIntrospectionError       string            `json:"last_introspection_error"`            // Error of last attempted introspection
	LastIntrospectionStatus      string            `json:"last_introspection_status"`           // Status of last introspection
	FailedIntrospectionsCount    int               `json:"failed_introspections_count"`         // Number of consecutive failed introspections
	PackageCount                 int               `json:"package_count"`                       // Number of packages last read in the repository
	Status                       string            `json:"status"`                              // Combined status of last introspection and snapshot of repository (Valid, Invalid, Unavailable, Pending)
	GpgKey                       string            `json:"gpg_key"`                             // GPG key for repository
	MetadataVerification         bool              `json:"metadata_verification"`               // Verify packages
	ModuleHotfixes               bool              `json:"module_hotfixes"`                     // Disable modularity filtering on this repository
	RepositoryUUID               string            `json:"-" swaggerignore:"true"`              // UUID of the dao.Repository
	Snapshot                     bool              `json:"snapshot"`                            // Enable snapshotting and hosting of this repository
	LastSnapshotUUID             string            `json:"last_snapshot_uuid,omitempty"`        // UUID of the last dao.Snapshot
	LastSnapshot                 *SnapshotResponse `json:"last_snapshot,omitempty"`             // Latest Snapshot taken
	LastSnapshotTaskUUID         string            `json:"last_snapshot_task_uuid,omitempty"`   // UUID of the last snapshot task
	LastSnapshotTask             *TaskInfoResponse `json:"last_snapshot_task,omitempty"`        // Last snapshot task response (contains last snapshot status)
}

// RepositoryRequest holds data received from request to create/update repository
type RepositoryRequest struct {
	UUID                 *string   `json:"uuid" readonly:"true" swaggerignore:"true"`
	Name                 *string   `json:"name"`                                              // Name of the remote yum repository
	URL                  *string   `json:"url"`                                               // URL of the remote yum repository
	DistributionVersions *[]string `json:"distribution_versions" example:"7,8"`               // Versions to restrict client usage to
	DistributionArch     *string   `json:"distribution_arch" example:"x86_64"`                // Architecture to restrict client usage to
	GpgKey               *string   `json:"gpg_key"`                                           // GPG key for repository
	MetadataVerification *bool     `json:"metadata_verification"`                             // Verify packages
	ModuleHotfixes       *bool     `json:"module_hotfixes"`                                   // Disable modularity filtering on this repository
	Snapshot             *bool     `json:"snapshot"`                                          // Enable snapshotting and hosting of this repository
	AccountID            *string   `json:"account_id" readonly:"true" swaggerignore:"true"`   // Account ID of the owner
	OrgID                *string   `json:"org_id" readonly:"true" swaggerignore:"true"`       // Organization ID of the owner
	Origin               *string   `json:"origin" readonly:"true" swaggerignore:"true"`       // Origin of the repository
	ContentType          *string   `json:"content_type" readonly:"true" swaggerignore:"true"` // Content Type (rpm) of the repository
}

func (r *RepositoryRequest) FillDefaults() {
	// Currently the user cannot change these
	r.Origin = pointy.Pointer(config.OriginExternal)
	r.ContentType = pointy.Pointer(config.ContentTypeRpm)
	// Fill in default values in case of PUT request, doesn't have to be valid, let the db validate that
	defaultName := ""
	defaultUrl := ""
	defaultVersions := []string{"any"}
	defaultArch := "any"
	defaultGpgKey := ""
	defaultMetadataVerification := false
	defaultModuleHotfixes := false

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
	if r.ModuleHotfixes == nil {
		r.ModuleHotfixes = &defaultModuleHotfixes
	}
}

type RepositoryIntrospectRequest struct {
	ResetCount bool `json:"reset_count"` // Reset the failed introspections count
}

type RepositoryCollectionResponse struct {
	Data  []RepositoryResponse `json:"data"`  // Requested Data
	Meta  ResponseMetadata     `json:"meta"`  // Metadata about the request
	Links Links                `json:"links"` // Links to other pages of results
}

func (r *RepositoryCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
