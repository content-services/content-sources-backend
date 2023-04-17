package tasks

import (
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
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
}

func TestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(SnapshotSuite))
}

func (s *SnapshotSuite) SetupTest() {
	s.mockDaoRegistry = dao.GetMockDaoRegistry(s.T())
}

func (s *SnapshotSuite) TestSnapshotFull() {
	repo := dao.Repository{UUID: uuid.NewString(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}

	s.mockDaoRegistry.RepositoryConfig.On("Fetch", repoConfig.OrgID, repoConfig.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.Repository.On("FetchForUrl", repoConfig.URL).Return(repo, nil)

	remoteHref := s.mockRemoteCreate(repoConfig, false)
	repoResp := s.mockRepoCreate(repoConfig, remoteHref, false)

	taskHref := "syncTaskHref"
	s.MockPulpClient.On("SyncRpmRepository", *(repoResp.PulpHref), (*string)(nil)).Return(taskHref, nil)

	versionHref := s.mockSync(taskHref, true)
	assert.NotNil(s.T(), versionHref)

	pubHref := s.mockPublish(*versionHref, false)

	distHref := s.mockCreateDist(pubHref)

	// Lookup the version
	counts := zest.RepositoryVersionResponseContentSummary{
		Present: map[string]map[string]interface{}{},
	}
	rpmVersion := zest.RepositoryVersionResponse{
		PulpHref:       versionHref,
		ContentSummary: &counts,
	}
	s.MockPulpClient.On("GetRpmRepositoryVersion", *versionHref).Return(&rpmVersion, nil)

	snapshotId := "abacadaba"
	expectedSnap := dao.Snapshot{
		VersionHref:      *versionHref,
		PublicationHref:  pubHref,
		DistributionHref: distHref,
		DistributionPath: fmt.Sprintf("%s/%s", repoConfig.UUID, snapshotId),
		OrgId:            repoConfig.OrgID,
		RepositoryUUID:   repo.UUID,
		ContentCounts:    ContentSummaryToContentCounts(&counts),
	}
	s.mockDaoRegistry.Snapshot.On("Create", &expectedSnap).Return(nil).Once()

	snapErr := SnapshotRepository(SnapshotOptions{
		OrgId:          repoConfig.OrgID,
		RepoConfigUuid: repoConfig.UUID,
		DaoRegistry:    s.mockDaoRegistry.ToDaoRegistry(),
		PulpClient:     &s.MockPulpClient,
		snapshotIdent:  &snapshotId,
	})
	assert.NoError(s.T(), snapErr)
}

func (s *SnapshotSuite) TestSnapshotResync() {
	repo := dao.Repository{UUID: uuid.NewString(), URL: "http://random.example.com/thing"}
	repoConfig := api.RepositoryResponse{OrgID: "OrgId", UUID: uuid.NewString(), URL: repo.URL}

	s.mockDaoRegistry.RepositoryConfig.On("Fetch", repoConfig.OrgID, repoConfig.UUID).Return(repoConfig, nil)
	s.mockDaoRegistry.Repository.On("FetchForUrl", repoConfig.URL).Return(repo, nil)

	remoteHref := s.mockRemoteCreate(repoConfig, true)
	repoResp := s.mockRepoCreate(repoConfig, remoteHref, true)

	taskHref := "syncTaskHref"
	s.MockPulpClient.On("SyncRpmRepository", *(repoResp.PulpHref), (*string)(nil)).Return(taskHref, nil)

	s.mockSync(taskHref, false)

	snapErr := SnapshotRepository(SnapshotOptions{
		OrgId:          repoConfig.OrgID,
		RepoConfigUuid: repoConfig.UUID,
		DaoRegistry:    s.mockDaoRegistry.ToDaoRegistry(),
		PulpClient:     &s.MockPulpClient,
	})
	assert.NoError(s.T(), snapErr)
}

func (s *SnapshotSuite) mockCreateDist(pubHref string) string {
	distPath := mock.AnythingOfType("string")
	distHref := "/pulp/api/v3/distributions/rpm/rpm/" + uuid.NewString() + "/"
	distTaskhref := "distTaskHref"

	s.MockPulpClient.On("CreateRpmDistribution", pubHref, mock.AnythingOfType("string"), distPath).Return(&distTaskhref, nil).Once()
	status := pulp_client.COMPLETED
	task := zest.TaskResponse{
		PulpHref:         &distTaskhref,
		State:            &status,
		CreatedResources: []string{distHref},
	}
	s.MockPulpClient.On("PollTask", distTaskhref).Return(&task, nil).Once()
	return distHref
}

func (s *SnapshotSuite) mockPublish(versionHref string, existing bool) string {
	publishTaskHref := "syncTaskHref"
	pubHref := "/pulp/api/v3/publications/rpm/rpm/" + uuid.NewString() + "/"

	if existing {
		resp := zest.RpmRpmPublicationResponse{PulpHref: &pubHref}
		s.MockPulpClient.On("FindRpmPublicationByVersion", versionHref).Return(&resp, nil).Once()
		return pubHref
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
	return pubHref
}

func (s *SnapshotSuite) mockSync(taskHref string, producesVersion bool) *string {
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
	return versionHref
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
