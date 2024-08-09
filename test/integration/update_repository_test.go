package integration

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	uuid2 "github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type UpdateRepositoryTaskSuite struct {
	Suite
	dao      *dao.DaoRegistry
	cpClient candlepin_client.CandlepinClient
	ctx      context.Context
	orgID    string
}

func (s *UpdateRepositoryTaskSuite) SetupTest() {
	s.Suite.SetupTest()

	s.cpClient = candlepin_client.NewCandlepinClient()

	// Force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"
}

func TestRepositoryUpdateSuite(t *testing.T) {
	suite.Run(t, new(UpdateRepositoryTaskSuite))
}

func (s *UpdateRepositoryTaskSuite) TestUpdateRepository() {
	s.dao = dao.GetDaoRegistry(s.db)
	s.ctx = context.Background()
	s.orgID = uuid2.NewString()

	repo1 := s.createAndSyncRepository(s.orgID, "https://jlsherrill.fedorapeople.org/fake-repos/really-empty/")

	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr("includes rpm unsigned"),
		RepositoryUUIDS: []string{repo1.UUID},
		OrgID:           utils.Ptr(repo1.OrgID),
		Arch:            utils.Ptr(config.AARCH64),
		Version:         utils.Ptr(config.El8),
	}
	tempResp, err := s.dao.Template.Create(s.ctx, reqTemplate)
	assert.NoError(s.T(), err)

	task := queue.Task{
		Typename: config.UpdateTemplateContentTask,
		Payload: payloads.UpdateTemplateContentPayload{
			TemplateUUID:    tempResp.UUID,
			RepoConfigUUIDs: []string{repo1.UUID},
		},
		OrgId: repo1.OrgID,
	}
	taskUUID, err := s.taskClient.Enqueue(task)
	assert.NoError(s.T(), err)
	s.WaitOnTask(taskUUID)

	// Verify no GPG Key and No modular_hotfixes override
	gpgURL := s.ContentGPGKeyUrl(repo1.UUID)
	assert.True(s.T(), gpgURL == nil || *gpgURL == "")
	assert.False(s.T(), s.HasModHotfixOverride(tempResp.UUID, repo1.Label))

	// Now set module hotfixes and GPGKey
	_, err = s.dao.RepositoryConfig.Update(s.ctx, s.orgID, repo1.UUID, api.RepositoryUpdateRequest{GpgKey: utils.Ptr("GPG KEY"), ModuleHotfixes: utils.Ptr(true)})
	assert.NoError(s.T(), err)
	task = queue.Task{
		Typename: config.UpdateRepositoryTask,
		Payload: tasks.UpdateRepositoryPayload{
			RepositoryConfigUUID: repo1.UUID,
		},
		OrgId: repo1.OrgID,
	}
	taskUUID, err = s.taskClient.Enqueue(task)
	assert.NoError(s.T(), err)
	s.WaitOnTask(taskUUID)

	gpgURL = s.ContentGPGKeyUrl(repo1.UUID)
	assert.True(s.T(), gpgURL != nil && *gpgURL != "")
	assert.True(s.T(), s.HasModHotfixOverride(tempResp.UUID, repo1.Label))

	// reset them to ensure they change back
	// Now set module hotfixes and GPGKey
	_, err = s.dao.RepositoryConfig.Update(s.ctx, s.orgID, repo1.UUID, api.RepositoryUpdateRequest{GpgKey: utils.Ptr(""), ModuleHotfixes: utils.Ptr(false)})
	assert.NoError(s.T(), err)
	task = queue.Task{
		Typename: config.UpdateRepositoryTask,
		Payload: tasks.UpdateRepositoryPayload{
			RepositoryConfigUUID: repo1.UUID,
		},
		OrgId: repo1.OrgID,
	}
	taskUUID, err = s.taskClient.Enqueue(task)
	assert.NoError(s.T(), err)
	s.WaitOnTask(taskUUID)

	// Check they are reset
	gpgURL = s.ContentGPGKeyUrl(repo1.UUID)
	assert.True(s.T(), gpgURL == nil || *gpgURL == "")
	assert.False(s.T(), s.HasModHotfixOverride(tempResp.UUID, repo1.Label))
}

func (s *UpdateRepositoryTaskSuite) ContentGPGKeyUrl(rcUUID string) *string {
	dto, err := s.cpClient.FetchContent(s.ctx, s.orgID, rcUUID)
	assert.NoError(s.T(), err)
	if dto == nil {
		return nil
	} else {
		return dto.GpgUrl
	}
}

func (s *UpdateRepositoryTaskSuite) HasModHotfixOverride(templateUUID string, repoLabel string) bool {
	overrides, err := s.cpClient.FetchContentOverridesForRepo(s.ctx, templateUUID, repoLabel)
	assert.NoError(s.T(), err)
	for _, override := range overrides {
		if *override.ContentLabel == repoLabel && *override.Name == candlepin_client.OverrideModuleHotfixes {
			return true
		}
	}
	return false
}
