package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/external_repos"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog/log"
)

// TODO possibly remove context arg
func IntrospectHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
	var p payloads.IntrospectPayload
	if err := json.Unmarshal(task.Payload, &p); err != nil {
		return fmt.Errorf("payload incorrect type for IntrospectHandler")
	}
	// https://github.com/go-playground/validator
	// FIXME Wrong usage of validator library
	validate := validator.New()
	if err := validate.Var(p.Url, "required"); err != nil {
		return err
	}

	newRpms, nonFatalErrs, errs := external_repos.IntrospectUrl(p.Url, p.Force)
	for i := 0; i < len(nonFatalErrs); i++ {
		log.Warn().Err(nonFatalErrs[i]).Msgf("Error %v introspecting repository %v", i, p.Url)
	}

	// Introspection failure isn't considered a message failure, as the message has been handled
	for i := 0; i < len(errs); i++ {
		log.Error().Err(errs[i]).Msgf("Error %v introspecting repository %v", i, p.Url)
	}
	log.Debug().Msgf("IntrospectionUrl returned %d new packages", newRpms)
	return nil
}
