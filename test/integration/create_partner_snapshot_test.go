package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	uuid2 "github.com/google/uuid"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CreatePartnerSnapshotSuite struct {
	Suite
	dao            *dao.DaoRegistry
	ctx            context.Context
	coprPulpClient pulp_client.PulpClient
}

func (s *CreatePartnerSnapshotSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ctx = context.Background()
	s.dao = dao.GetDaoRegistry(db.DB)

	// Force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"

	// Force content guard setup
	config.Get().Clients.Pulp.RepoContentGuards = true
	config.Get().Clients.Pulp.GuardSubjectDn = "warlin.door"

	// Initialize Copr domain pulp client
	coprDomain := config.Get().Clients.Pulp.CoprDomain
	s.coprPulpClient = pulp_client.GetPulpClientWithDomain(coprDomain)
}

func TestCreatePartnerSnapshotSuite(t *testing.T) {
	suite.Run(t, new(CreatePartnerSnapshotSuite))
}

// createPublicationInCoprDomain creates a repository and publication in the cs-partner domain
// This simulates what Copr would have created
func (s *CreatePartnerSnapshotSuite) createPublicationInCoprDomain() (publicationHref string, versionHref string) {
	// Ensure the domain exists
	_, err := s.coprPulpClient.LookupOrCreateDomain(s.ctx, config.Get().Clients.Pulp.CoprDomain)
	require.NoError(s.T(), err)

	// Create a repository in the cs-partner domain
	repoName := "copr-test-repo-" + uuid2.NewString()
	repoResp, err := s.coprPulpClient.CreateRpmRepository(s.ctx, repoName, nil)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), repoResp.PulpHref)

	// Create a publication from the repository (even if empty)
	pubTaskHref, err := s.coprPulpClient.CreateRpmPublication(s.ctx, *repoResp.LatestVersionHref)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), pubTaskHref)

	// Wait for publication task to complete and extract publication href
	pubTask, err := s.coprPulpClient.PollTask(s.ctx, *pubTaskHref)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), pubTask)

	pubHrefPtr := pulp_client.SelectPublicationHref(pubTask)
	require.NotNil(s.T(), pubHrefPtr, "Publication task should create a publication")

	publicationHref = *pubHrefPtr
	versionHref = *repoResp.LatestVersionHref

	return publicationHref, versionHref
}

func (s *CreatePartnerSnapshotSuite) TestCreatePartnerSnapshot_NewRepository() {
	accountId := uuid2.NewString()
	orgId := accountId

	// Create a publication in the cs-partner domain (simulating what Copr would have created)
	publicationHref, versionHref := s.createPublicationInCoprDomain()

	// Now test create-partner-snapshot with a new repository name
	partnerRepoName := "partner-repo-" + uuid2.NewString()
	coprRepoHref := "/pulp/api/v3/repositories/rpm/rpm/" + uuid2.NewString() + "/"
	payload := payloads.CreatePartnerSnapshotPayload{
		PublicationHref:    publicationHref,
		RepositoryName:     partnerRepoName,
		RepositoryVersion:  "9",
		RepositoryArch:     "x86_64",
		CoprProjectID:      "test-project-123",
		CoprRepositoryHref: coprRepoHref,
		CoprBuildID:        "test-build-456",
	}

	task := queue.Task{
		Typename: config.CreatePartnerSnapshotTask,
		Payload:  payload,
		OrgId:    orgId,
	}

	taskUUID, err := s.taskClient.Enqueue(task)
	require.NoError(s.T(), err)

	s.WaitOnTask(taskUUID)

	// Verify the repository configuration was created with Copr metadata
	repoConfig, err := s.dao.RepositoryConfig.InternalOnly_FetchRepoConfigByCoprRepositoryHref(s.ctx, orgId, coprRepoHref)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), partnerRepoName, repoConfig.Name)
	assert.Equal(s.T(), config.OriginUpload, repoConfig.Origin)
	assert.True(s.T(), repoConfig.Snapshot)
	assert.Contains(s.T(), repoConfig.DistributionVersions, "9")
	assert.Equal(s.T(), "x86_64", repoConfig.DistributionArch)

	// Verify Copr metadata is stored
	var repoConfigModel models.RepositoryConfiguration
	err = db.DB.Where("uuid = ?", repoConfig.UUID).First(&repoConfigModel).Error
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "test-project-123", repoConfigModel.CoprProjectID)
	assert.Equal(s.T(), coprRepoHref, repoConfigModel.CoprRepositoryHref)

	// Verify snapshot was created with Copr build ID
	snaps, _, err := s.dao.Snapshot.List(s.ctx, orgId, repoConfig.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), snaps.Data)
	assert.Equal(s.T(), publicationHref, snaps.Data[0].PublicationHref)
	assert.False(s.T(), snaps.Data[0].Published) // Should start unpublished

	// Verify Copr build ID is stored
	var createdSnapModel models.Snapshot
	err = db.DB.Where("uuid = ?", snaps.Data[0].UUID).First(&createdSnapModel).Error
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "test-build-456", createdSnapModel.CoprBuildID)

	// Verify distribution was created in Pulp (in the cs-partner domain)
	// Get the snapshot model to access distribution details
	var partnerSnapModel models.Snapshot
	err = db.DB.Where("uuid = ?", snaps.Data[0].UUID).First(&partnerSnapModel).Error
	require.NoError(s.T(), err)

	// Verify distribution exists in Pulp by looking it up by path
	distribution, err := s.coprPulpClient.FindDistributionByPath(s.ctx, partnerSnapModel.DistributionPath)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), distribution, "Distribution should exist in Pulp")
	assert.Equal(s.T(), partnerSnapModel.DistributionPath, distribution.BasePath, "Distribution base path should match")
	pubHref := distribution.Publication.Get()
	assert.NotNil(s.T(), pubHref, "Distribution publication should not be nil")
	assert.Equal(s.T(), publicationHref, *pubHref, "Distribution should point to the correct publication")

	// Verify distribution is accessible via HTTP
	distPath := fmt.Sprintf("%v/repodata/repomd.xml", snaps.Data[0].URL)
	err = s.getRequest(distPath, identity.Identity{OrgID: orgId, Internal: identity.Internal{OrgID: orgId}}, 200)
	assert.NoError(s.T(), err)

	// Verify publication exists in Pulp (in cs-partner domain)
	publication, err := s.coprPulpClient.FindRpmPublicationByVersion(s.ctx, versionHref)
	require.NoError(s.T(), err)
	assert.NotNil(s.T(), publication)
}

