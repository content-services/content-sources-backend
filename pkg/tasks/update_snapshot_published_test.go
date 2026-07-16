package tasks

import (
	"context"
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type UpdateSnapshotPublishedSuite struct {
	suite.Suite
	mockDaoRegistry *dao.MockDaoRegistry
	mockPulpClient  pulp_client.MockPulpClient
}

func TestUpdateSnapshotPublishedSuite(t *testing.T) {
	suite.Run(t, new(UpdateSnapshotPublishedSuite))
}

func (s *UpdateSnapshotPublishedSuite) SetupTest() {
	s.mockDaoRegistry = dao.GetMockDaoRegistry(s.T())
	s.mockPulpClient = *pulp_client.NewMockPulpClient(s.T())
	s.T().Setenv("CLIENTS_PULP_SERVER", "mock")
	config.Load()
	config.Get().Clients.Pulp.RepoContentGuards = true
}

func (s *UpdateSnapshotPublishedSuite) newTask(snap models.Snapshot, published bool) UpdateSnapshotPublished {
	return UpdateSnapshotPublished{
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		ctx:        context.Background(),
		orgID:      test_handler.MockOrgId,
		pulpClient: &s.mockPulpClient,
		payload: &UpdateSnapshotPublishedPayload{
			SnapshotUUID: snap.UUID,
			Published:    published,
		},
	}
}

func (s *UpdateSnapshotPublishedSuite) TestPublishRemovesContentGuard() {
	ctx := context.Background()

	repoConfigUUID := uuid.NewString()
	distName := "test-dist"
	distPath := "test/path"
	distHref := "/pulp/distribution/" + uuid.NewString()
	pubHref := "/pulp/publication/" + uuid.NewString()
	taskHref := uuid.NewString()

	snap := models.Snapshot{
		Base:                        models.Base{UUID: uuid.NewString()},
		RepositoryConfigurationUUID: repoConfigUUID,
		DistributionHref:            distHref,
		DistributionPath:            distPath,
		PublicationHref:             pubHref,
	}

	s.mockDaoRegistry.Snapshot.On("FetchModel", ctx, snap.UUID, false).Return(snap, nil)
	s.mockPulpClient.On("FindDistributionByPath", ctx, distPath).Return(&zest.RpmRpmDistributionResponse{
		PulpHref: &distHref,
		Name:     distName,
		BasePath: distPath,
	}, nil)
	// Publish: content guard should be nil
	s.mockPulpClient.On("UpdateRpmDistribution", ctx, distHref, pubHref, distName, distPath, (*string)(nil)).Return(taskHref, nil)
	s.mockPulpClient.On("PollTask", ctx, taskHref).Return(&zest.TaskResponse{}, nil)

	// Latest distribution: no latest dist exists
	latestPath := repoConfigUUID + "/latest"
	s.mockPulpClient.On("FindDistributionByPath", ctx, latestPath).Return((*zest.RpmRpmDistributionResponse)(nil), nil)

	task := s.newTask(snap, true)
	err := task.Run()
	assert.NoError(s.T(), err)
}

func (s *UpdateSnapshotPublishedSuite) TestUnpublishRestoresOrgGuard() {
	ctx := context.Background()
	repoConfigUUID := uuid.NewString()
	distName := "test-dist"
	distPath := "test/path"
	distHref := "/pulp/distribution/" + uuid.NewString()
	pubHref := "/pulp/publication/" + uuid.NewString()
	guardHref := "/pulp/guard/" + uuid.NewString()
	taskHref := uuid.NewString()

	snap := models.Snapshot{
		Base:                        models.Base{UUID: uuid.NewString()},
		RepositoryConfigurationUUID: repoConfigUUID,
		DistributionHref:            distHref,
		DistributionPath:            distPath,
		PublicationHref:             pubHref,
	}

	s.mockDaoRegistry.Snapshot.On("FetchModel", ctx, snap.UUID, false).Return(snap, nil)
	s.mockPulpClient.On("FindDistributionByPath", ctx, distPath).Return(&zest.RpmRpmDistributionResponse{
		PulpHref: &distHref,
		Name:     distName,
		BasePath: distPath,
	}, nil)
	// Unpublish: should restore org guard
	s.mockPulpClient.On("CreateOrUpdateGuardsForOrg", ctx, test_handler.MockOrgId).Return(guardHref, nil)
	s.mockPulpClient.On("UpdateRpmDistribution", ctx, distHref, pubHref, distName, distPath, &guardHref).Return(taskHref, nil)
	s.mockPulpClient.On("PollTask", ctx, taskHref).Return(&zest.TaskResponse{}, nil)

	// Latest distribution: no latest dist exists
	latestPath := repoConfigUUID + "/latest"
	s.mockPulpClient.On("FindDistributionByPath", ctx, latestPath).Return((*zest.RpmRpmDistributionResponse)(nil), nil)

	task := s.newTask(snap, false)
	err := task.Run()
	assert.NoError(s.T(), err)
}

func (s *UpdateSnapshotPublishedSuite) TestSnapshotNotFound() {
	ctx := context.Background()
	snapUUID := uuid.NewString()

	s.mockDaoRegistry.Snapshot.On("FetchModel", ctx, snapUUID, false).Return(models.Snapshot{}, fmt.Errorf("not found"))

	task := UpdateSnapshotPublished{
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		ctx:        ctx,
		orgID:      test_handler.MockOrgId,
		pulpClient: &s.mockPulpClient,
		payload: &UpdateSnapshotPublishedPayload{
			SnapshotUUID: snapUUID,
			Published:    true,
		},
	}

	err := task.Run()
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "error fetching snapshot")
}

