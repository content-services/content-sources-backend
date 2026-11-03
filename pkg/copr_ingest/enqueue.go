package copr_ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/rs/zerolog"
)

// Enqueue retry limits. Failed enqueues imply DB unavailability; dropped builds are covered by later cumulative Copr snapshots.
const maxEnqueueAttempts = 3
const enqueueRetryBackoff = 2 * time.Second // multiplied by the attempt number

// enqueue enqueues the partner snapshot task for a successful build, retrying with a linear backoff.
func (h *BuildEndHandler) enqueue(ctx context.Context, eventID string, payload payloads.CreatePartnerSnapshotPayload) error {
	task := queue.Task{
		Typename:  config.CreatePartnerSnapshotTask,
		Payload:   payload,
		OrgId:     config.CoprOrg,
		RequestID: eventID,
	}
	var err error
	for attempt := 1; attempt <= maxEnqueueAttempts; attempt++ {
		if _, err = h.taskClient.Enqueue(task); err == nil {
			return nil
		}
		if attempt == maxEnqueueAttempts {
			break
		}
		zerolog.Ctx(ctx).Warn().Err(err).Int("attempt", attempt).Msg("snapshot task enqueue failed, retrying")
		if sleepErr := utils.SleepWithCancel(ctx, time.Duration(attempt)*h.enqueueRetryBackoff); sleepErr != nil {
			return fmt.Errorf("enqueue snapshot task: %w", sleepErr)
		}
	}
	return fmt.Errorf("enqueue snapshot task after %d attempts: %w", maxEnqueueAttempts, err)
}
