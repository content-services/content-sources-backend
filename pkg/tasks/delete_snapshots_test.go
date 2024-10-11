package tasks

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type DeleteSnapshotsSuite struct {
	suite.Suite
	mockDaoRegistry *dao.MockDaoRegistry
	mockPulpClient  pulp_client.MockPulpClient
	MockQueue       queue.MockQueue
	Queue           queue.Queue
}

func TestDeleteSnapshotSuite(t *testing.T) {
	suite.Run(t, new(DeleteSnapshotsSuite))
}

func (s *DeleteSnapshotsSuite) pulpClient() pulp_client.PulpClient {
	return &s.mockPulpClient
}

func (s *DeleteSnapshotsSuite) SetupTest() {
	s.mockDaoRegistry = dao.GetMockDaoRegistry(s.T())
	s.mockPulpClient = *pulp_client.NewMockPulpClient(s.T())
}

func (s *DeleteSnapshotsSuite) TestDeleteSnapshots() {
	t := s.T()
	ctx := context.Background()

	orgID := test_handler.MockOrgId
	repo := api.RepositoryResponse{
		UUID:  uuid.NewString(),
		OrgID: orgID,
	}
	snap := models.Snapshot{
		Base: models.Base{
			UUID: uuid.NewString(),
		},
		DistributionHref: uuid.NewString(),
		PublicationHref:  uuid.NewString(),
		VersionHref:      uuid.NewString(),
	}
	deleteDistributionHref := uuid.NewString()

	s.mockDaoRegistry.RepositoryConfig.On("Fetch", ctx, orgID, repo.UUID).Return(repo, nil)
	s.mockDaoRegistry.Snapshot.On("FetchUnscoped", ctx, snap.UUID).Return(snap, nil)
	s.mockDaoRegistry.Snapshot.On("Delete", ctx, snap.UUID).Return(nil)
	s.mockPulpClient.On("WithDomain", mock.Anything).Return(nil)
	s.mockPulpClient.On("DeleteRpmDistribution", ctx, snap.DistributionHref).Return(deleteDistributionHref, nil)
	s.mockPulpClient.On("PollTask", ctx, deleteDistributionHref).Return(nil, nil)
	s.mockPulpClient.On("DeleteRpmPublication", ctx, snap.PublicationHref).Return(nil)
	s.mockPulpClient.On("DeleteRpmRepositoryVersion", ctx, snap.VersionHref).Return("taskHref", nil)

	pulpClient := s.pulpClient()
	task := DeleteSnapshots{
		orgID: orgID,
		ctx:   ctx,
		payload: utils.Ptr(payloads.DeleteSnapshotsPayload{
			RepoUUID:       repo.UUID,
			SnapshotsUUIDs: []string{snap.UUID},
		}),
		task:       nil,
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient: &pulpClient,
	}

	taskErr := task.Run()
	assert.NoError(t, taskErr)
}
