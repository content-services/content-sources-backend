package tasks

import (
	"context"
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	zest "github.com/content-services/zest/release/v2024"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type DeleteRepositorySnapshotsSuite struct {
	suite.Suite
	mockDaoRegistry *dao.MockDaoRegistry
	MockPulpClient  pulp_client.MockPulpClient
	MockQueue       queue.MockQueue
	Queue           queue.Queue
	mockCpClient    candlepin_client.MockCandlepinClient
}

func TestDeleteSnapshotSuite(t *testing.T) {
	suite.Run(t, new(DeleteRepositorySnapshotsSuite))
}

func (s *DeleteRepositorySnapshotsSuite) pulpClient() pulp_client.PulpClient {
	return &s.MockPulpClient
}

func (s *DeleteRepositorySnapshotsSuite) SetupTest() {
	s.mockDaoRegistry = dao.GetMockDaoRegistry(s.T())
	s.MockPulpClient = *pulp_client.NewMockPulpClient(s.T())
	s.mockCpClient = *candlepin_client.NewMockCandlepinClient(s.T())
}

func (s *DeleteRepositorySnapshotsSuite) TestLookupOptionalPulpClient() {
	task := models.TaskInfo{
		Id:         uuid.UUID{},
		OrgId:      uuid.NewString(),
		ObjectUUID: uuid.New(),
	}
	ctx := context.Background()
	config.Get().Clients.Pulp.Server = "some-server-address" // This ensures that PulpConfigured returns true

	s.mockDaoRegistry.Domain.On("FetchOrCreateDomain", ctx, task.OrgId).Return("myDomain", nil)
	s.MockPulpClient.On("LookupDomain", ctx, "myDomain").Return("somepath", nil)
	found, err := lookupOptionalPulpClient(ctx, s.pulpClient(), &task, s.mockDaoRegistry.ToDaoRegistry())
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), found)
}

func (s *DeleteRepositorySnapshotsSuite) TestLookupOptionalPulpClientWithNoPulpServerConfigured() {
	task := models.TaskInfo{
		Id:         uuid.UUID{},
		OrgId:      uuid.NewString(),
		ObjectUUID: uuid.New(),
	}
	config.Get().Clients.Pulp.Server = "" // This ensures that PulpConfigured returns false
	s.mockDaoRegistry.Domain.On("FetchOrCreateDomain", task.OrgId).Return("myDomain", nil)
	s.MockPulpClient.On("LookupDomain", "myDomain").Return("somepath", nil)
	found, err := lookupOptionalPulpClient(context.Background(), s.pulpClient(), &task, s.mockDaoRegistry.ToDaoRegistry())
	assert.Nil(s.T(), err)
	assert.Nil(s.T(), found)
}

func (s *DeleteRepositorySnapshotsSuite) TestLookupOptionalPulpClientNil() {
	task := models.TaskInfo{
		Id:         uuid.UUID{},
		OrgId:      uuid.NewString(),
		ObjectUUID: uuid.New(),
	}
	ctx := context.Background()
	s.mockDaoRegistry.Domain.On("FetchOrCreateDomain", ctx, task.OrgId).Return("myDomain", nil)
	s.MockPulpClient.On("LookupDomain", ctx, "myDomain").Return("", nil)
	found, err := lookupOptionalPulpClient(ctx, s.pulpClient(), &task, s.mockDaoRegistry.ToDaoRegistry())
	assert.NoError(s.T(), err)
	assert.Nil(s.T(), found)
}

func (s *DeleteRepositorySnapshotsSuite) TestDeleteNoSnapshotsWithoutClient() {
	repoUuid := uuid.New()
	ctx := context.Background()
	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}
	task := models.TaskInfo{
		Id:         uuid.UUID{},
		OrgId:      repoConfig.OrgID,
		ObjectUUID: repoUuid,
	}

	s.mockDaoRegistry.Snapshot.On("FetchForRepoConfigUUID", ctx, repoConfig.UUID).Return([]models.Snapshot{}, nil).Once()
	s.mockDaoRegistry.RepositoryConfig.On("Delete", ctx, repoConfig.OrgID, repoConfig.UUID).Return(nil).Once()
	s.mockCpClient.On("DeleteContent", ctx, repoConfig.OrgID, repoConfig.UUID).Return(nil).Once()
	s.mockDaoRegistry.Template.On("List", ctx, repoConfig.OrgID, api.PaginationData{Limit: -1}, api.TemplateFilterData{RepositoryUUIDs: []string{repoConfig.UUID}}).Return(api.TemplateCollectionResponse{}, int64(0), nil).Once()

	payload := DeleteRepositorySnapshotsPayload{
		RepoConfigUUID: repoConfig.UUID,
	}
	snap := DeleteRepositorySnapshots{
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient: nil,
		payload:    &payload,
		task:       &task,
		ctx:        ctx,
		cpClient:   &s.mockCpClient,
	}
	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}

