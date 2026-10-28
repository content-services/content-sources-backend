package tasks

import (
	"context"
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type CreatePartnerSnapshotSuite struct {
	suite.Suite
	mockDaoRegistry *dao.MockDaoRegistry
	MockPulpClient  pulp_client.MockPulpClient
	MockQueue       queue.MockQueue
	Queue           queue.Queue
}

func TestCreatePartnerSnapshotSuite(t *testing.T) {
	suite.Run(t, new(CreatePartnerSnapshotSuite))
}

func (s *CreatePartnerSnapshotSuite) SetupTest() {
	s.mockDaoRegistry = dao.GetMockDaoRegistry(s.T())
	s.MockPulpClient = *pulp_client.NewMockPulpClient(s.T())
	s.MockQueue = *queue.NewMockQueue(s.T())
	s.Queue = &s.MockQueue
	config.Get().Clients.Pulp.RepoContentGuards = false
}

func (s *CreatePartnerSnapshotSuite) TestCreatePartnerSnapshot_Idempotent() {
	ctx := context.Background()
	orgID := "test-org-123"
	accountID := "test-account-456"
	domainName := "test-domain"

	publicationHref := "/pulp/api/v3/publications/rpm/rpm/pub-uuid/"
	versionHref := "/pulp/api/v3/repositories/rpm/rpm/repo-uuid/versions/1/"
	coprRepoHref := "/pulp/api/v3/repositories/rpm/rpm/copr-repo-uuid/"

	payload := payloads.CreatePartnerSnapshotPayload{
		PublicationHref:    publicationHref,
		RepositoryName:     "test-partner-repo",
		RepositoryVersion:  "9",
		RepositoryArch:     "x86_64",
		CoprProjectID:      "copr-project-123",
		CoprRepositoryHref: coprRepoHref,
		CoprBuildID:        "copr-build-456",
	}

	task := models.TaskInfo{
		Id:        uuid.New(),
		OrgId:     orgID,
		AccountId: accountID,
	}

	// Mock domain lookup/create
	domainHref := "/pulp/api/v3/domains/" + domainName + "/"
	s.MockPulpClient.On("LookupOrCreateDomain", ctx, domainName).Return(domainHref, nil)

	// Mock publication lookup
	publication := zest.RpmRpmPublicationResponse{
		PulpHref:          &publicationHref,
		RepositoryVersion: &versionHref,
	}
	s.MockPulpClient.On("GetRpmPublication", ctx, publicationHref).Return(&publication, nil)

	// Mock repository config lookup - found
	existingRepoConfig := api.RepositoryResponse{
		UUID:           uuid.NewString(),
		RepositoryUUID: uuid.NewString(),
		OrgID:          orgID,
		Name:           "test-partner-repo",
	}
	s.mockDaoRegistry.RepositoryConfig.On("InternalOnly_FetchRepoConfigByCoprRepositoryHref", ctx, orgID, coprRepoHref).
		Return(existingRepoConfig, nil)

	// Mock snapshot lookup - FOUND (snapshot already exists)
	existingSnapshot := &api.SnapshotResponse{
		UUID:            uuid.NewString(),
		PublicationHref: publicationHref,
	}
	s.mockDaoRegistry.Snapshot.On("FetchSnapshotByVersionHref", ctx, existingRepoConfig.UUID, versionHref).
		Return(existingSnapshot, nil)

	// Should NOT call any Pulp methods or snapshot creation

	cps := CreatePartnerSnapshot{
		orgID:      orgID,
		accountID:  accountID,
		domainName: domainName,
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient: &s.MockPulpClient,
		payload:    &payload,
		task:       &task,
		queue:      &s.Queue,
		ctx:        ctx,
		logger:     &log.Logger,
	}

	err := cps.Run()
	assert.NoError(s.T(), err)
	// Verify no snapshot was created (idempotent)
	s.mockDaoRegistry.Snapshot.AssertNotCalled(s.T(), "Create")
}

func (s *CreatePartnerSnapshotSuite) TestCreatePartnerSnapshot_PublicationNotFound() {
	ctx := context.Background()
	orgID := "test-org-123"
	accountID := "test-account-456"
	domainName := "test-domain"

	publicationHref := "/pulp/api/v3/publications/rpm/rpm/nonexistent/"

	payload := payloads.CreatePartnerSnapshotPayload{
		PublicationHref:    publicationHref,
		RepositoryName:     "test-partner-repo",
		RepositoryVersion:  "9",
		RepositoryArch:     "x86_64",
		CoprProjectID:      "copr-project-123",
		CoprRepositoryHref: "/pulp/api/v3/repositories/rpm/rpm/copr-repo-uuid/",
		CoprBuildID:        "copr-build-456",
	}

	task := models.TaskInfo{
		Id:        uuid.New(),
		OrgId:     orgID,
		AccountId: accountID,
	}

	// Mock domain lookup/create
	domainHref := "/pulp/api/v3/domains/" + domainName + "/"
	s.MockPulpClient.On("LookupOrCreateDomain", ctx, domainName).Return(domainHref, nil)

	// Mock publication lookup - returns nil (not found)
	s.MockPulpClient.On("GetRpmPublication", ctx, publicationHref).
		Return((*zest.RpmRpmPublicationResponse)(nil), nil)

	cps := CreatePartnerSnapshot{
		orgID:      orgID,
		accountID:  accountID,
		domainName: domainName,
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient: &s.MockPulpClient,
		payload:    &payload,
		task:       &task,
		queue:      &s.Queue,
		ctx:        ctx,
		logger:     &log.Logger,
	}

	err := cps.Run()
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "publication not found")
}