func (s *CreatePartnerSnapshotSuite) TestCreatePartnerSnapshot_ExistingRepository() {
	accountId := uuid2.NewString()
	orgId := accountId

	// Create a publication in the cs-partner domain (simulating what Copr would have created)
	publicationHref, _ := s.createPublicationInCoprDomain()

	// Create a repository configuration first
	partnerRepoName := "existing-partner-repo-" + uuid2.NewString()
	existingRepo, err := s.dao.RepositoryConfig.Create(s.ctx, api.RepositoryRequest{
		Name:                 utils.Ptr(partnerRepoName),
		AccountID:            utils.Ptr(accountId),
		OrgID:                utils.Ptr(orgId),
		Snapshot:             utils.Ptr(true),
		Origin:               utils.Ptr(config.OriginUpload),
		DistributionVersions: utils.Ptr([]string{"9"}),
		DistributionArch:     utils.Ptr("x86_64"),
	})
	require.NoError(s.T(), err)

	// Set up Copr metadata on the existing repo
	coprRepoHref := "/pulp/api/v3/repositories/rpm/rpm/" + uuid2.NewString() + "/"
	err = s.dao.RepositoryConfig.InternalOnly_UpdateCoprMetadata(s.ctx, existingRepo.UUID, "existing-project-789", coprRepoHref)
	require.NoError(s.T(), err)

	// Now run create-partner-snapshot with the Copr repository href
	payload := payloads.CreatePartnerSnapshotPayload{
		PublicationHref:    publicationHref,
		RepositoryName:     partnerRepoName,
		RepositoryVersion:  "9",
		RepositoryArch:     "x86_64",
		CoprProjectID:      "existing-project-789",
		CoprRepositoryHref: coprRepoHref,
		CoprBuildID:        "existing-build-999",
	}

	task := queue.Task{
		Typename: config.CreatePartnerSnapshotTask,
		Payload:  payload,
		OrgId:    orgId,
	}

	taskUUID, err := s.taskClient.Enqueue(task)
	require.NoError(s.T(), err)

	s.WaitOnTask(taskUUID)

	// Verify the existing repository was used (looked up by Copr repository href)
	repoConfig, err := s.dao.RepositoryConfig.InternalOnly_FetchRepoConfigByCoprRepositoryHref(s.ctx, orgId, coprRepoHref)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), existingRepo.UUID, repoConfig.UUID)
	assert.Equal(s.T(), existingRepo.RepositoryUUID, repoConfig.RepositoryUUID)

	// Verify snapshot was created
	snaps, _, err := s.dao.Snapshot.List(s.ctx, orgId, repoConfig.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), snaps.Data)
	assert.Equal(s.T(), publicationHref, snaps.Data[0].PublicationHref)

	// Verify distribution exists in Pulp (in the cs-partner domain)
	var existingSnapModel models.Snapshot
	err = db.DB.Where("uuid = ?", snaps.Data[0].UUID).First(&existingSnapModel).Error
	require.NoError(s.T(), err)

	distribution, err := s.coprPulpClient.FindDistributionByPath(s.ctx, existingSnapModel.DistributionPath)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), distribution, "Distribution should exist in Pulp")
	assert.Equal(s.T(), existingSnapModel.DistributionPath, distribution.BasePath, "Distribution base path should match")
	pubHref := distribution.Publication.Get()
	assert.NotNil(s.T(), pubHref, "Distribution publication should not be nil")
	assert.Equal(s.T(), publicationHref, *pubHref, "Distribution should point to the correct publication")
}

