package jobs

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/rs/zerolog/log"
)

type templateIdentity struct {
	UUID  string
	OrgID string
}

func SendTemplateUpdateEvents(_ []string) {
	ctx := context.Background()
	daoReg := dao.GetDaoRegistry(db.DB)

	config.SetupTemplateEvents()

	var templates []templateIdentity
	err := db.DB.Model(&models.Template{}).
		Where("deleted_at IS NULL").
		Order("uuid").
		Select("uuid", "org_id").
		Find(&templates).Error
	if err != nil {
		log.Fatal().Err(err).Msg("failed to fetch templates")
	}

	log.Info().Int("template_count", len(templates)).Msg("Found templates to process")

	eventsSent := 0
	processed := 0

	for _, template := range templates {
		processed++

		templateResp, err := daoReg.Template.Fetch(ctx, template.OrgID, template.UUID, false)
		if err != nil {
			log.Error().
				Err(err).
				Str("template_uuid", template.UUID).
				Str("org_id", template.OrgID).
				Msg("failed to fetch template")
			continue
		}

		event.SendTemplateEvent(template.OrgID, event.TemplateUpdated, []event.TemplateEvent{event.MapTemplateResponse(templateResp)})
		eventsSent++

		log.Info().
			Str("template_uuid", template.UUID).
			Str("org_id", template.OrgID).
			Int("processed", processed).
			Int("total", len(templates)).
			Msg("Sent template-updated event")
	}

	log.Info().
		Int("events_sent", eventsSent).
		Int("total_templates", len(templates)).
		Msg("Finished sending template-updated events")
}
