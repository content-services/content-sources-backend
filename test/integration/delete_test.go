package integration

import (
	"context"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	uuid2 "github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// This is a delete integration tests without any snapshotting
type DeleteTest struct {
	Suite
	dao        *dao.DaoRegistry
	pulpClient pulp_client.PulpClient
	ctx        context.Context
}

func (s *DeleteTest) SetupTest() {
	s.Suite.SetupTest()
	s.ctx = context.Background()
	s.dao = dao.GetDaoRegistry(db.DB)
	// Force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"
}

func TestDeleteTest(t *testing.T) {
	suite.Run(t, new(DeleteTest))
}

func (s *DeleteTest) TestDeleteRepositorySnapshots() {
	// Setup the repository
	accountId := uuid2.NewString()
	repo, err := s.dao.RepositoryConfig.Create(s.ctx, api.RepositoryRequest{
		Name:      utils.Ptr(uuid2.NewString()),
		URL:       utils.Ptr("https://fixtures.pulpproject.org/rpm-unsigned/"),
		AccountID: utils.Ptr(accountId),
		OrgID:     utils.Ptr(accountId),
		Snapshot:  utils.Ptr(false),
	})
	assert.NoError(s.T(), err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)

	// Start the task
	taskClient := client.NewTaskClient(&s.queue)

	// Delete the repository
	taskUuid, err := taskClient.Enqueue(queue.Task{
		Typename:   config.DeleteRepositorySnapshotsTask,
		Payload:    tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repo.UUID},
		OrgId:      repo.OrgID,
		ObjectUUID: utils.Ptr(repoUuid.String()),
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
	})
	assert.NoError(s.T(), err)
	s.WaitOnTask(taskUuid)

	results, _, err := s.dao.RepositoryConfig.List(s.ctx, accountId, api.PaginationData{}, api.FilterData{
		Name: repo.Name,
	})
	assert.NoError(s.T(), err)
	assert.Empty(s.T(), results.Data)
}

func (s *DeleteTest) TestDeleteSnapshot() {
	t := s.T()
	config.Get().Features.Snapshots.Enabled = true

	orgID := uuid2.NewString()
	taskClient := client.NewTaskClient(&s.queue)
	domain, err := s.dao.Domain.FetchOrCreateDomain(s.ctx, orgID)
	assert.NoError(s.T(), err)
	s.pulpClient = pulp_client.GetPulpClientWithDomain(domain)
	assert.NotNil(t, s.pulpClient)

	// Create a repo and two snapshots for it
	repo := s.createAndSyncRepository(orgID, "https://fedorapeople.org/groups/katello/fakerepos/zoo/")
	_, err = s.dao.RepositoryConfig.Update(s.ctx, orgID, repo.UUID, api.RepositoryUpdateRequest{
		URL: utils.Ptr("https://rverdile.fedorapeople.org/dummy-repos/comps/repo1/"),
	})
	assert.NoError(t, err)
	repo, err = s.dao.RepositoryConfig.Fetch(s.ctx, orgID, repo.UUID)
	assert.NoError(t, err)
	s.snapshotAndWait(taskClient, repo, dao.UuidifyString(repo.RepositoryUUID), orgID)
	repoSnaps, _, err := s.dao.Snapshot.List(s.ctx, orgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(t, err)
	assert.Len(t, repoSnaps.Data, 2)

	// Soft delete one of the snapshots
	snap, err := s.dao.Snapshot.FetchUnscoped(s.ctx, repoSnaps.Data[0].UUID)
	assert.NoError(t, err)
	otherSnapUUID := repoSnaps.Data[1].UUID
	err = s.dao.Snapshot.SoftDelete(s.ctx, snap.UUID)
	assert.NoError(t, err)

	// Start and wait for the delete snapshot task
	requestID := uuid2.NewString()
	taskUUID, err := taskClient.Enqueue(queue.Task{
		Typename: config.DeleteSnapshotsTask,
		Payload: utils.Ptr(payloads.DeleteSnapshotsPayload{
			RepoUUID:       repo.UUID,
			SnapshotsUUIDs: []string{snap.UUID},
		}),
		OrgId:      orgID,
		AccountId:  orgID,
		ObjectUUID: utils.Ptr(repo.UUID),
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		RequestID:  requestID,
	})
	assert.NoError(t, err)
	s.WaitOnTask(taskUUID)

	// Verify the snapshot is deleted
	repoSnaps, _, err = s.dao.Snapshot.List(s.ctx, orgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(t, err)
	assert.Len(t, repoSnaps.Data, 1)
	assert.Equal(t, repoSnaps.Data[0].UUID, otherSnapUUID)
	deletedSnap, err := s.dao.Snapshot.FetchUnscoped(s.ctx, snap.UUID)
	assert.Error(t, err)
	assert.Equal(t, models.Snapshot{}, deletedSnap)

	// Verify correct pulp deletion/cleanup
	resp1, err := s.pulpClient.FindDistributionByPath(s.ctx, snap.DistributionPath)
	assert.Nil(t, resp1)
	_, err = s.pulpClient.FindRpmPublicationByVersion(s.ctx, snap.VersionHref)
	assert.Error(t, err)
	_, err = s.pulpClient.GetRpmRepositoryVersion(s.ctx, snap.VersionHref)
	assert.Error(t, err)
}

func (s *DeleteTest) WaitOnTask(taskUuid uuid2.UUID) {
	// Poll until the task is complete
	taskInfo, err := s.queue.Status(taskUuid)
	assert.NoError(s.T(), err)
	for {
		if taskInfo.Status == config.TaskStatusRunning || taskInfo.Status == config.TaskStatusPending {
			log.Logger.Error().Msg("SLEEPING")
			time.Sleep(1 * time.Second)
		} else {
			break
		}
		taskInfo, err = s.queue.Status(taskUuid)
		assert.NoError(s.T(), err)
	}
	if taskInfo.Error != nil {
		assert.Nil(s.T(), *taskInfo.Error)
	}

	assert.Equal(s.T(), config.TaskStatusCompleted, taskInfo.Status)
}
