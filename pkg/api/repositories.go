package api

import (
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/utils"
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
	LatestSnapshotURL            string            `json:"latest_snapshot_url,omitempty"`       // Latest URL for the snapshot distribution
}

// RepositoryRequest holds data received from request to create repository
type RepositoryRequest struct {
	UUID        *string `json:"uuid" readonly:"true" swaggerignore:"true"`
	AccountID   *string `json:"account_id" readonly:"true" swaggerignore:"true"`   // Account ID of the owner
	OrgID       *string `json:"org_id" readonly:"true" swaggerignore:"true"`       // Organization ID of the owner
	Origin      *string `json:"origin" readonly:"true"`                            // Origin of the repository
	ContentType *string `json:"content_type" readonly:"true" swaggerignore:"true"` // Content Type (rpm) of the repository

	Name                 *string   `json:"name"`                                // Name of the remote yum repository
	URL                  *string   `json:"url"`                                 // URL of the remote yum repository
	DistributionVersions *[]string `json:"distribution_versions" example:"7,8"` // Versions to restrict client usage to
	DistributionArch     *string   `json:"distribution_arch" example:"x86_64"`  // Architecture to restrict client usage to
	GpgKey               *string   `json:"gpg_key"`                             // GPG key for repository
	MetadataVerification *bool     `json:"metadata_verification"`               // Verify packages
	ModuleHotfixes       *bool     `json:"module_hotfixes"`                     // Disable modularity filtering on this repository
	Snapshot             *bool     `json:"snapshot"`                            // Enable snapshotting and hosting of this repository
}

type RepositoryUpdateRequest struct {
	Name                 *string   `json:"name"`                                // Name of the remote yum repository
	URL                  *string   `json:"url"`                                 // URL of the remote yum repository
	DistributionVersions *[]string `json:"distribution_versions" example:"7,8"` // Versions to restrict client usage to
	DistributionArch     *string   `json:"distribution_arch" example:"x86_64"`  // Architecture to restrict client usage to
	GpgKey               *string   `json:"gpg_key"`                             // GPG key for repository
	MetadataVerification *bool     `json:"metadata_verification"`               // Verify packages
	ModuleHotfixes       *bool     `json:"module_hotfixes"`                     // Disable modularity filtering on this repository
	Snapshot             *bool     `json:"snapshot"`                            // Enable snapshotting and hosting of this repository
}

func (r *RepositoryRequest) ToRepositoryUpdateRequest() RepositoryUpdateRequest {
	return RepositoryUpdateRequest{
		Name:                 r.Name,
		URL:                  r.URL,
		DistributionVersions: r.DistributionVersions,
		DistributionArch:     r.DistributionArch,
		GpgKey:               r.GpgKey,
		MetadataVerification: r.MetadataVerification,
		ModuleHotfixes:       r.ModuleHotfixes,
		Snapshot:             r.Snapshot,
	}
}

var defaultRepoValues = RepositoryUpdateRequest{
	Name:                 utils.Ptr(""),
	URL:                  utils.Ptr(""),
	DistributionVersions: utils.Ptr([]string{"any"}),
	DistributionArch:     utils.Ptr("any"),
	GpgKey:               utils.Ptr(""),
	MetadataVerification: utils.Ptr(false),
	ModuleHotfixes:       utils.Ptr(false),
	Snapshot:             utils.Ptr(false),
}

func (r *RepositoryUpdateRequest) FillDefaults() {
	if r.Name == nil {
		r.Name = defaultRepoValues.Name
	}
	if r.URL == nil {
		r.URL = defaultRepoValues.URL
	}
	if r.DistributionVersions == nil {
		r.DistributionVersions = defaultRepoValues.DistributionVersions
	}
	if r.DistributionArch == nil {
		r.DistributionArch = defaultRepoValues.DistributionArch
	}
	if r.GpgKey == nil {
		r.GpgKey = defaultRepoValues.GpgKey
	}
	if r.MetadataVerification == nil {
		r.MetadataVerification = defaultRepoValues.MetadataVerification
	}
	if r.ModuleHotfixes == nil {
		r.ModuleHotfixes = defaultRepoValues.ModuleHotfixes
	}
}

func (r *RepositoryRequest) FillDefaults() {
	// Currently the user cannot change these, only set at creation
	r.ContentType = utils.Ptr(config.ContentTypeRpm)

	if r.Origin == nil {
		r.Origin = utils.Ptr(config.OriginExternal)
	}

	// copied from RepositoryUpdateRequest FillDefaults
	if r.Name == nil {
		r.Name = defaultRepoValues.Name
	}
	if r.URL == nil {
		r.URL = defaultRepoValues.URL
	}
	if r.DistributionVersions == nil {
		r.DistributionVersions = defaultRepoValues.DistributionVersions
	}
	if r.DistributionArch == nil {
		r.DistributionArch = defaultRepoValues.DistributionArch
	}
	if r.GpgKey == nil {
		r.GpgKey = defaultRepoValues.GpgKey
	}
	if r.MetadataVerification == nil {
		r.MetadataVerification = defaultRepoValues.MetadataVerification
	}
	if r.ModuleHotfixes == nil {
		r.ModuleHotfixes = defaultRepoValues.ModuleHotfixes
	}
}

type RepositoryIntrospectRequest struct {
	ResetCount bool `json:"reset_count"` // Reset the failed introspections count
}

type RepositoryExportRequest struct {
	RepositoryUuids []string `json:"repository_uuids"` // List of repository uuids to export
}

type RepositoryExportResponse struct {
	Name                 string   `json:"name"`                               // Name of the remote yum repository
	URL                  string   `json:"url"`                                // URL of the remote yum repository
	Origin               string   `json:"origin"`                             // Origin of the repository
	DistributionVersions []string `json:"distribution_versions" example:"8"`  // Versions to restrict client usage to
	DistributionArch     string   `json:"distribution_arch" example:"x86_64"` // Architecture to restrict client usage to
	GpgKey               string   `json:"gpg_key"`                            // GPG key for repository
	MetadataVerification bool     `json:"metadata_verification"`              // Verify packages
	ModuleHotfixes       bool     `json:"module_hotfixes"`                    // Disable modularity filtering on this repository
	Snapshot             bool     `json:"snapshot"`                           // Enable snapshotting and hosting of this repository
}

type RepositoryImportResponse struct {
	RepositoryResponse
	Warnings []map[string]interface{} `json:"warnings"` // Warnings to alert user of mismatched fields if there is an existing repo with the same URL
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

func (r *RepositoryResponse) Introspectable() bool {
	return r.Origin != config.OriginUpload
}
