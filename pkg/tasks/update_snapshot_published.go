package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/helpers"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
)

type UpdateSnapshotPublishedPayload struct {
	SnapshotUUID string
	Published    bool
}

// UpdateSnapshotPublishedHandler updates the content guard on a snapshot's distribution
// to publish (no guard) or unpublish (restore org guard), and retargets the repository's
// /latest distribution to the effective latest snapshot (latest published).
func UpdateSnapshotPublishedHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
	opts := UpdateSnapshotPublishedPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for UpdateSnapshotPublishedPayload")
	}

	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	ctxWithLogger := logger.WithContext(ctx)

	daoReg := dao.GetDaoRegistry(db.DB)

	domainName, err := daoReg.Domain.Fetch(ctxWithLogger, task.OrgId)
	if err != nil {
		return fmt.Errorf("error fetching domain for org %s: %w", task.OrgId, err)
	}

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	t := UpdateSnapshotPublished{
		daoReg:     daoReg,
		ctx:        ctxWithLogger,
		orgID:      task.OrgId,
		payload:    &opts,
		pulpClient: pulpClient,
	}

	return t.Run()
}

type UpdateSnapshotPublished struct {
	daoReg     *dao.DaoRegistry
	ctx        context.Context
	orgID      string
	payload    *UpdateSnapshotPublishedPayload
	pulpClient pulp_client.PulpClient
}

func (t *UpdateSnapshotPublished) Run() error {
	snap, err := t.daoReg.Snapshot.FetchModel(t.ctx, t.payload.SnapshotUUID, false)
	if err != nil {
		return fmt.Errorf("error fetching snapshot %s: %w", t.payload.SnapshotUUID, err)
	}

	dist, err := t.pulpClient.FindDistributionByPath(t.ctx, snap.DistributionPath)
	if err != nil {
		return fmt.Errorf("error fetching distribution for snapshot %s: %w", t.payload.SnapshotUUID, err)
	}
	if dist == nil {
		return fmt.Errorf("distribution not found for snapshot %s at path %s", t.payload.SnapshotUUID, snap.DistributionPath)
	}

	var contentGuardHref *string
	if t.payload.Published {
		// Publishing: remove content guard (nil = no guard = publicly accessible)
		contentGuardHref = nil
	} else {
		// Unpublishing: restore org-scoped content guard
		pdh := helpers.NewPulpDistributionHelper(t.ctx, t.pulpClient)
		href, err := pdh.FetchContentGuard(t.orgID, "" /* featureName */)
		if err != nil {
			return fmt.Errorf("error fetching content guard for org %s: %w", t.orgID, err)
		}
		contentGuardHref = href
	}

	taskHref, err := t.pulpClient.UpdateRpmDistribution(
		t.ctx,
		dist.GetPulpHref(),
		snap.PublicationHref,
		dist.GetName(),
		dist.GetBasePath(),
		contentGuardHref,
	)
	if err != nil {
		return fmt.Errorf("error updating distribution content guard: %w", err)
	}
	if _, err := t.pulpClient.PollTask(t.ctx, taskHref); err != nil {
		return fmt.Errorf("error polling distribution update task: %w", err)
	}

	if err := t.updateLatestDistribution(snap.RepositoryConfigurationUUID); err != nil {
		return err
	}

	return nil
}

// updateLatestDistribution points the repository's /latest Pulp distribution at the
// effective latest snapshot's publication and syncs its content guard with the repo's
// public status (no guard when any published snapshots remain).
func (t *UpdateSnapshotPublished) updateLatestDistribution(repoConfigUUID string) error {
	latestPath := helpers.GetLatestRepoDistPath(repoConfigUUID)
	dist, err := t.pulpClient.FindDistributionByPath(t.ctx, latestPath)
	if err != nil {
		return fmt.Errorf("error fetching latest distribution for repo config %s: %w", repoConfigUUID, err)
	}
	if dist == nil {
		return nil
	}

	latestSnap, err := t.daoReg.Snapshot.FetchLatestSnapshotModel(t.ctx, repoConfigUUID)
	if err != nil {
		return fmt.Errorf("error fetching latest snapshot for repo config %s: %w", repoConfigUUID, err)
	}

	repoPublic, err := t.daoReg.Repository.FetchPublicStatus(t.ctx, repoConfigUUID)
	if err != nil {
		return err
	}

	var contentGuardHref *string
	if !repoPublic {
		// No published snapshots remain: restore org-scoped content guard.
		pdh := helpers.NewPulpDistributionHelper(t.ctx, t.pulpClient)
		href, err := pdh.FetchContentGuard(t.orgID, "" /* featureName */)
		if err != nil {
			return fmt.Errorf("error fetching content guard for org %s: %w", t.orgID, err)
		}
		contentGuardHref = href
	}

	taskHref, err := t.pulpClient.UpdateRpmDistribution(
		t.ctx,
		dist.GetPulpHref(),
		latestSnap.PublicationHref,
		dist.GetName(),
		dist.GetBasePath(),
		contentGuardHref,
	)
	if err != nil {
		return fmt.Errorf("error updating latest distribution: %w", err)
	}
	if _, err := t.pulpClient.PollTask(t.ctx, taskHref); err != nil {
		return fmt.Errorf("error polling latest distribution update task: %w", err)
	}

	return nil
}