func (s *UpdateSnapshotPublishedSuite) TestDistributionNotFound() {
	ctx := context.Background()
	distPath := "test/path"

	snap := models.Snapshot{
		Base:             models.Base{UUID: uuid.NewString()},
		DistributionHref: "/pulp/distribution/" + uuid.NewString(),
		DistributionPath: distPath,
		PublicationHref:  "/pulp/publication/" + uuid.NewString(),
	}

	s.mockDaoRegistry.Snapshot.On("FetchModel", ctx, snap.UUID, false).Return(snap, nil)
	s.mockPulpClient.On("FindDistributionByPath", ctx, distPath).Return((*zest.RpmRpmDistributionResponse)(nil), nil)

	task := s.newTask(snap, true)
	err := task.Run()
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "distribution not found")
}

func (s *UpdateSnapshotPublishedSuite) TestDistributionFetchError() {
	ctx := context.Background()
	distPath := "test/path"

	snap := models.Snapshot{
		Base:             models.Base{UUID: uuid.NewString()},
		DistributionHref: "/pulp/distribution/" + uuid.NewString(),
		DistributionPath: distPath,
		PublicationHref:  "/pulp/publication/" + uuid.NewString(),
	}

	s.mockDaoRegistry.Snapshot.On("FetchModel", ctx, snap.UUID, false).Return(snap, nil)
	s.mockPulpClient.On("FindDistributionByPath", ctx, distPath).Return((*zest.RpmRpmDistributionResponse)(nil), fmt.Errorf("pulp error"))

	task := s.newTask(snap, true)
	err := task.Run()
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "error fetching distribution")
}

func (s *UpdateSnapshotPublishedSuite) TestUpdateDistributionError() {
	ctx := context.Background()
	distName := "test-dist"
	distPath := "test/path"
	distHref := "/pulp/distribution/" + uuid.NewString()
	pubHref := "/pulp/publication/" + uuid.NewString()

	snap := models.Snapshot{
		Base:             models.Base{UUID: uuid.NewString()},
		DistributionHref: distHref,
		DistributionPath: distPath,
		PublicationHref:  pubHref,
	}

	s.mockDaoRegistry.Snapshot.On("FetchModel", ctx, snap.UUID, false).Return(snap, nil)
	s.mockPulpClient.On("FindDistributionByPath", ctx, distPath).Return(&zest.RpmRpmDistributionResponse{
		PulpHref: &distHref,
		Name:     distName,
		BasePath: distPath,
	}, nil)
	s.mockPulpClient.On("UpdateRpmDistribution", ctx, distHref, pubHref, distName, distPath, (*string)(nil)).Return("", fmt.Errorf("update failed"))

	task := s.newTask(snap, true)
	err := task.Run()
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "error updating distribution content guard")
}