func (s *CreatePartnerSnapshotSuite) TestCreatePartnerSnapshot_RepositoryCreationFails() {
	ctx := context.Background()
	orgID := "test-org-123"
	accountID := "test-account-456"
	domainName := "test-domain"

	publicationHref := "/pulp/api/v3/publications/rpm/rpm/pub-uuid/"
	versionHref := "/pulp/api/v3/repositories/rpm/rpm/repo-uuid/versions/1/"
	coprRepoHref := "/pulp/api/v3/repositories/rpm/rpm/copr-repo-uuid/"

	payload := payloads.CreatePartnerSnapshotPayload{
		PublicationHref:    publicationHref,
		RepositoryName:     "test-partner-repo",
		RepositoryVersion:  "9",
		RepositoryArch:     "x86_64",
		CoprProjectID:      "copr-project-123",
		CoprRepositoryHref: coprRepoHref,
		CoprBuildID:        "copr-build-456",
	}

	task := models.TaskInfo{
		Id:        uuid.New(),
		OrgId:     orgID,
		AccountId: accountID,
	}

	// Mock domain lookup/create
	domainHref := "/pulp/api/v3/domains/" + domainName + "/"
	s.MockPulpClient.On("LookupOrCreateDomain", ctx, domainName).Return(domainHref, nil)

	// Mock publication lookup
	publication := zest.RpmRpmPublicationResponse{
		PulpHref:          &publicationHref,
		RepositoryVersion: &versionHref,
	}
	s.MockPulpClient.On("GetRpmPublication", ctx, publicationHref).Return(&publication, nil)

	// Mock repository config lookup - not found
	notFoundErr := &ce.DaoError{NotFound: true, Message: "Not found"}
	s.mockDaoRegistry.RepositoryConfig.On("InternalOnly_FetchRepoConfigByCoprRepositoryHref", ctx, orgID, coprRepoHref).
		Return(api.RepositoryResponse{}, notFoundErr)

	// Mock repository config creation - FAILS
	createErr := fmt.Errorf("database error")
	s.mockDaoRegistry.RepositoryConfig.On("Create", ctx, mock.Anything).
		Return(api.RepositoryResponse{}, createErr)

	cps := CreatePartnerSnapshot{
		orgID:      orgID,
		accountID:  accountID,
		domainName: domainName,
		daoReg:     s.mockDaoRegistry.ToDaoRegistry(),
		pulpClient: &s.MockPulpClient,
		payload:    &payload,
		task:       &task,
		queue:      &s.Queue,
		ctx:        ctx,
		logger:     &log.Logger,
	}

	err := cps.Run()
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "failed to create repository config")
}

func (s *CreatePartnerSnapshotSuite) TestResumableSnapshotInterface() {
	payload := payloads.CreatePartnerSnapshotPayload{
		PublicationHref: "/pulp/api/v3/publications/rpm/rpm/pub-uuid/",
	}

	task := models.TaskInfo{
		Id: uuid.New(),
	}

	mockQueue := queue.NewMockQueue(s.T())
	var q queue.Queue = mockQueue

	cps := CreatePartnerSnapshot{
		payload: &payload,
		task:    &task,
		queue:   &q,
	}

	// Test GetPublicationTaskHref - should always return nil for Copr repos
	assert.Nil(s.T(), cps.GetPublicationTaskHref())

	// Test SavePublicationTaskHref - should be no-op
	err := cps.SavePublicationTaskHref("some-href")
	assert.NoError(s.T(), err)

	// Test DistributionTaskHref
	distHref := "dist-task-href"
	mockQueue.On("UpdatePayload", &task, mock.Anything).Return(&task, nil)
	err = cps.SaveDistributionTaskHref(distHref)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), distHref, *cps.GetDistributionTaskHref())

	// Test SnapshotIdent
	ident := uuid.NewString()
	err = cps.SaveSnapshotIdent(ident)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), ident, *cps.GetSnapshotIdent())

	// Test SnapshotUUID
	snapUUID := uuid.NewString()
	err = cps.SaveSnapshotUUID(snapUUID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), snapUUID, *cps.GetSnapshotUUID())
}
