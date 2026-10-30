package dao

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// PublishTemplateEvent sends a template CloudEvent.
// template-created and template-updated include advisory_ids loaded by FetchTemplateErrataIDs.
// A lookup error is logged and the event is still sent without advisory_ids.
// template-deleted never includes advisory_ids.
// No event is sent when the template event client is not configured.
func PublishTemplateEvent(ctx context.Context, db *gorm.DB, orgID string, eventName event.EventName, template api.TemplateResponse) {
	if config.Get().TemplateEventClient == nil {
		return
	}

	mapped := event.MapTemplateResponse(template)
	if eventName == event.TemplateCreated || eventName == event.TemplateUpdated {
		ids, err := GetRpmDao(db, nil).FetchTemplateErrataIDs(ctx, orgID, template.UUID)
		if err != nil {
			log.Error().Err(err).
				Str("template_uuid", template.UUID).
				Str("org_id", orgID).
				Str("event", eventName.String()).
				Msg("failed to fetch template advisory ids; sending event without advisory_ids")
		} else {
			mapped.AdvisoryIDs = ids
		}
	}
	event.SendTemplateEvent(orgID, eventName, []event.TemplateEvent{mapped})
}
