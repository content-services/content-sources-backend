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
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	zest "github.com/content-services/zest/release/v2024"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
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
	config.Get().Clients.Pulp.CustomRepoContentGuards = false
}

func (s *SnapshotSuite) TestSnapshotFull() {
	snapshotId := "abacadaba"
	repoUuid := uuid.New()
	ctx := context.Background()

	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}
	task := models.TaskInfo{
		Id:         uuid.UUID{},
		OrgId:      repoConfig.OrgID,
		ObjectUUID: repoUuid,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
	}

	domainName := "myDomain"
	s.mockDaoRegistry.RepositoryConfig.On("FetchByRepoUuid", ctx, repoConfig.OrgID, repo.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.RepositoryConfig.On("Fetch", ctx, repoConfig.OrgID, repoConfig.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.Repository.On("FetchForUrl", ctx, repoConfig.URL).Return(repo, nil)

	remoteHref := s.mockRemoteCreate(ctx, repoConfig, false)
	repoResp := s.mockRepoCreate(ctx, repoConfig, remoteHref, false)

	taskHref := "SyncTaskHref"
	s.MockPulpClient.On("SyncRpmRepository", ctx, *(repoResp.PulpHref), &remoteHref).Return(taskHref, nil)
	s.MockPulpClient.On("LookupOrCreateDomain", ctx, domainName).Return("found", nil)
	s.MockPulpClient.On("UpdateDomainIfNeeded", ctx, domainName).Return(nil)

	versionHref, syncTask := s.mockSync(ctx, taskHref, true)
	pubHref, pubTask := s.mockPublish(ctx, *versionHref, false)
	distHref, distTask := s.mockCreateDist(ctx, pubHref)

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
	counts := zest.ContentSummaryResponse{
		Present: map[string]map[string]interface{}{},
		Added:   map[string]map[string]interface{}{},
		Removed: map[string]map[string]interface{}{},
	}
	rpmVersion := zest.RepositoryVersionResponse{
		PulpHref:       versionHref,
		ContentSummary: &counts,
	}
	s.MockPulpClient.On("GetRpmRepositoryVersion", ctx, *versionHref).Return(&rpmVersion, nil)
	current, added, removed := ContentSummaryToContentCounts(&counts)
	distPath := fmt.Sprintf("%s/%s", repoConfig.UUID, snapshotId)
	expectedSnap := models.Snapshot{
		VersionHref:                 *versionHref,
		PublicationHref:             pubHref,
		DistributionHref:            distHref,
		DistributionPath:            distPath,
		RepositoryConfigurationUUID: repoConfig.UUID,
		ContentCounts:               current,
		AddedCounts:                 added,
		RemovedCounts:               removed,
		RepositoryPath:              fmt.Sprintf("%v/%v", domainName, distPath),
	}

	payload := payloads.SnapshotPayload{
		SnapshotIdent: &snapshotId,
	}

	snap := SnapshotRepository{
		orgId:          repoConfig.OrgID,
		domainName:     domainName,
		repositoryUUID: repoUuid,
		daoReg:         s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient:     &s.MockPulpClient,
		payload:        &payload,
		task:           &task,
		queue:          &s.Queue,
		ctx:            ctx,
		logger:         &log.Logger,
	}

	s.mockDaoRegistry.Snapshot.On("Create", &expectedSnap).Return(nil).Once()

	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}

func (s *SnapshotSuite) TestSnapshotResync() {
	ctx := context.Background()
	repoUuid := uuid.New()
	domainName := "myDomain"
	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}

	s.mockDaoRegistry.RepositoryConfig.On("FetchByRepoUuid", ctx, repoConfig.OrgID, repo.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.RepositoryConfig.On("Fetch", ctx, repoConfig.OrgID, repoConfig.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.Repository.On("FetchForUrl", ctx, repoConfig.URL).Return(repo, nil)

	remoteHref := s.mockRemoteCreate(ctx, repoConfig, true)
	repoResp := s.mockRepoCreate(ctx, repoConfig, remoteHref, true)

	taskHref := "SyncTaskHref"
	s.MockPulpClient.On("LookupOrCreateDomain", ctx, domainName).Return("found", nil)
	s.MockPulpClient.On("UpdateDomainIfNeeded", ctx, domainName).Return(nil)
	s.MockPulpClient.On("SyncRpmRepository", ctx, *(repoResp.PulpHref), &remoteHref).Return(taskHref, nil)

	_, syncTask := s.mockSync(ctx, taskHref, false)

	task := models.TaskInfo{
		Id:         uuid.UUID{},
		OrgId:      repoConfig.OrgID,
		ObjectUUID: repoUuid,
		ObjectType: utils.Ptr(config.ObjectTypeRepository)}

	s.MockQueue.On("UpdatePayload", &task, payloads.SnapshotPayload{
		SyncTaskHref: &syncTask,
	}).Return(&task, nil)

	snap := SnapshotRepository{
		orgId:          repoConfig.OrgID,
		domainName:     domainName,
		repositoryUUID: repoUuid,
		daoReg:         s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient:     &s.MockPulpClient,
		payload:        &payloads.SnapshotPayload{},
		task:           &task,
		queue:          &s.Queue,
		ctx:            ctx,
		logger:         &log.Logger,
	}
	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}

func (s *SnapshotSuite) TestSnapshotResyncWithOrphanVersion() {
	ctx := context.Background()
	repoUuid := uuid.New()
	domainName := "myDomain"
	snapshotId := "testResyncWithOrphanVersion"
	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}
	existingVersionHref := "existing_Href"

	s.mockDaoRegistry.RepositoryConfig.On("FetchByRepoUuid", ctx, repoConfig.OrgID, repo.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.RepositoryConfig.On("Fetch", ctx, repoConfig.OrgID, repoConfig.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.Repository.On("FetchForUrl", ctx, repoConfig.URL).Return(repo, nil)
	s.mockDaoRegistry.Snapshot.On("FetchSnapshotByVersionHref", ctx, repoConfig.UUID, existingVersionHref).Return(nil, nil)
	remoteHref := s.mockRemoteCreate(ctx, repoConfig, true)
	repoResp := s.mockRepoCreateWithLatestVersion(ctx, repoConfig, existingVersionHref)

	taskHref := "SyncTaskHref"
	s.MockPulpClient.On("LookupOrCreateDomain", ctx, domainName).Return("found", nil)
	s.MockPulpClient.On("UpdateDomainIfNeeded", ctx, domainName).Return(nil)
	s.MockPulpClient.On("SyncRpmRepository", ctx, *(repoResp.PulpHref), &remoteHref).Return(taskHref, nil)

	_, syncTask := s.mockSync(ctx, taskHref, false)

	task := models.TaskInfo{
		Id:         uuid.UUID{},
		OrgId:      repoConfig.OrgID,
		ObjectUUID: repoUuid,
		ObjectType: utils.Ptr(config.ObjectTypeRepository)}

	pubHref, pubTask := s.mockPublish(ctx, existingVersionHref, false)
	distHref, distTask := s.mockCreateDist(ctx, pubHref)

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
	counts := zest.ContentSummaryResponse{
		Present: map[string]map[string]interface{}{},
		Added:   map[string]map[string]interface{}{},
		Removed: map[string]map[string]interface{}{},
	}
	rpmVersion := zest.RepositoryVersionResponse{
		PulpHref:       &existingVersionHref,
		ContentSummary: &counts,
	}
	s.MockPulpClient.On("GetRpmRepositoryVersion", ctx, existingVersionHref).Return(&rpmVersion, nil)
	current, added, removed := ContentSummaryToContentCounts(&counts)
	distPath := fmt.Sprintf("%s/%s", repoConfig.UUID, snapshotId)
	expectedSnap := models.Snapshot{
		VersionHref:                 existingVersionHref,
		PublicationHref:             pubHref,
		DistributionHref:            distHref,
		DistributionPath:            distPath,
		RepositoryConfigurationUUID: repoConfig.UUID,
		ContentCounts:               current,
		AddedCounts:                 added,
		RemovedCounts:               removed,
		RepositoryPath:              fmt.Sprintf("%v/%v", domainName, distPath),
	}

	payload := payloads.SnapshotPayload{
		SnapshotIdent: &snapshotId,
	}

	snap := SnapshotRepository{
		orgId:          repoConfig.OrgID,
		domainName:     domainName,
		repositoryUUID: repoUuid,
		daoReg:         s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient:     &s.MockPulpClient,
		payload:        &payload,
		task:           &task,
		queue:          &s.Queue,
		ctx:            ctx,
		logger:         &log.Logger,
	}

	s.mockDaoRegistry.Snapshot.On("Create", ctx, &expectedSnap).Return(nil).Once()

	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}

// TestSnapshotRestartAfterSync this test simulates what happens if a worker is restarted after the sync has started, and a
// sync task is already present in the payload
func (s *SnapshotSuite) TestSnapshotRestartAfterSync() {
	ctx := context.Background()
	snapshotId := "abacadaba"
	repoUuid := uuid.New()
	domainName := "MyComain"

	repo := dao.Repository{UUID: repoUuid.String(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}
	task := models.TaskInfo{
		Id:         uuid.UUID{},
		OrgId:      repoConfig.OrgID,
		ObjectUUID: repoUuid,
		ObjectType: utils.Ptr(config.ObjectTypeRepository)}

	s.mockDaoRegistry.RepositoryConfig.On("FetchByRepoUuid", ctx, repoConfig.OrgID, repo.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.RepositoryConfig.On("Fetch", ctx, repoConfig.OrgID, repoConfig.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.Repository.On("FetchForUrl", ctx, repoConfig.URL).Return(repo, nil)
	s.MockPulpClient.On("LookupOrCreateDomain", ctx, domainName).Return("found", nil)
	s.MockPulpClient.On("UpdateDomainIfNeeded", ctx, domainName).Return(nil)

	remoteHref := s.mockRemoteCreate(ctx, repoConfig, false)
	s.mockRepoCreate(ctx, repoConfig, remoteHref, false)
	versionHref := "some/version"

	syncTaskHref := "SyncTaskHref"
	syncTask := zest.TaskResponse{
		PulpHref:         utils.Ptr(syncTaskHref),
		PulpCreated:      nil,
		State:            utils.Ptr("completed"),
		CreatedResources: []string{versionHref},
	}
	s.MockPulpClient.On("PollTask", syncTaskHref).Return(&syncTask, nil)

	pubHref, pubTask := s.mockPublish(ctx, versionHref, false)
	distHref, distTask := s.mockCreateDist(ctx, pubHref)

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
	counts := zest.ContentSummaryResponse{
		Present: map[string]map[string]interface{}{},
		Added:   map[string]map[string]interface{}{},
		Removed: map[string]map[string]interface{}{},
	}
	rpmVersion := zest.RepositoryVersionResponse{
		PulpHref:       &versionHref,
		ContentSummary: &counts,
	}

	s.MockPulpClient.On("GetRpmRepositoryVersion", versionHref).Return(&rpmVersion, nil)

	current, added, removed := ContentSummaryToContentCounts(&counts)
	expectedSnap := models.Snapshot{
		VersionHref:                 versionHref,
		PublicationHref:             pubHref,
		DistributionHref:            distHref,
		DistributionPath:            fmt.Sprintf("%s/%s", repoConfig.UUID, snapshotId),
		RepositoryConfigurationUUID: repoConfig.UUID,
		ContentCounts:               current,
		AddedCounts:                 added,
		RemovedCounts:               removed,
	}

	payload := payloads.SnapshotPayload{
		SnapshotIdent: &snapshotId,
		SyncTaskHref:  &syncTaskHref,
	}

	snap := SnapshotRepository{
		orgId:          repoConfig.OrgID,
		domainName:     domainName,
		repositoryUUID: repoUuid,
		daoReg:         s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient:     &s.MockPulpClient,
		payload:        &payload,
		task:           &task,
		queue:          &s.Queue,
		ctx:            ctx,
		logger:         &log.Logger,
	}

	s.mockDaoRegistry.Snapshot.On("Create", &expectedSnap).Return(nil).Once()

	snapErr := snap.Run()
	assert.NoError(s.T(), snapErr)
}

func (s *SnapshotSuite) mockCreateDist(ctx context.Context, pubHref string) (string, string) {
	distPath := mock.AnythingOfType("string")
	var guardPath *string
	distHref := "/pulp/DNAME/api/v3/distributions/rpm/rpm/" + uuid.NewString() + "/"
	distTaskhref := "distTaskHref"

	s.MockPulpClient.On("FindDistributionByPath", ctx, distPath).Return(nil, nil)
	s.MockPulpClient.On("CreateRpmDistribution", ctx, pubHref, mock.AnythingOfType("string"), distPath, guardPath).Return(&distTaskhref, nil).Twice()
	status := pulp_client.COMPLETED
	task := zest.TaskResponse{
		PulpHref:         &distTaskhref,
		State:            &status,
		CreatedResources: []string{distHref},
	}
	s.MockPulpClient.On("PollTask", ctx, distTaskhref).Return(&task, nil).Twice()
	return distHref, distTaskhref
}

func (s *SnapshotSuite) mockPublish(ctx context.Context, versionHref string, existing bool) (string, string) {
	publishTaskHref := "SyncTaskHref"
	pubHref := "/pulp/DNAME/api/v3/publications/rpm/rpm/" + uuid.NewString() + "/"

	if existing {
		resp := zest.RpmRpmPublicationResponse{PulpHref: &pubHref}
		s.MockPulpClient.On("FindRpmPublicationByVersion", ctx, versionHref).Return(&resp, nil).Once()
		return pubHref, ""
	}
	s.MockPulpClient.On("FindRpmPublicationByVersion", ctx, versionHref).Return(nil, nil).Once()
	s.MockPulpClient.On("CreateRpmPublication", ctx, versionHref).Return(&publishTaskHref, nil).Once()
	status := pulp_client.COMPLETED
	task := zest.TaskResponse{
		PulpHref:         &publishTaskHref,
		State:            &status,
		CreatedResources: []string{pubHref},
	}
	s.MockPulpClient.On("PollTask", ctx, publishTaskHref).Return(&task, nil).Once()
	return pubHref, publishTaskHref
}

func (s *SnapshotSuite) mockSync(ctx context.Context, taskHref string, producesVersion bool) (*string, string) {
	var versionHref *string
	var createdResources []string
	if producesVersion {
		versionHref = utils.Ptr("/pulp/api/v3/repositories/rpm/rpm/" + uuid.NewString() + "/versions/1/")
		createdResources = append(createdResources, *versionHref)
	}
	status := pulp_client.COMPLETED
	task := zest.TaskResponse{
		PulpHref:         &taskHref,
		State:            &status,
		CreatedResources: createdResources,
	}
	s.MockPulpClient.On("PollTask", ctx, taskHref).Return(&task, nil).Once()

	return versionHref, taskHref
}

func (s *SnapshotSuite) mockRepoCreateWithLatestVersion(ctx context.Context, repoConfig api.RepositoryResponse, existingVersion string) zest.RpmRpmRepositoryResponse {
	repoResp := zest.RpmRpmRepositoryResponse{PulpHref: utils.Ptr("repoHref"), LatestVersionHref: &existingVersion}
	s.MockPulpClient.On("GetRpmRepositoryByName", ctx, repoConfig.UUID).Return(&repoResp, nil)
	return repoResp
}

func (s *SnapshotSuite) mockRepoCreate(ctx context.Context, repoConfig api.RepositoryResponse, remoteHref string, existingRepo bool) zest.RpmRpmRepositoryResponse {
	repoResp := zest.RpmRpmRepositoryResponse{PulpHref: utils.Ptr("repoHref")}
	if existingRepo {
		s.MockPulpClient.On("GetRpmRepositoryByName", ctx, repoConfig.UUID).Return(&repoResp, nil)
	} else {
		s.MockPulpClient.On("GetRpmRepositoryByName", ctx, repoConfig.UUID).Return(nil, nil)
		s.MockPulpClient.On("CreateRpmRepository", ctx, repoConfig.UUID, &remoteHref).Return(&repoResp, nil).Once()
	}

	return repoResp
}

func (s *SnapshotSuite) mockRemoteCreate(ctx context.Context, repoConfig api.RepositoryResponse, existingRemote bool) string {
	remoteResp := zest.RpmRpmRemoteResponse{PulpHref: utils.Ptr("remoteHref"), Url: repoConfig.URL}
	var nilString *string
	if existingRemote {
		s.MockPulpClient.On("GetRpmRemoteByName", ctx, repoConfig.UUID).Return(&remoteResp, nil).Once()
		s.MockPulpClient.On("UpdateRpmRemote", ctx, "remoteHref", repoConfig.URL, nilString, nilString, nilString).Return("someTaskHref", nil).Once()
	} else {
		s.MockPulpClient.On("GetRpmRemoteByName", ctx, repoConfig.UUID).Return(nil, nil).Once()
		s.MockPulpClient.On("CreateRpmRemote", ctx, repoConfig.UUID, repoConfig.URL, nilString, nilString, nilString).Return(&remoteResp, nil).Once()
	}
	return *remoteResp.PulpHref
}
