package copr_ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func expectedTask() queue.Task {
	return queue.Task{
		Typename:  config.CreatePartnerSnapshotTask,
		Payload:   expectedPayload(),
		OrgId:     config.Get().Clients.Pulp.CoprOrgID,
		RequestID: testEventID,
	}
}

func TestBuildEndHandler_EnqueueRetriesAndSucceeds(t *testing.T) {
	h, taskClient := newTestHandler(t)
	transientErr := errors.New("db unavailable")
	taskClient.EXPECT().Enqueue(expectedTask()).Return(uuid.Nil, transientErr).Times(maxEnqueueAttempts - 1)
	taskClient.EXPECT().Enqueue(expectedTask()).Return(uuid.New(), nil).Once()

	err := h.enqueue(context.Background(), testEventID, expectedPayload())
	assert.NoError(t, err)
}

func TestBuildEndHandler_EnqueueExhaustsRetries(t *testing.T) {
	h, taskClient := newTestHandler(t)
	enqueueErr := errors.New("db unavailable")
	taskClient.EXPECT().Enqueue(expectedTask()).Return(uuid.Nil, enqueueErr).Times(maxEnqueueAttempts)

	err := h.enqueue(context.Background(), testEventID, expectedPayload())
	assert.ErrorIs(t, err, enqueueErr)
	assert.ErrorContains(t, err, "enqueue snapshot task after 3 attempts")
}
