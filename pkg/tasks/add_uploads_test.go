package tasks

import (
	"context"
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type AddUploadsSuite struct {
	suite.Suite
	mockDaoRegistry *dao.MockDaoRegistry
	MockPulpClient  pulp_client.MockPulpClient
	MockQueue       queue.MockQueue
	Queue           queue.Queue
}

func TestAddUploadsSuite(t *testing.T) {
	suite.Run(t, new(AddUploadsSuite))
}

func (s *AddUploadsSuite) SetupTest() {
	s.mockDaoRegistry = dao.GetMockDaoRegistry(s.T())
	s.MockPulpClient = *pulp_client.NewMockPulpClient(s.T())
	s.MockQueue = *queue.NewMockQueue(s.T())
	s.Queue = &s.MockQueue
	config.Get().Clients.Pulp.RepoContentGuards = false
}

func (s *AddUploadsSuite) TestAddUploadsNoNewVersion() {
	ctx := context.Background()
	domainName := "myDomain"
	artifactSha := "abc123"
	artifactHref := "/pulp/artifact/abc123/"
	packageHref := "/pulp/content/rpm/packages/abc123/"
	repoHref := "repoHref"

	repoConfig := api.RepositoryResponse{
		OrgID:          "OrgId",
		UUID:           uuid.NewString(),
		RepositoryUUID: uuid.NewString(),
		Origin:         config.OriginUpload,
		Snapshot:       true,
	}

	s.MockPulpClient.On("LookupPackage", ctx, artifactSha).Return(&packageHref, nil)
	repoResp := zest.RpmRpmRepositoryResponse{PulpHref: &repoHref, LatestVersionHref: utils.Ptr("existing_Href")}
	s.MockPulpClient.On("GetRpmRepositoryByName", ctx, repoConfig.UUID).Return(&repoResp, nil)

	modifyTaskHref := "modifyTaskHref"
	s.MockPulpClient.On("ModifyRpmRepositoryContent", ctx, repoHref, []string{packageHref}, []string{}).Return(modifyTaskHref, nil)
	status := pulp_client.COMPLETED
	modifyTask := zest.TaskResponse{
		PulpHref:         &modifyTaskHref,
		State:            &status,
		CreatedResources: []string{},
	}
	s.MockPulpClient.On("PollTask", ctx, modifyTaskHref).Return(&modifyTask, nil)
	s.mockDaoRegistry.Snapshot.On("FetchSnapshotByVersionHref", ctx, repoConfig.UUID, "existing_Href").Return(&api.SnapshotResponse{UUID: uuid.NewString()}, nil)

	task := models.TaskInfo{
		Id:    uuid.UUID{},
		OrgId: repoConfig.OrgID,
	}
	payload := AddUploadsPayload{
		RepositoryConfigUUID: repoConfig.UUID,
		Artifacts:            []api.Artifact{{Sha256: artifactSha, Href: artifactHref}},
	}
	ur := AddUploads{
		orgID:      repoConfig.OrgID,
		domainName: domainName,
		ctx:        ctx,
		payload:    &payload,
		task:       &task,
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		repo:       repoConfig,
		pulpClient: &s.MockPulpClient,
		queue:      &s.Queue,
		logger:     &log.Logger,
	}

	err := ur.Run()
	assert.NoError(s.T(), err)
	assert.Nil(s.T(), ur.payload.VersionHref)
}

func (s *AddUploadsSuite) TestAddUploadsWithOrphanVersion() {
	ctx := context.Background()
	domainName := "myDomain"
	snapshotId := "testAddUploadsWithOrphanVersion"
	artifactSha := "abc123"
	artifactHref := "/pulp/artifact/abc123/"
	packageHref := "/pulp/content/rpm/packages/abc123/"
	repoHref := "repoHref"
	existingVersionHref := "existing_Href"

	repoConfig := api.RepositoryResponse{
		OrgID:          "OrgId",
		UUID:           uuid.NewString(),
		RepositoryUUID: uuid.NewString(),
		Origin:         config.OriginUpload,
		Snapshot:       true,
	}

	s.MockPulpClient.On("LookupPackage", ctx, artifactSha).Return(&packageHref, nil)
	repoResp := zest.RpmRpmRepositoryResponse{PulpHref: &repoHref, LatestVersionHref: &existingVersionHref}
	s.MockPulpClient.On("GetRpmRepositoryByName", ctx, repoConfig.UUID).Return(&repoResp, nil)

	modifyTaskHref := "modifyTaskHref"
	s.MockPulpClient.On("ModifyRpmRepositoryContent", ctx, repoHref, []string{packageHref}, []string{}).Return(modifyTaskHref, nil)
	status := pulp_client.COMPLETED
	modifyTask := zest.TaskResponse{
		PulpHref:         &modifyTaskHref,
		State:            &status,
		CreatedResources: []string{},
	}
	s.MockPulpClient.On("PollTask", ctx, modifyTaskHref).Return(&modifyTask, nil)
	s.mockDaoRegistry.Snapshot.On("FetchSnapshotByVersionHref", ctx, repoConfig.UUID, existingVersionHref).Return(nil, nil)

	task := models.TaskInfo{
		Id:    uuid.UUID{},
		OrgId: repoConfig.OrgID,
	}

	pubHref, _ := s.mockPublish(ctx, existingVersionHref)
	distHref, _ := s.mockCreateDist(ctx, pubHref)

	s.MockQueue.On("UpdatePayload", &task, mock.Anything).Return(&task, nil)

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
	current, added, removed := models.ContentSummaryToContentCounts(&counts)
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
	s.mockDaoRegistry.Snapshot.On("Create", ctx, &expectedSnap).Return(nil).Once()

	s.MockPulpClient.On("ListVersionAllPackages", ctx, existingVersionHref).Return([]zest.RpmPackageResponse{}, nil)
	s.mockDaoRegistry.Rpm.On("InsertForRepository", ctx, repoConfig.RepositoryUUID, mock.Anything).Return(int64(0), nil)
	s.mockDaoRegistry.Repository.On("Update", ctx, mock.AnythingOfType("dao.RepositoryUpdate")).Return(nil)

	payload := AddUploadsPayload{
		RepositoryConfigUUID: repoConfig.UUID,
		Artifacts:            []api.Artifact{{Sha256: artifactSha, Href: artifactHref}},
		SnapshotIdent:        &snapshotId,
	}
	ur := AddUploads{
		orgID:      repoConfig.OrgID,
		domainName: domainName,
		ctx:        ctx,
		payload:    &payload,
		task:       &task,
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		repo:       repoConfig,
		pulpClient: &s.MockPulpClient,
		queue:      &s.Queue,
		logger:     &log.Logger,
	}

	err := ur.Run()
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), existingVersionHref, *ur.payload.VersionHref)
}

func (s *AddUploadsSuite) mockCreateDist(ctx context.Context, pubHref string) (string, string) {
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

func (s *AddUploadsSuite) mockPublish(ctx context.Context, versionHref string) (string, string) {
	publishTaskHref := "publishTaskHref"
	pubHref := "/pulp/DNAME/api/v3/publications/rpm/rpm/" + uuid.NewString() + "/"

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