func (s *DeleteRepositorySnapshotsSuite) TestDeleteNoSnapshotsWithClient() {
	repoUuid := uuid.New()
	ctx := context.Background()
	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}
	task := models.TaskInfo{
		Id:         uuid.UUID{},
		OrgId:      repoConfig.OrgID,
		ObjectUUID: repoUuid,
	}

	s.mockDaoRegistry.Snapshot.On("FetchForRepoConfigUUID", ctx, repoConfig.UUID).Return([]models.Snapshot{}, nil).Once()
	s.mockDaoRegistry.RepositoryConfig.On("Delete", ctx, repoConfig.OrgID, repoConfig.UUID).Return(nil).Once()

	s.MockPulpClient.On("FindDistributionByPath", ctx, fmt.Sprintf("%v/%v", repoConfig.UUID, "latest")).Return(nil, nil).Once()
	s.MockPulpClient.On("GetRpmRemoteByName", ctx, repoConfig.UUID).Return(nil).Return(nil, nil).Once()
	s.MockPulpClient.On("GetRpmRepositoryByName", ctx, repoConfig.UUID).Return(nil, nil).Once()

	s.mockCpClient.On("DeleteContent", ctx, repoConfig.OrgID, repoConfig.UUID).Return(nil).Once()
	s.mockDaoRegistry.Template.On("List", ctx, repoConfig.OrgID, api.PaginationData{Limit: -1}, api.TemplateFilterData{RepositoryUUIDs: []string{repoConfig.UUID}}).Return(api.TemplateCollectionResponse{}, int64(0), nil).Once()

	payload := DeleteRepositorySnapshotsPayload{
		RepoConfigUUID: repoConfig.UUID,
	}
	pulpClient := s.pulpClient()
	snap := DeleteRepositorySnapshots{
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient: &pulpClient,
		payload:    &payload,
		task:       &task,
		ctx:        ctx,
		cpClient:   &s.mockCpClient,
	}
	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}

func (s *DeleteRepositorySnapshotsSuite) TestDeleteSnapshotFull() {
	snapshotId := "abacadaba"
	ctx := context.Background()
	repoUuid := uuid.New()
	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}
	task := models.TaskInfo{
		Id:         uuid.UUID{},
		OrgId:      repoConfig.OrgID,
		ObjectUUID: repoUuid,
	}
	counts := zest.ContentSummaryResponse{
		Present: map[string]map[string]interface{}{},
		Added:   map[string]map[string]interface{}{},
		Removed: map[string]map[string]interface{}{},
	}
	current, added, removed := ContentSummaryToContentCounts(&counts)
	expectedSnap := models.Snapshot{
		VersionHref:                 "version-href",
		PublicationHref:             "pub-href",
		DistributionHref:            "dist-href",
		DistributionPath:            fmt.Sprintf("%s/%s", repoConfig.UUID, snapshotId),
		RepositoryConfigurationUUID: repoConfig.UUID,
		ContentCounts:               current,
		AddedCounts:                 added,
		RemovedCounts:               removed,
	}
	taskResp := zest.TaskResponse{PulpHref: utils.Ptr("taskHref")}
	remoteResp := zest.RpmRpmRemoteResponse{PulpHref: utils.Ptr("remoteHref"), Url: repoConfig.URL}
	repoResp := zest.RpmRpmRepositoryResponse{PulpHref: utils.Ptr("repoHref")}

	s.mockDaoRegistry.Snapshot.On("FetchForRepoConfigUUID", ctx, repoConfig.UUID).Return([]models.Snapshot{expectedSnap}, nil).Once()
	s.mockDaoRegistry.Snapshot.On("Delete", ctx, expectedSnap.UUID).Return(nil).Once()
	s.mockDaoRegistry.RepositoryConfig.On("Delete", ctx, repoConfig.OrgID, repoConfig.UUID).Return(nil).Once()
	s.mockDaoRegistry.Template.On("DeleteTemplateSnapshot", ctx, expectedSnap.UUID).Return(nil).Once()

	s.MockPulpClient.On("PollTask", ctx, "taskHref").Return(&taskResp, nil).Times(3)
	s.MockPulpClient.On("DeleteRpmRepositoryVersion", ctx, expectedSnap.VersionHref).Return(nil).Once()
	s.MockPulpClient.On("FindDistributionByPath", ctx, fmt.Sprintf("%v/%v", repoConfig.UUID, "latest")).Return(nil, nil).Once()
	s.MockPulpClient.On("DeleteRpmDistribution", ctx, expectedSnap.DistributionHref).Return("taskHref", nil).Once()

	s.MockPulpClient.On("GetRpmRemoteByName", ctx, repoConfig.UUID).Return(nil).Return(&remoteResp, nil).Once()
	s.MockPulpClient.On("GetRpmRepositoryByName", ctx, repoConfig.UUID).Return(&repoResp, nil).Once()
	s.MockPulpClient.On("DeleteRpmRepository", ctx, *repoResp.PulpHref).Return("taskHref", nil).Once()
	s.MockPulpClient.On("DeleteRpmRemote", ctx, *remoteResp.PulpHref).Return("taskHref", nil).Once()

	s.mockCpClient.On("DeleteContent", ctx, repoConfig.OrgID, repoConfig.UUID).Return(nil).Once()
	s.mockDaoRegistry.Template.On("List", ctx, repoConfig.OrgID, api.PaginationData{Limit: -1}, api.TemplateFilterData{RepositoryUUIDs: []string{repoConfig.UUID}}).Return(api.TemplateCollectionResponse{}, int64(0), nil).Once()

	payload := DeleteRepositorySnapshotsPayload{
		RepoConfigUUID: repoConfig.UUID,
	}
	pulpClient := s.pulpClient()
	snap := DeleteRepositorySnapshots{
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient: &pulpClient,
		payload:    &payload,
		task:       &task,
		ctx:        ctx,
		cpClient:   &s.mockCpClient,
	}
	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}
