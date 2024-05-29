package api

import (
	"time"

	"github.com/openlyinc/pointy"
)

type TemplateRequest struct {
	UUID            *string    `json:"uuid" readonly:"true" swaggerignore:"true"`
	Name            *string    `json:"name"`                                        // Name of the template
	Description     *string    `json:"description"`                                 // Description of the template
	RepositoryUUIDS []string   `json:"repository_uuids"`                            // Repositories to add to the template
	Arch            *string    `json:"arch"`                                        // Architecture of the template
	Version         *string    `json:"version"`                                     // Version of the template
	Date            *time.Time `json:"date"`                                        // Latest date to include snapshots for
	OrgID           *string    `json:"org_id" readonly:"true" swaggerignore:"true"` // Organization ID of the owner
}

type TemplateResponse struct {
	UUID              string    `json:"uuid" readonly:"true"`
	Name              string    `json:"name"`                // Name of the template
	OrgID             string    `json:"org_id"`              // Organization ID of the owner
	Description       string    `json:"description"`         // Description of the template
	Arch              string    `json:"arch"`                // Architecture of the template
	Version           string    `json:"version"`             // Version of the template
	Date              time.Time `json:"date"`                // Latest date to include snapshots for
	RepositoryUUIDS   []string  `json:"repository_uuids"`    // Repositories added to the template
	RHSMEnvironmentId string    `json:"rhsm_environment_id"` // Environment ID used by subscription-manager & candlepin
}

// We use a separate struct because name, version, arch cannot be updated
type TemplateUpdateRequest struct {
	UUID            *string    `json:"uuid" readonly:"true" swaggerignore:"true"`
	Description     *string    `json:"description"`                                 // Description of the template
	RepositoryUUIDS []string   `json:"repository_uuids"`                            // Repositories to add to the template
	Date            *time.Time `json:"date"`                                        // Latest date to include snapshots for
	OrgID           *string    `json:"org_id" readonly:"true" swaggerignore:"true"` // Organization ID of the owner
}

type TemplateCollectionResponse struct {
	Data  []TemplateResponse `json:"data"`  // Requested Data
	Meta  ResponseMetadata   `json:"meta"`  // Metadata about the request
	Links Links              `json:"links"` // Links to other pages of results
}

func (r *TemplateCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}

type TemplateFilterData struct {
	Name    string `json:"name"`    // Filter templates by name using an exact match.
	Arch    string `json:"arch"`    // Filter templates by arch using an exact match.
	Version string `json:"version"` // Filter templates by version using an exact match.
	Search  string `json:"search"`  // Search string based query to optionally filter on
}

// Provides defaults if not provided during PUT request
func (r *TemplateUpdateRequest) FillDefaults() {
	emptyStr := ""
	if r.Description == nil {
		r.Description = &emptyStr
	}
	if r.Date == nil {
		r.Date = pointy.Pointer(time.Now())
	}
	if r.RepositoryUUIDS == nil {
		r.RepositoryUUIDS = []string{}
	}
}
