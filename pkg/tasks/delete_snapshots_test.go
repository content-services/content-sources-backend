package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/utils"
	zest "github.com/content-services/zest/release/v2024"
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
	t.Setenv("CLIENTS_PULP_SERVER", "mock")
	config.Load()
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
		DistributionHref:            uuid.NewString(),
		PublicationHref:             uuid.NewString(),
		VersionHref:                 uuid.NewString(),
		RepositoryConfigurationUUID: repo.UUID,
	}
	snap2 := models.Snapshot{
		Base: models.Base{
			UUID: uuid.NewString(),
		},
		DistributionHref:            uuid.NewString(),
		PublicationHref:             uuid.NewString(),
		VersionHref:                 uuid.NewString(),
		RepositoryConfigurationUUID: repo.UUID,
	}
	template := api.TemplateResponse{
		UUID:      uuid.NewString(),
		Name:      "test",
		Date:      time.Time{},
		Version:   config.El8,
		Arch:      config.X8664,
		UseLatest: true,
	}
	templateCollection := api.TemplateCollectionResponse{
		Data: []api.TemplateResponse{template},
		Meta: api.ResponseMetadata{Count: 1},
	}
	deleteDistributionHref := uuid.NewString()
	taskInfoFilter := api.TaskInfoFilterData{
		Status:         config.TaskStatusCompleted,
		Typename:       config.RepositorySnapshotTask,
		RepoConfigUUID: repo.UUID,
	}
	taskInfoResp := api.TaskInfoCollectionResponse{
		Data: []api.TaskInfoResponse{{UUID: uuid.NewString()}},
		Meta: api.ResponseMetadata{Count: 1},
	}

	s.mockDaoRegistry.RepositoryConfig.On("Fetch", ctx, orgID, repo.UUID).Return(repo, nil)
	s.mockDaoRegistry.Snapshot.On("FetchModel", ctx, snap.UUID, true).Return(snap, nil)
	s.mockDaoRegistry.Snapshot.On("Delete", ctx, snap.UUID).Return(nil)
	s.mockDaoRegistry.Template.On("List", ctx, orgID, false, mock.Anything, mock.Anything).Return(templateCollection, int64(1), nil)
	s.mockDaoRegistry.Snapshot.On("FetchSnapshotsModelByDateAndRepository", ctx, orgID, mock.Anything).Return([]models.Snapshot{snap2}, nil)
	s.mockDaoRegistry.Template.On("UpdateSnapshots", ctx, template.UUID, []string{snap.RepositoryConfigurationUUID}, []models.Snapshot{snap2}).Return(nil)
	s.mockDaoRegistry.Template.On("DeleteTemplateSnapshot", ctx, snap.UUID).Return(nil)
	s.mockDaoRegistry.Snapshot.On("FetchLatestSnapshotModel", ctx, repo.UUID).Return(snap2, nil)
	s.mockDaoRegistry.TaskInfo.On("List", ctx, orgID, api.PaginationData{Limit: 1}, taskInfoFilter).Return(taskInfoResp, int64(1), nil)
	s.mockDaoRegistry.RepositoryConfig.On("UpdateLastSnapshot", ctx, orgID, repo.UUID, snap2.UUID).Return(nil)
	s.mockDaoRegistry.RepositoryConfig.On("UpdateLastSnapshotTask", ctx, taskInfoResp.Data[0].UUID, orgID, repo.UUID).Return(nil)
	s.mockPulpClient.On("WithDomain", mock.Anything).Return(nil)
	s.mockPulpClient.On("FindDistributionByPath", ctx, mock.Anything).Return(utils.Ptr(zest.RpmRpmDistributionResponse{PulpHref: utils.Ptr(uuid.NewString())}), nil)
	s.mockPulpClient.On("CreateOrUpdateGuardsForOrg", ctx, orgID).Return(uuid.NewString(), nil)
	s.mockPulpClient.On("CreateRpmDistribution", ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(uuid.NewString(), nil)
	s.mockPulpClient.On("UpdateRpmDistribution", ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(uuid.NewString(), nil)
	s.mockPulpClient.On("DeleteRpmDistribution", ctx, snap.DistributionHref).Return(&deleteDistributionHref, nil)
	s.mockPulpClient.On("PollTask", ctx, mock.Anything).Return(nil, nil)
	s.mockPulpClient.On("DeleteRpmRepositoryVersion", ctx, snap.VersionHref).Return(utils.Ptr("taskHref"), nil)

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