func (s *UpdateSnapshotPublishedSuite) TestPollTaskError() {
	ctx := context.Background()
	distName := "test-dist"
	distPath := "test/path"
	distHref := "/pulp/distribution/" + uuid.NewString()
	pubHref := "/pulp/publication/" + uuid.NewString()
	taskHref := uuid.NewString()

	snap := models.Snapshot{
		Base:             models.Base{UUID: uuid.NewString()},
		DistributionHref: distHref,
		DistributionPath: distPath,
		PublicationHref:  pubHref,
	}

	s.mockDaoRegistry.Snapshot.On("FetchModel", ctx, snap.UUID, false).Return(snap, nil)
	s.mockPulpClient.On("FindDistributionByPath", ctx, distPath).Return(&zest.RpmRpmDistributionResponse{
		PulpHref: &distHref,
		Name:     distName,
		BasePath: distPath,
	}, nil)
	s.mockPulpClient.On("UpdateRpmDistribution", ctx, distHref, pubHref, distName, distPath, (*string)(nil)).Return(taskHref, nil)
	s.mockPulpClient.On("PollTask", ctx, taskHref).Return((*zest.TaskResponse)(nil), fmt.Errorf("task failed"))

	task := s.newTask(snap, true)
	err := task.Run()
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "error polling distribution update task")
}

func (s *UpdateSnapshotPublishedSuite) TestUnpublishFetchGuardError() {
	ctx := context.Background()
	distName := "test-dist"
	distPath := "test/path"
	distHref := "/pulp/distribution/" + uuid.NewString()

	snap := models.Snapshot{
		Base:             models.Base{UUID: uuid.NewString()},
		DistributionHref: distHref,
		DistributionPath: distPath,
		PublicationHref:  "/pulp/publication/" + uuid.NewString(),
	}

	s.mockDaoRegistry.Snapshot.On("FetchModel", ctx, snap.UUID, false).Return(snap, nil)
	s.mockPulpClient.On("FindDistributionByPath", ctx, distPath).Return(&zest.RpmRpmDistributionResponse{
		PulpHref: &distHref,
		Name:     distName,
		BasePath: distPath,
	}, nil)
	s.mockPulpClient.On("CreateOrUpdateGuardsForOrg", ctx, test_handler.MockOrgId).Return("", fmt.Errorf("guard error"))

	task := s.newTask(snap, false)
	err := task.Run()
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "error fetching content guard")
}

func (s *UpdateSnapshotPublishedSuite) TestUpdateLatestDistributionPublicRetargetsAndRemovesGuard() {
	ctx := context.Background()
	repoConfigUUID := uuid.NewString()
	latestPath := repoConfigUUID + "/latest"
	latestDistHref := "/pulp/distribution/" + uuid.NewString()
	latestDistName := "latest-dist"
	oldPubHref := "/pulp/publication/" + uuid.NewString()
	newPubHref := "/pulp/publication/" + uuid.NewString()
	taskHref := uuid.NewString()

	latestSnap := models.Snapshot{
		Base:            models.Base{UUID: uuid.NewString()},
		PublicationHref: newPubHref,
	}

	s.mockPulpClient.On("FindDistributionByPath", ctx, latestPath).Return(&zest.RpmRpmDistributionResponse{
		PulpHref:    &latestDistHref,
		Name:        latestDistName,
		BasePath:    latestPath,
		Publication: *zest.NewNullableString(&oldPubHref),
	}, nil)
	s.mockDaoRegistry.Snapshot.On("FetchLatestSnapshotModel", ctx, repoConfigUUID).Return(latestSnap, nil)
	s.mockDaoRegistry.Repository.On("FetchPublicStatus", ctx, repoConfigUUID).Return(true, nil)
	// Public: retarget to latest published snapshot publication, content guard nil
	s.mockPulpClient.On("UpdateRpmDistribution", ctx, latestDistHref, newPubHref, latestDistName, latestPath, (*string)(nil)).Return(taskHref, nil)
	s.mockPulpClient.On("PollTask", ctx, taskHref).Return(&zest.TaskResponse{}, nil)

	task := UpdateSnapshotPublished{
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		ctx:        ctx,
		orgID:      test_handler.MockOrgId,
		pulpClient: &s.mockPulpClient,
		payload:    &UpdateSnapshotPublishedPayload{},
	}
	err := task.updateLatestDistribution(repoConfigUUID)
	assert.NoError(s.T(), err)
}

