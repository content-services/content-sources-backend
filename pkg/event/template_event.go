package event

import (
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
)

type TemplateEvent struct {
	UUID              string    `json:"uuid" readonly:"true"`
	Name              string    `json:"name"`                // Name of the template
	OrgID             string    `json:"org_id"`              // Organization ID of the owner
	Description       *string   `json:"description"`         // Description of the template
	Arch              string    `json:"arch"`                // Architecture of the template
	Version           string    `json:"version"`             // Version of the template
	Date              time.Time `json:"date"`                // Latest date to include snapshots for
	RepositoryUUIDS   []string  `json:"repository_uuids"`    // Repositories added to the template
	RHSMEnvironmentID string    `json:"rhsm_environment_id"` // Environment ID used by subscription-manager & candlepin
}

func MapTemplateResponse(t api.TemplateResponse) TemplateEvent {
	return TemplateEvent{
		UUID:              t.UUID,
		Name:              t.Name,
		OrgID:             t.OrgID,
		Description:       &t.Description,
		Arch:              t.Arch,
		Version:           t.Version,
		Date:              t.Date,
		RepositoryUUIDS:   t.RepositoryUUIDS,
		RHSMEnvironmentID: t.RHSMEnvironmentID,
	}
}