func (s *CreatePartnerSnapshotSuite) TestCreatePartnerSnapshot_Idempotency() {
	accountId := uuid2.NewString()
	orgId := accountId

	// Create a publication in the cs-partner domain (simulating what Copr would have created)
	publicationHref, _ := s.createPublicationInCoprDomain()

	// First run - create the snapshot
	partnerRepoName := "idempotent-partner-repo-" + uuid2.NewString()
	coprRepoHref := "/pulp/api/v3/repositories/rpm/rpm/" + uuid2.NewString() + "/"
	payload := payloads.CreatePartnerSnapshotPayload{
		PublicationHref:    publicationHref,
		RepositoryName:     partnerRepoName,
		RepositoryVersion:  "9",
		RepositoryArch:     "x86_64",
		CoprProjectID:      "idempotent-project-111",
		CoprRepositoryHref: coprRepoHref,
		CoprBuildID:        "idempotent-build-222",
	}

	task := queue.Task{
		Typename: config.CreatePartnerSnapshotTask,
		Payload:  payload,
		OrgId:    orgId,
	}

	taskUUID1, err := s.taskClient.Enqueue(task)
	require.NoError(s.T(), err)
	s.WaitOnTask(taskUUID1)

	// Get snapshot count after first run
	repoConfig, err := s.dao.RepositoryConfig.InternalOnly_FetchRepoConfigByCoprRepositoryHref(s.ctx, orgId, coprRepoHref)
	require.NoError(s.T(), err)

	snaps1, _, err := s.dao.Snapshot.List(s.ctx, orgId, repoConfig.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, len(snaps1.Data))
	firstSnapshotUUID := snaps1.Data[0].UUID

	// Wait a bit to ensure timestamps would differ
	time.Sleep(1 * time.Second)

	// Second run - should be idempotent (no new snapshot)
	taskUUID2, err := s.taskClient.Enqueue(task)
	require.NoError(s.T(), err)
	s.WaitOnTask(taskUUID2)

	// Verify still only one snapshot exists (same one)
	snaps2, _, err := s.dao.Snapshot.List(s.ctx, orgId, repoConfig.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, len(snaps2.Data), "Should still have only 1 snapshot (idempotent)")
	assert.Equal(s.T(), firstSnapshotUUID, snaps2.Data[0].UUID, "Should be the same snapshot UUID")

	// Verify distribution still exists and wasn't duplicated (in cs-partner domain)
	var idempotentSnapModel models.Snapshot
	err = db.DB.Where("uuid = ?", firstSnapshotUUID).First(&idempotentSnapModel).Error
	require.NoError(s.T(), err)

	distribution, err := s.coprPulpClient.FindDistributionByPath(s.ctx, idempotentSnapModel.DistributionPath)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), distribution, "Distribution should still exist after idempotent run")
}

func (s *CreatePartnerSnapshotSuite) TestCreatePartnerSnapshot_PublicationNotFound() {
	accountId := uuid2.NewString()
	orgId := accountId

	// Try with a non-existent publication href
	payload := payloads.CreatePartnerSnapshotPayload{
		PublicationHref:    "/pulp/api/v3/publications/rpm/rpm/nonexistent-uuid/",
		RepositoryName:     "partner-repo-" + uuid2.NewString(),
		RepositoryVersion:  "9",
		RepositoryArch:     "x86_64",
		CoprProjectID:      "error-project",
		CoprRepositoryHref: "/pulp/api/v3/repositories/rpm/rpm/error/",
		CoprBuildID:        "error-build",
	}

	task := queue.Task{
		Typename: config.CreatePartnerSnapshotTask,
		Payload:  payload,
		OrgId:    orgId,
	}

	taskUUID, err := s.taskClient.Enqueue(task)
	require.NoError(s.T(), err)

	// Task should fail
	taskInfo := s.waitOnTask(taskUUID)
	assert.Equal(s.T(), config.TaskStatusFailed, taskInfo.Status)
	assert.NotNil(s.T(), taskInfo.Error)
	assert.Contains(s.T(), *taskInfo.Error, "failed to get publication")
}