func (s *UpdateSnapshotPublishedSuite) TestUpdateLatestDistributionNotPublicRetargetsAndRestoresGuard() {
	ctx := context.Background()
	repoConfigUUID := uuid.NewString()
	latestPath := repoConfigUUID + "/latest"
	latestDistHref := "/pulp/distribution/" + uuid.NewString()
	latestDistName := "latest-dist"
	oldPubHref := "/pulp/publication/" + uuid.NewString()
	newPubHref := "/pulp/publication/" + uuid.NewString()
	guardHref := "/pulp/guard/" + uuid.NewString()
	taskHref := uuid.NewString()

	latestSnap := models.Snapshot{
		Base:            models.Base{UUID: uuid.NewString()},
		PublicationHref: newPubHref,
	}

	s.mockPulpClient.On("FindDistributionByPath", ctx, latestPath).Return(&zest.RpmRpmDistributionResponse{
		PulpHref:    &latestDistHref,
		Name:        latestDistName,
		BasePath:    latestPath,
		Publication: *zest.NewNullableString(&oldPubHref),
	}, nil)
	s.mockDaoRegistry.Snapshot.On("FetchLatestSnapshotModel", ctx, repoConfigUUID).Return(latestSnap, nil)
	s.mockDaoRegistry.Repository.On("FetchPublicStatus", ctx, repoConfigUUID).Return(false, nil)
	s.mockPulpClient.On("CreateOrUpdateGuardsForOrg", ctx, test_handler.MockOrgId).Return(guardHref, nil)
	s.mockPulpClient.On("UpdateRpmDistribution", ctx, latestDistHref, newPubHref, latestDistName, latestPath, &guardHref).Return(taskHref, nil)
	s.mockPulpClient.On("PollTask", ctx, taskHref).Return(&zest.TaskResponse{}, nil)

	task := UpdateSnapshotPublished{
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		ctx:        ctx,
		orgID:      test_handler.MockOrgId,
		pulpClient: &s.mockPulpClient,
		payload:    &UpdateSnapshotPublishedPayload{},
	}
	err := task.updateLatestDistribution(repoConfigUUID)
	assert.NoError(s.T(), err)
}

func (s *UpdateSnapshotPublishedSuite) TestUpdateLatestDistributionNoLatestDist() {
	ctx := context.Background()
	repoConfigUUID := uuid.NewString()
	latestPath := repoConfigUUID + "/latest"

	s.mockPulpClient.On("FindDistributionByPath", ctx, latestPath).Return((*zest.RpmRpmDistributionResponse)(nil), nil)

	task := UpdateSnapshotPublished{
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		ctx:        ctx,
		orgID:      test_handler.MockOrgId,
		pulpClient: &s.mockPulpClient,
		payload:    &UpdateSnapshotPublishedPayload{},
	}
	err := task.updateLatestDistribution(repoConfigUUID)
	assert.NoError(s.T(), err)
}

func (s *UpdateSnapshotPublishedSuite) TestUpdateLatestDistributionFetchLatestSnapshotError() {
	ctx := context.Background()
	repoConfigUUID := uuid.NewString()
	latestPath := repoConfigUUID + "/latest"
	latestDistHref := "/pulp/distribution/" + uuid.NewString()
	latestDistName := "latest-dist"
	oldPubHref := "/pulp/publication/" + uuid.NewString()

	s.mockPulpClient.On("FindDistributionByPath", ctx, latestPath).Return(&zest.RpmRpmDistributionResponse{
		PulpHref:    &latestDistHref,
		Name:        latestDistName,
		BasePath:    latestPath,
		Publication: *zest.NewNullableString(&oldPubHref),
	}, nil)
	s.mockDaoRegistry.Snapshot.On("FetchLatestSnapshotModel", ctx, repoConfigUUID).Return(models.Snapshot{}, fmt.Errorf("not found"))

	task := UpdateSnapshotPublished{
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		ctx:        ctx,
		orgID:      test_handler.MockOrgId,
		pulpClient: &s.mockPulpClient,
		payload:    &UpdateSnapshotPublishedPayload{},
	}
	err := task.updateLatestDistribution(repoConfigUUID)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "error fetching latest snapshot")
}
