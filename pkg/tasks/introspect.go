package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func IntrospectHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
	var p payloads.IntrospectPayload

	if err := json.Unmarshal(task.Payload, &p); err != nil {
		return fmt.Errorf("payload incorrect type for IntrospectHandler: %w", err)
	}
	intro := IntrospectionTask{
		URL:    p.Url,
		daoReg: dao.GetDaoRegistry(db.DB),
		ctx:    ctx,
		logger: LogForTask(task.Id.String(), task.Typename, task.RequestID),
	}
	return intro.Run()
}

type IntrospectionTask struct {
	URL    string
	daoReg *dao.DaoRegistry
	ctx    context.Context
	logger *zerolog.Logger
}

func (i *IntrospectionTask) Run() error {
	logger := i.logger
	repo, err := i.daoReg.Repository.FetchForUrl(i.ctx, i.URL)
	if err != nil {
		return fmt.Errorf("error loading repository during introspection %w", err)
	}
	newRpms, nonFatalErr, err := external_repos.IntrospectUrl(i.logger.WithContext(i.ctx), i.URL)
	if err != nil && !IsTaskCancelled(i.ctx) {
		logger.Error().Err(err).Msgf("Fatal error introspecting repository %v", i.URL)
		return err
	}
	if nonFatalErr != nil && !IsTaskCancelled(i.ctx) {
		msg := fmt.Sprintf("Error introspecting repository %v", i.URL)
		if repo.Public {
			logger.Error().Err(nonFatalErr).Msg(msg)
		} else {
			logger.Info().Err(nonFatalErr).Msg(msg)
		}
		return nonFatalErr
	}

	logger.Debug().Msgf("IntrospectionUrl returned %d new packages", newRpms)
	return nil
}

func LogForTask(taskID, typename, requestID string) *zerolog.Logger {
	logger := log.Logger.With().
		Str("task_type", typename).
		Str("task_id", taskID).
		Str(config.RequestIdLoggingKey, requestID).
		Logger()
	return &logger
}

// IsTaskCancelled returns true if context is cancelled for expected reason
func IsTaskCancelled(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), queue.ErrTaskCanceled) || errors.Is(context.Cause(ctx), ce.ErrServerExited)
}
