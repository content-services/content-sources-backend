package tasks

import (
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	zest "github.com/content-services/zest/release/v3"
	"github.com/google/uuid"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type SnapshotSuite struct {
	suite.Suite
	mockDaoRegistry *dao.MockDaoRegistry
	MockPulpClient  pulp_client.MockPulpClient
	MockQueue       queue.MockQueue
	Queue           queue.Queue
}

func TestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(SnapshotSuite))
}

func (s *SnapshotSuite) SetupTest() {
	s.mockDaoRegistry = dao.GetMockDaoRegistry(s.T())
	s.MockPulpClient = *pulp_client.NewMockPulpClient(s.T())
	s.MockQueue = *queue.NewMockQueue(s.T())
	s.Queue = &s.MockQueue
}

func (s *SnapshotSuite) TestSnapshotFull() {
	snapshotId := "abacadaba"
	repoUuid := uuid.New()

	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}
	task := models.TaskInfo{
		Id:             uuid.UUID{},
		OrgId:          repoConfig.OrgID,
		RepositoryUUID: repoUuid,
	}

	s.mockDaoRegistry.RepositoryConfig.On("FetchByRepoUuid", repoConfig.OrgID, repo.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.RepositoryConfig.On("Fetch", repoConfig.OrgID, repoConfig.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.Repository.On("FetchForUrl", repoConfig.URL).Return(repo, nil)

	remoteHref := s.mockRemoteCreate(repoConfig, false)
	repoResp := s.mockRepoCreate(repoConfig, remoteHref, false)

	taskHref := "SyncTaskHref"
	s.MockPulpClient.On("SyncRpmRepository", *(repoResp.PulpHref), (*string)(nil)).Return(taskHref, nil)

	versionHref, syncTask := s.mockSync(taskHref, true)
	pubHref, pubTask := s.mockPublish(*versionHref, false)
	distHref, distTask := s.mockCreateDist(pubHref)

	s.MockQueue.On("UpdatePayload", &task, payloads.SnapshotPayload{
		SnapshotIdent: &snapshotId,
		SyncTaskHref:  &syncTask,
	}).Return(&task, nil)
	s.MockQueue.On("UpdatePayload", &task, payloads.SnapshotPayload{
		SnapshotIdent:       &snapshotId,
		SyncTaskHref:        &syncTask,
		PublicationTaskHref: &pubTask,
	}).Return(&task, nil)
	s.MockQueue.On("UpdatePayload", &task, payloads.SnapshotPayload{
		SnapshotIdent:        &snapshotId,
		SyncTaskHref:         &syncTask,
		PublicationTaskHref:  &pubTask,
		DistributionTaskHref: &distTask,
	}).Return(&task, nil)

	// Lookup the version
	counts := zest.RepositoryVersionResponseContentSummary{
		Present: map[string]map[string]interface{}{},
	}
	rpmVersion := zest.RepositoryVersionResponse{
		PulpHref:       versionHref,
		ContentSummary: &counts,
	}
	s.MockPulpClient.On("GetRpmRepositoryVersion", *versionHref).Return(&rpmVersion, nil)

	expectedSnap := models.Snapshot{
		VersionHref:      *versionHref,
		PublicationHref:  pubHref,
		DistributionHref: distHref,
		DistributionPath: fmt.Sprintf("%s/%s", repoConfig.UUID, snapshotId),
		OrgId:            repoConfig.OrgID,
		RepositoryUUID:   repoUuid.String(),
		ContentCounts:    ContentSummaryToContentCounts(&counts),
	}

	payload := payloads.SnapshotPayload{
		SnapshotIdent: &snapshotId,
	}

	snap := SnapshotRepository{
		orgId:          repoConfig.OrgID,
		repositoryUUID: repoUuid,
		daoReg:         s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient:     &s.MockPulpClient,
		payload:        &payload,
		task:           &task,
		queue:          &s.Queue,
		ctx:            nil,
	}

	s.mockDaoRegistry.Snapshot.On("Create", &expectedSnap).Return(nil).Once()

	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}

func (s *SnapshotSuite) TestSnapshotResync() {
	repoUuid := uuid.New()
	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}

	s.mockDaoRegistry.RepositoryConfig.On("FetchByRepoUuid", repoConfig.OrgID, repo.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.RepositoryConfig.On("Fetch", repoConfig.OrgID, repoConfig.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.Repository.On("FetchForUrl", repoConfig.URL).Return(repo, nil)

	remoteHref := s.mockRemoteCreate(repoConfig, true)
	repoResp := s.mockRepoCreate(repoConfig, remoteHref, true)

	taskHref := "SyncTaskHref"
	s.MockPulpClient.On("SyncRpmRepository", *(repoResp.PulpHref), (*string)(nil)).Return(taskHref, nil)

	_, syncTask := s.mockSync(taskHref, false)

	task := models.TaskInfo{
		Id:             uuid.UUID{},
		OrgId:          repoConfig.OrgID,
		RepositoryUUID: repoUuid,
	}

	s.MockQueue.On("UpdatePayload", &task, payloads.SnapshotPayload{
		SyncTaskHref: &syncTask,
	}).Return(&task, nil)

	snap := SnapshotRepository{
		orgId:          repoConfig.OrgID,
		repositoryUUID: repoUuid,
		daoReg:         s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient:     &s.MockPulpClient,
		payload:        &payloads.SnapshotPayload{},
		task:           &task,
		queue:          &s.Queue,
		ctx:            nil,
	}
	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}

// TestSnapshotRestartAfterSync this test simulates what happens if a worker is restarted after the sync has started, and a
// sync task is already present in the payload
func (s *SnapshotSuite) TestSnapshotRestartAfterSync() {
	snapshotId := "abacadaba"
	repoUuid := uuid.New()

	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}
	task := models.TaskInfo{
		Id:             uuid.UUID{},
		OrgId:          repoConfig.OrgID,
		RepositoryUUID: repoUuid,
	}

	s.mockDaoRegistry.RepositoryConfig.On("FetchByRepoUuid", repoConfig.OrgID, repo.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.RepositoryConfig.On("Fetch", repoConfig.OrgID, repoConfig.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.Repository.On("FetchForUrl", repoConfig.URL).Return(repo, nil)

	remoteHref := s.mockRemoteCreate(repoConfig, false)
	s.mockRepoCreate(repoConfig, remoteHref, false)
	versionHref := "some/version"

	syncTaskHref := "SyncTaskHref"
	syncTask := zest.TaskResponse{
		PulpHref:         pointy.String(syncTaskHref),
		PulpCreated:      nil,
		State:            pointy.String("completed"),
		CreatedResources: []string{versionHref},
	}
	s.MockPulpClient.On("PollTask", syncTaskHref).Return(&syncTask, nil)

	pubHref, pubTask := s.mockPublish(versionHref, false)
	distHref, distTask := s.mockCreateDist(pubHref)

	s.MockQueue.On("UpdatePayload", &task, payloads.SnapshotPayload{
		SnapshotIdent:       &snapshotId,
		SyncTaskHref:        &syncTaskHref,
		PublicationTaskHref: &pubTask,
	}).Return(&task, nil)
	s.MockQueue.On("UpdatePayload", &task, payloads.SnapshotPayload{
		SnapshotIdent:        &snapshotId,
		SyncTaskHref:         &syncTaskHref,
		PublicationTaskHref:  &pubTask,
		DistributionTaskHref: &distTask,
	}).Return(&task, nil)

	// Lookup the version
	counts := zest.RepositoryVersionResponseContentSummary{
		Present: map[string]map[string]interface{}{},
	}
	rpmVersion := zest.RepositoryVersionResponse{
		PulpHref:       &versionHref,
		ContentSummary: &counts,
	}
	s.MockPulpClient.On("GetRpmRepositoryVersion", versionHref).Return(&rpmVersion, nil)

	expectedSnap := models.Snapshot{
		VersionHref:      versionHref,
		PublicationHref:  pubHref,
		DistributionHref: distHref,
		DistributionPath: fmt.Sprintf("%s/%s", repoConfig.UUID, snapshotId),
		OrgId:            repoConfig.OrgID,
		RepositoryUUID:   repoUuid.String(),
		ContentCounts:    ContentSummaryToContentCounts(&counts),
	}

	payload := payloads.SnapshotPayload{
		SnapshotIdent: &snapshotId,
		SyncTaskHref:  &syncTaskHref,
	}

	snap := SnapshotRepository{
		orgId:          repoConfig.OrgID,
		repositoryUUID: repoUuid,
		daoReg:         s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient:     &s.MockPulpClient,
		payload:        &payload,
		task:           &task,
		queue:          &s.Queue,
		ctx:            nil,
	}

	s.mockDaoRegistry.Snapshot.On("Create", &expectedSnap).Return(nil).Once()

	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}

func (s *SnapshotSuite) mockCreateDist(pubHref string) (string, string) {
	distPath := mock.AnythingOfType("string")
	distHref := "/pulp/api/v3/distributions/rpm/rpm/" + uuid.NewString() + "/"
	distTaskhref := "distTaskHref"

	s.MockPulpClient.On("FindDistributionByPath", distPath).Return(nil, nil)
	s.MockPulpClient.On("CreateRpmDistribution", pubHref, mock.AnythingOfType("string"), distPath).Return(&distTaskhref, nil).Once()
	status := pulp_client.COMPLETED
	task := zest.TaskResponse{
		PulpHref:         &distTaskhref,
		State:            &status,
		CreatedResources: []string{distHref},
	}
	s.MockPulpClient.On("PollTask", distTaskhref).Return(&task, nil).Once()
	return distHref, distTaskhref
}

func (s *SnapshotSuite) mockPublish(versionHref string, existing bool) (string, string) {
	publishTaskHref := "SyncTaskHref"
	pubHref := "/pulp/api/v3/publications/rpm/rpm/" + uuid.NewString() + "/"

	if existing {
		resp := zest.RpmRpmPublicationResponse{PulpHref: &pubHref}
		s.MockPulpClient.On("FindRpmPublicationByVersion", versionHref).Return(&resp, nil).Once()
		return pubHref, ""
	}
	s.MockPulpClient.On("FindRpmPublicationByVersion", versionHref).Return(nil, nil).Once()
	s.MockPulpClient.On("CreateRpmPublication", versionHref).Return(&publishTaskHref, nil).Once()
	status := pulp_client.COMPLETED
	task := zest.TaskResponse{
		PulpHref:         &publishTaskHref,
		State:            &status,
		CreatedResources: []string{pubHref},
	}
	s.MockPulpClient.On("PollTask", publishTaskHref).Return(&task, nil).Once()
	return pubHref, publishTaskHref
}

func (s *SnapshotSuite) mockSync(taskHref string, producesVersion bool) (*string, string) {
	var versionHref *string
	var createdResources []string
	if producesVersion {
		versionHref = pointy.String("/pulp/api/v3/repositories/rpm/rpm/" + uuid.NewString() + "/versions/1/")
		createdResources = append(createdResources, *versionHref)
	}
	status := pulp_client.COMPLETED
	task := zest.TaskResponse{
		PulpHref:         &taskHref,
		State:            &status,
		CreatedResources: createdResources,
	}
	s.MockPulpClient.On("PollTask", taskHref).Return(&task, nil).Once()

	return versionHref, taskHref
}

func (s *SnapshotSuite) mockRepoCreate(repoConfig api.RepositoryResponse, remoteHref string, existingRepo bool) zest.RpmRpmRepositoryResponse {
	repoResp := zest.RpmRpmRepositoryResponse{PulpHref: pointy.String("repoHref")}
	if existingRepo {
		s.MockPulpClient.On("GetRpmRepositoryByName", repoConfig.UUID).Return(&repoResp, nil).Once()
	} else {
		s.MockPulpClient.On("GetRpmRepositoryByName", repoConfig.UUID).Return(nil, nil).Once()
		s.MockPulpClient.On("CreateRpmRepository", repoConfig.UUID, &remoteHref).Return(&repoResp, nil).Once()
	}

	return repoResp
}

func (s *SnapshotSuite) mockRemoteCreate(repoConfig api.RepositoryResponse, existingRemote bool) string {
	remoteResp := zest.RpmRpmRemoteResponse{PulpHref: pointy.String("remoteHref"), Url: repoConfig.URL}
	if existingRemote {
		s.MockPulpClient.On("GetRpmRemoteByName", repoConfig.UUID).Return(&remoteResp, nil).Once()
	} else {
		s.MockPulpClient.On("GetRpmRemoteByName", repoConfig.UUID).Return(nil, nil).Once()
		s.MockPulpClient.On("CreateRpmRemote", repoConfig.UUID, repoConfig.URL).Return(&remoteResp, nil).Once()
	}
	return *remoteResp.PulpHref
}
