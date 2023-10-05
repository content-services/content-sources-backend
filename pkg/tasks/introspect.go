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
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func IntrospectHandler(ctx context.Context, task *models.TaskInfo, q *queue.Queue) error {
	var p payloads.IntrospectPayload

	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)

	if err := json.Unmarshal(task.Payload, &p); err != nil {
		return fmt.Errorf("payload incorrect type for IntrospectHandler")
	}
	// https://github.com/go-playground/validator
	// FIXME Wrong usage of validator library
	validate := validator.New()
	if err := validate.Var(p.Url, "required"); err != nil {
		return err
	}
	newRpms, nonFatalErrs, errs := external_repos.IntrospectUrl(logger.WithContext(context.Background()), p.Url, p.Force)
	for i := 0; i < len(nonFatalErrs); i++ {
		logger.Warn().Err(nonFatalErrs[i]).Msgf("Error %v introspecting repository %v", i, p.Url)
	}

	// Introspection failure isn't considered a message failure, as the message has been handled
	for i := 0; i < len(errs); i++ {
		logger.Error().Err(errs[i]).Msgf("Error %v introspecting repository %v", i, p.Url)
	}
	logger.Debug().Msgf("IntrospectionUrl returned %d new packages", newRpms)

	select {
	case <-ctx.Done():
		return queue.ErrTaskCanceled
	default:
		return nil
	}
}

func LogForTask(taskID, typename, requestID string) *zerolog.Logger {
	logger := log.Logger.With().
		Str("task_type", typename).
		Str("task_id", taskID).
		Str("request_id", requestID).
		Logger()
	return &logger
}
