package api

import (
	"encoding/json"
	"time"

	"github.com/content-services/content-sources-backend/pkg/utils"
	"gorm.io/gorm"
)

type TemplateRequest struct {
	UUID            *string        `json:"uuid" readonly:"true" swaggerignore:"true"`
	Name            *string        `json:"name"`                                            // Name of the template
	Description     *string        `json:"description"`                                     // Description of the template
	RepositoryUUIDS []string       `json:"repository_uuids"`                                // Repositories to add to the template
	Arch            *string        `json:"arch"`                                            // Architecture of the template
	Version         *string        `json:"version"`                                         // Version of the template
	Date            *EmptiableDate `json:"date"`                                            // Latest date to include snapshots for
	OrgID           *string        `json:"org_id" readonly:"true" swaggerignore:"true"`     // Organization ID of the owner
	User            *string        `json:"created_by" readonly:"true" swaggerignore:"true"` // User creating the template
	UseLatest       *bool          `json:"use_latest"`                                      // Use latest snapshot for all repositories in the template
}

type TemplateResponse struct {
	UUID              string         `json:"uuid" readonly:"true"`
	Name              string         `json:"name"`                   // Name of the template
	OrgID             string         `json:"org_id"`                 // Organization ID of the owner
	Description       string         `json:"description"`            // Description of the template
	Arch              string         `json:"arch"`                   // Architecture of the template
	Version           string         `json:"version"`                // Version of the template
	Date              time.Time      `json:"date"`                   // Latest date to include snapshots for
	RepositoryUUIDS   []string       `json:"repository_uuids"`       // Repositories added to the template
	RHSMEnvironmentID string         `json:"rhsm_environment_id"`    // Environment ID used by subscription-manager and candlepin
	CreatedBy         string         `json:"created_by"`             // User that created the template
	LastUpdatedBy     string         `json:"last_updated_by"`        // User that most recently updated the template
	CreatedAt         time.Time      `json:"created_at"`             // Datetime template was created
	UpdatedAt         time.Time      `json:"updated_at"`             // Datetime template was last updated
	DeletedAt         gorm.DeletedAt `json:"-" swaggerignore:"true"` // Datetime template was deleted
	UseLatest         bool           `json:"use_latest"`             // Use latest snapshot for all repositories in the template
}

// We use a separate struct because version and arch cannot be updated
type TemplateUpdateRequest struct {
	UUID            *string        `json:"uuid" readonly:"true" swaggerignore:"true"`
	Name            *string        `json:"name"`                                                 // Name of the template
	Description     *string        `json:"description"`                                          // Description of the template
	RepositoryUUIDS []string       `json:"repository_uuids"`                                     // Repositories to add to the template
	Date            *EmptiableDate `json:"date"`                                                 // Latest date to include snapshots for
	OrgID           *string        `json:"org_id" readonly:"true" swaggerignore:"true"`          // Organization ID of the owner
	User            *string        `json:"last_updated_by" readonly:"true" swaggerignore:"true"` // User creating the template
	UseLatest       *bool          `json:"use_latest"`                                           // Use latest snapshot for all repositories in the template
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
	Name            string   `json:"name"`             // Filter templates by name using an exact match.
	Arch            string   `json:"arch"`             // Filter templates by arch using an exact match.
	Version         string   `json:"version"`          // Filter templates by version using an exact match.
	Search          string   `json:"search"`           // Search string based query to optionally filter on
	RepositoryUUIDs []string `json:"repository_uuids"` // List templates that contain one or more of these Repositories
	UseLatest       bool     `json:"use_latest"`       // List templates that have use_latest set to true
}

// Provides defaults if not provided during PUT request
func (r *TemplateUpdateRequest) FillDefaults() {
	emptyStr := ""
	if r.Description == nil {
		r.Description = &emptyStr
	}
	if r.Date == nil || r.Date.AsTime().Before(time.Time{}) {
		r.Date = (*EmptiableDate)(utils.Ptr(time.Now()))
		if r.IsUsingLatest() {
			r.Date = (*EmptiableDate)(utils.Ptr(time.Time{}))
		}
	}
	if r.RepositoryUUIDS == nil {
		r.RepositoryUUIDS = []string{}
	}
}

func (r *TemplateUpdateRequest) IsUsingLatest() bool {
	return r.UseLatest != nil && *r.UseLatest
}

type EmptiableDate time.Time

func (d EmptiableDate) AsTime() time.Time {
	return time.Time(d)
}

func (d EmptiableDate) IsZero() bool {
	return time.Time(d).IsZero()
}

func (d EmptiableDate) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(d).Format(time.RFC3339))
}

func (d *EmptiableDate) UnmarshalJSON(b []byte) error {
	str := string(b)
	if len(b) == 0 || str == "null" || str == `""` || str == "" {
		*d = EmptiableDate(time.Time{})
		return nil
	}

	var t time.Time
	if err := json.Unmarshal(b, &t); err != nil {
		return err
	}
	*d = EmptiableDate(t)
	return nil
}
