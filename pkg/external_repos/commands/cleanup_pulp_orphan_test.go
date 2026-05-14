package commands

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// When poll fails, further batches must not run. batchSize 3 runs three goroutines in parallel per batch.
func TestRunPulpOrphanCleanupForDomains_StopsOnFirstPollFailure(t *testing.T) {
	prev := zlog.Logger
	zlog.Logger = zerolog.Nop()
	t.Cleanup(func() { zlog.Logger = prev })

	ctx := context.Background()
	domains := []models.Domain{
		{OrgId: "1", DomainName: "d1"},
		{OrgId: "2", DomainName: "d2"},
		{OrgId: "3", DomainName: "d3"},
		{OrgId: "4", DomainName: "d4"},
		{OrgId: "5", DomainName: "d5"},
		{OrgId: "6", DomainName: "d6"},
		{OrgId: "7", DomainName: "d7"},
		{OrgId: "8", DomainName: "d8"},
		{OrgId: "9", DomainName: "d9"},
	}
	const batchSize = 3
	taskHref := "https://pulp.example/api/v3/tasks/fake-href/"

	var orphanStarts atomic.Int32
	mockPulp := pulp_client.NewMockPulpClient(t)
	mockPulp.On("OrphanCleanup", mock.Anything).Return(taskHref, nil).Run(func(mock.Arguments) {
		orphanStarts.Add(1)
	})
	mockPulp.On("PollTask", mock.Anything, taskHref).Return(nil, errors.New("injected poll failure"))

	getPulpClient := func(_ string) pulp_client.PulpClient {
		return mockPulp
	}

	err := runPulpOrphanCleanupForDomains(ctx, domains, batchSize, getPulpClient)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pulp orphan cleanup aborted")
	assert.Contains(t, err.Error(), "failed task poll")

	starts := orphanStarts.Load()
	assert.GreaterOrEqual(t, starts, int32(1), "at least one domain in the first batch should start orphan cleanup")
	assert.LessOrEqual(t, starts, int32(batchSize), "only the first batch should run before abort (domains 4–9 must not be processed)")
}
