package tasks

import (
	"context"
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	zest "github.com/content-services/zest/release/v2023"
	"github.com/google/uuid"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type DeleteRepositorySnapshotsSuite struct {
	suite.Suite
	mockDaoRegistry *dao.MockDaoRegistry
	MockPulpClient  pulp_client.MockPulpClient
	MockQueue       queue.MockQueue
	Queue           queue.Queue
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
	s.MockQueue = *queue.NewMockQueue(s.T())
	s.Queue = &s.MockQueue
}

func (s *DeleteRepositorySnapshotsSuite) TestLookupOptionalPulpClient() {
	task := models.TaskInfo{
		Id:             uuid.UUID{},
		OrgId:          uuid.NewString(),
		RepositoryUUID: uuid.New(),
	}
	config.Get().Clients.Pulp.Server = "some-server-address" // This ensures that PulpConfigured returns true

	s.mockDaoRegistry.Domain.On("FetchOrCreateDomain", task.OrgId).Return("myDomain", nil)
	s.MockPulpClient.On("LookupDomain", "myDomain").Return("somepath", nil)
	found, err := lookupOptionalPulpClient(context.Background(), s.pulpClient(), &task, s.mockDaoRegistry.ToDaoRegistry())
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), found)
}

func (s *DeleteRepositorySnapshotsSuite) TestLookupOptionalPulpClientWithNoPulpServerConfigured() {
	task := models.TaskInfo{
		Id:             uuid.UUID{},
		OrgId:          uuid.NewString(),
		RepositoryUUID: uuid.New(),
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
		Id:             uuid.UUID{},
		OrgId:          uuid.NewString(),
		RepositoryUUID: uuid.New(),
	}

	s.mockDaoRegistry.Domain.On("FetchOrCreateDomain", task.OrgId).Return("myDomain", nil)
	s.MockPulpClient.On("LookupDomain", "myDomain").Return("", nil)
	found, err := lookupOptionalPulpClient(context.Background(), s.pulpClient(), &task, s.mockDaoRegistry.ToDaoRegistry())
	assert.NoError(s.T(), err)
	assert.Nil(s.T(), found)
}

func (s *DeleteRepositorySnapshotsSuite) TestDeleteNoSnapshotsWithoutClient() {
	repoUuid := uuid.New()
	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}
	task := models.TaskInfo{
		Id:             uuid.UUID{},
		OrgId:          repoConfig.OrgID,
		RepositoryUUID: repoUuid,
	}

	s.mockDaoRegistry.Snapshot.On("FetchForRepoConfigUUID", repoConfig.UUID).Return([]models.Snapshot{}, nil).Once()
	s.mockDaoRegistry.RepositoryConfig.On("Delete", repoConfig.OrgID, repoConfig.UUID).Return(nil).Once()

	payload := DeleteRepositorySnapshotsPayload{
		RepoConfigUUID: repoConfig.UUID,
	}
	snap := DeleteRepositorySnapshots{
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient: nil,
		payload:    &payload,
		task:       &task,
		ctx:        nil,
	}
	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}

func (s *DeleteRepositorySnapshotsSuite) TestDeleteNoSnapshotsWithClient() {
	repoUuid := uuid.New()
	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}
	task := models.TaskInfo{
		Id:             uuid.UUID{},
		OrgId:          repoConfig.OrgID,
		RepositoryUUID: repoUuid,
	}

	s.mockDaoRegistry.Snapshot.On("FetchForRepoConfigUUID", repoConfig.UUID).Return([]models.Snapshot{}, nil).Once()
	s.mockDaoRegistry.RepositoryConfig.On("Delete", repoConfig.OrgID, repoConfig.UUID).Return(nil).Once()

	s.MockPulpClient.On("GetRpmRemoteByName", repoConfig.UUID).Return(nil).Return(nil, nil).Once()
	s.MockPulpClient.On("GetRpmRepositoryByName", repoConfig.UUID).Return(nil, nil).Once()

	payload := DeleteRepositorySnapshotsPayload{
		RepoConfigUUID: repoConfig.UUID,
	}
	pulpClient := s.pulpClient()
	snap := DeleteRepositorySnapshots{
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient: &pulpClient,
		payload:    &payload,
		task:       &task,
		ctx:        nil,
	}
	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}

func (s *DeleteRepositorySnapshotsSuite) TestDeleteSnapshotFull() {
	snapshotId := "abacadaba"
	repoUuid := uuid.New()
	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}
	task := models.TaskInfo{
		Id:             uuid.UUID{},
		OrgId:          repoConfig.OrgID,
		RepositoryUUID: repoUuid,
	}
	counts := zest.RepositoryVersionResponseContentSummary{
		Present: map[string]map[string]interface{}{},
	}
	expectedSnap := models.Snapshot{
		VersionHref:                 "version-href",
		PublicationHref:             "pub-href",
		DistributionHref:            "dist-href",
		DistributionPath:            fmt.Sprintf("%s/%s", repoConfig.UUID, snapshotId),
		RepositoryConfigurationUUID: repoConfig.UUID,
		ContentCounts:               ContentSummaryToContentCounts(&counts),
	}
	taskResp := zest.TaskResponse{PulpHref: pointy.String("taskHref")}
	remoteResp := zest.RpmRpmRemoteResponse{PulpHref: pointy.String("remoteHref"), Url: repoConfig.URL}
	repoResp := zest.RpmRpmRepositoryResponse{PulpHref: pointy.String("repoHref")}

	s.mockDaoRegistry.Snapshot.On("FetchForRepoConfigUUID", repoConfig.UUID).Return([]models.Snapshot{expectedSnap}, nil).Once()
	s.mockDaoRegistry.Snapshot.On("Delete", expectedSnap.UUID).Return(nil).Once()
	s.mockDaoRegistry.RepositoryConfig.On("Delete", repoConfig.OrgID, repoConfig.UUID).Return(nil).Once()

	s.MockPulpClient.On("PollTask", "taskHref").Return(&taskResp, nil).Times(3)
	s.MockPulpClient.On("DeleteRpmRepositoryVersion", expectedSnap.VersionHref).Return(nil).Once()
	s.MockPulpClient.On("DeleteRpmDistribution", expectedSnap.DistributionHref).Return("taskHref", nil).Once()

	s.MockPulpClient.On("GetRpmRemoteByName", repoConfig.UUID).Return(nil).Return(&remoteResp, nil).Once()
	s.MockPulpClient.On("GetRpmRepositoryByName", repoConfig.UUID).Return(&repoResp, nil).Once()
	s.MockPulpClient.On("DeleteRpmRepository", *repoResp.PulpHref).Return("taskHref", nil).Once()
	s.MockPulpClient.On("DeleteRpmRemote", *remoteResp.PulpHref).Return("taskHref", nil).Once()

	payload := DeleteRepositorySnapshotsPayload{
		RepoConfigUUID: repoConfig.UUID,
	}
	pulpClient := s.pulpClient()
	snap := DeleteRepositorySnapshots{
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient: &pulpClient,
		payload:    &payload,
		task:       &task,
		ctx:        nil,
	}
	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}
