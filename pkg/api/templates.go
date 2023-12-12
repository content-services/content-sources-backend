package api

import "time"

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
	UUID        string
	Name        string    `json:"name"`        // Name of the template
	OrgID       string    `json:"org_id"`      // Organization ID of the owner
	Description string    `json:"description"` // Description of the template
	Arch        string    `json:"arch"`        // Architecture of the template
	Version     string    `json:"version"`     // Version of the template
	Date        time.Time `json:"date"`        // Latest date to include snapshots for
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
