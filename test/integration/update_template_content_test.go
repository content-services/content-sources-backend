package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"testing"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	uuid2 "github.com/google/uuid"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UpdateTemplateContentSuite struct {
	Suite
	dao      *dao.DaoRegistry
	cpClient candlepin_client.CandlepinClient
}

func (s *UpdateTemplateContentSuite) SetupTest() {
	s.Suite.SetupTest()

	s.cpClient = candlepin_client.NewCandlepinClient()
	s.dao = dao.GetDaoRegistry(db.DB)

	// Force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"

	// Force content guard setup
	config.Get().Clients.Pulp.CustomRepoContentGuards = true
	config.Get().Clients.Pulp.GuardSubjectDn = "warlin.door"
}

func TestCandlepinContentUpdateSuite(t *testing.T) {
	suite.Run(t, new(UpdateTemplateContentSuite))
}

func (s *UpdateTemplateContentSuite) TestUseLatest() {
	config.Get().Features.Snapshots.Enabled = true
	err := config.ConfigureTang()
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), config.Tang)

	ctx := context.Background()
	orgID := uuid2.NewString()

	domainName, err := s.dao.Domain.FetchOrCreateDomain(ctx, orgID)
	assert.NoError(s.T(), err)

	repo := s.createAndSyncRepository(orgID, "https://rverdile.fedorapeople.org/dummy-repos/comps/repo1/")
	repoNewURL := "https://rverdile.fedorapeople.org/dummy-repos/comps/repo2/"

	_, err = s.dao.RepositoryConfig.Update(ctx, orgID, repo.UUID, api.RepositoryUpdateRequest{URL: &repoNewURL})
	assert.NoError(s.T(), err)

	repo, err = s.dao.RepositoryConfig.Fetch(ctx, orgID, repo.UUID)
	assert.NoError(s.T(), err)

	repoUUID, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)
	s.snapshotAndWait(s.taskClient, repo, repoUUID, orgID)

	// Create template
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr("includes rpm unsigned"),
		RepositoryUUIDS: []string{repo.UUID},
		OrgID:           utils.Ptr(repo.OrgID),
		UseLatest:       utils.Ptr(true),
		Arch:            utils.Ptr(config.X8664),
		Version:         utils.Ptr(config.El8),
	}
	tempResp, err := s.dao.Template.Create(ctx, reqTemplate)
	assert.NoError(s.T(), err)

	s.updateTemplateContentAndWait(orgID, tempResp.UUID, []string{repo.UUID})
	rpmPath := fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v/Packages/b", config.Get().Clients.Pulp.Server, domainName, tempResp.UUID, repo.UUID)

	// Verify the correct snapshot content is being served
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)

	rpms, _, err := s.dao.Rpm.ListTemplateRpms(ctx, orgID, tempResp.UUID, "bear", api.PaginationData{})
	assert.NoError(s.T(), err)
	assert.Len(s.T(), rpms, 1)
}

func (s *UpdateTemplateContentSuite) TestCreateCandlepinContent() {
	ctx := context.Background()
	orgID := uuid2.NewString()

	domainName, err := s.dao.Domain.FetchOrCreateDomain(ctx, orgID)
	assert.NoError(s.T(), err)

	repo1 := s.createAndSyncRepository(orgID, "https://fixtures.pulpproject.org/rpm-unsigned/")
	repo2 := s.createAndSyncRepository(orgID, "https://rverdile.fedorapeople.org/dummy-repos/comps/repo1/")

	// Repo3 is not synced, so it when included with a template, should be ignored
	repo3Name := uuid2.NewString()
	repoURL := "https://rverdile.fedorapeople.org/dummy-repos/comps/repo2/"
	repo3, err := s.dao.RepositoryConfig.Create(ctx, api.RepositoryRequest{
		Name:      &repo3Name,
		URL:       &repoURL,
		OrgID:     &orgID,
		AccountID: &orgID,
	})
	assert.NoError(s.T(), err)

	repo1ContentID := candlepin_client.GetContentID(repo1.UUID)
	repo2ContentID := candlepin_client.GetContentID(repo2.UUID)
	repo3ContentID := candlepin_client.GetContentID(repo3.UUID)

	// Create initial template
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr("includes rpm unsigned"),
		RepositoryUUIDS: []string{repo1.UUID},
		OrgID:           utils.Ptr(repo1.OrgID),
		Arch:            utils.Ptr(config.AARCH64),
		Version:         utils.Ptr(config.El8),
	}
	tempResp, err := s.dao.Template.Create(ctx, reqTemplate)
	assert.NoError(s.T(), err)
	assert.False(s.T(), tempResp.RHSMEnvironmentCreated)

	distPath1 := fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v", config.Get().Clients.Pulp.Server, domainName, tempResp.UUID, repo1.UUID)
	distPath2 := fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v", config.Get().Clients.Pulp.Server, domainName, tempResp.UUID, repo2.UUID)
	distPath3 := fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v", config.Get().Clients.Pulp.Server, domainName, tempResp.UUID, repo3.UUID)

	repo1UrlOverride := caliri.ContentOverrideDTO{
		Name:         utils.Ptr("baseurl"),
		ContentLabel: utils.Ptr(repo1.Label),
		Value:        utils.Ptr(distPath1),
	}
	repo1CaOverride := caliri.ContentOverrideDTO{
		Name:         utils.Ptr("sslcacert"),
		ContentLabel: utils.Ptr(repo1.Label),
		Value:        utils.Ptr(" "),
	}
	repo1VerifyOverride := caliri.ContentOverrideDTO{
		Name:         utils.Ptr("sslverifystatus"),
		ContentLabel: utils.Ptr(repo1.Label),
		Value:        utils.Ptr("0"),
	}

	repo2UrlOverride := caliri.ContentOverrideDTO{
		Name:         utils.Ptr("baseurl"),
		ContentLabel: utils.Ptr(repo2.Label),
		Value:        utils.Ptr(distPath2),
	}
	repo2CaOverride := caliri.ContentOverrideDTO{
		Name:         utils.Ptr("sslcacert"),
		ContentLabel: utils.Ptr(repo2.Label),
		Value:        utils.Ptr(" "),
	}
	repo2VerifyOverride := caliri.ContentOverrideDTO{
		Name:         utils.Ptr("sslverifystatus"),
		ContentLabel: utils.Ptr(repo2.Label),
		Value:        utils.Ptr("0"),
	}

	// Update template with new repository
	payload := s.updateTemplateContentAndWait(orgID, tempResp.UUID, []string{repo1.UUID})

	// Verify environment_created was set
	tempResp, err = s.dao.Template.Fetch(ctx, orgID, tempResp.UUID, false)
	assert.NoError(s.T(), err)
	assert.True(s.T(), tempResp.RHSMEnvironmentCreated)

	// Verify correct distribution has been created in pulp
	err = s.getRequest(distPath1, identity.Identity{OrgID: repo1.OrgID, Internal: identity.Internal{OrgID: repo1.OrgID}}, 200)
	assert.NoError(s.T(), err)

	// Verify Candlepin contents for initial template
	ownerKey := candlepin_client.DevelOrgKey
	productID := candlepin_client.GetProductID(ownerKey)

	require.NotNil(s.T(), payload.PoolID)
	poolID := payload.PoolID

	pool, err := s.cpClient.FetchPool(ctx, ownerKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), *poolID, pool.GetId())
	assert.Equal(s.T(), productID, pool.GetProductId())

	product, err := s.cpClient.FetchProduct(ctx, orgID, candlepin_client.GetProductID(candlepin_client.OwnerKey(orgID)))
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), productID, product.GetId())

	environmentID := candlepin_client.GetEnvironmentID(payload.TemplateUUID)
	environment, err := s.cpClient.FetchEnvironment(ctx, environmentID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), environmentID, environment.GetId())

	environmentContent := environment.GetEnvironmentContent()
	require.NotEmpty(s.T(), environmentContent)
	var environmentContentIDs []string
	for _, content := range environmentContent {
		environmentContentIDs = append(environmentContentIDs, content.GetContentId())
	}
	assert.Contains(s.T(), environmentContentIDs, repo1ContentID)

	s.AssertOverrides(ctx, environmentID, []caliri.ContentOverrideDTO{repo1UrlOverride, repo1CaOverride, repo1VerifyOverride})

	// Add new repositories to template
	updateReq := api.TemplateUpdateRequest{
		RepositoryUUIDS: []string{repo1.UUID, repo2.UUID, repo3.UUID},
		OrgID:           &orgID,
	}
	_, err = s.dao.Template.Update(ctx, orgID, tempResp.UUID, updateReq)
	assert.NoError(s.T(), err)

	// Update templates with new repositories
	s.updateTemplateContentAndWait(orgID, tempResp.UUID, []string{repo1.UUID, repo2.UUID, repo3.UUID})

	// Verify correct distributions have been created in pulp
	err = s.getRequest(distPath1, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 200)
	assert.NoError(s.T(), err)
	err = s.getRequest(distPath2, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 200)
	assert.NoError(s.T(), err)
	// Repo3 Should be a 404, since it was never snapshotted
	err = s.getRequest(distPath3, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 404)
	assert.NoError(s.T(), err)

	// Verify new content has been correctly added to candlepin environment
	environment, err = s.cpClient.FetchEnvironment(ctx, environmentID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), environmentID, environment.GetId())

	environmentContent = environment.GetEnvironmentContent()
	require.NotEmpty(s.T(), environmentContent)
	environmentContentIDs = []string{}
	for _, content := range environmentContent {
		environmentContentIDs = append(environmentContentIDs, content.GetContentId())
	}
	assert.Contains(s.T(), environmentContentIDs, repo1ContentID)
	assert.Contains(s.T(), environmentContentIDs, repo2ContentID)
	assert.NotContains(s.T(), environmentContentIDs, repo3ContentID)

	s.AssertOverrides(ctx, environmentID, []caliri.ContentOverrideDTO{repo1UrlOverride, repo1CaOverride, repo1VerifyOverride, repo2UrlOverride, repo2CaOverride, repo2VerifyOverride})

	// Remove 2 repositories from the template
	updateReq = api.TemplateUpdateRequest{
		RepositoryUUIDS: []string{repo1.UUID},
		OrgID:           &orgID,
	}
	_, err = s.dao.Template.Update(ctx, orgID, tempResp.UUID, updateReq)
	assert.NoError(s.T(), err)

	// Update template content to remove the two repositories
	s.updateTemplateContentAndWait(orgID, tempResp.UUID, []string{repo1.UUID})

	// Verify distribution for first repo still exists, but the no longer exists for the two removed repositories
	err = s.getRequest(distPath1, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 200)
	assert.NoError(s.T(), err)
	err = s.getRequest(distPath2, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 404)
	assert.NoError(s.T(), err)
	err = s.getRequest(distPath3, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 404)
	assert.NoError(s.T(), err)

	// Verify the candlepin environment contains the content from the first repo, but not the removed repos
	environment, err = s.cpClient.FetchEnvironment(ctx, environmentID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), environmentID, environment.GetId())

	environmentContent = environment.GetEnvironmentContent()
	require.NotEmpty(s.T(), environmentContent)
	environmentContentIDs = []string{}
	for _, content := range environmentContent {
		environmentContentIDs = append(environmentContentIDs, content.GetContentId())
	}
	assert.Contains(s.T(), environmentContentIDs, repo1ContentID)
	assert.NotContains(s.T(), environmentContentIDs, repo2ContentID)
	assert.NotContains(s.T(), environmentContentIDs, repo3ContentID)

	s.AssertOverrides(ctx, environmentID, []caliri.ContentOverrideDTO{repo1UrlOverride, repo1CaOverride, repo1VerifyOverride})

	// Rename template
	updateReq = api.TemplateUpdateRequest{
		Name: utils.Ptr(fmt.Sprintf("updated %s", *reqTemplate.Name)),
	}
	tempResp, err = s.dao.Template.Update(ctx, orgID, tempResp.UUID, updateReq)
	assert.NoError(s.T(), err)

	// Update template so that the environment name also changes
	s.updateTemplateContentAndWait(orgID, tempResp.UUID, []string{repo1.UUID})

	// Verify renaming of the environment
	environment, err = s.cpClient.FetchEnvironment(ctx, environmentID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), environmentID, environment.GetId())
	assert.Equal(s.T(), *updateReq.Name, tempResp.Name)
	assert.Equal(s.T(), tempResp.Name, environment.GetName())

	tempResp, err = s.dao.Template.Fetch(ctx, orgID, tempResp.UUID, false)

	assert.NoError(s.T(), err)
	s.deleteTemplateAndWait(orgID, tempResp)

	// Verify distribution has been deleted
	err = s.getRequest(distPath1, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 404)
	assert.NoError(s.T(), err)

	// Verify environment has been deleted
	env, err := s.cpClient.FetchEnvironment(ctx, environmentID)
	assert.Nil(s.T(), err)
	assert.Nil(s.T(), env)

	// Verify template has been deleted
	tempResp, err = s.dao.Template.Fetch(ctx, orgID, tempResp.UUID, false)
	assert.Error(s.T(), err)
	if err != nil {
		daoError, ok := err.(*ce.DaoError)
		assert.True(s.T(), ok)
		assert.True(s.T(), daoError.NotFound)
	}
}

func pathForUrl(t *testing.T, urlIn string) string {
	fullUrl, err := url.Parse(urlIn)
	assert.NoError(t, err)
	return fullUrl.Path
}

func (s *UpdateTemplateContentSuite) TestDelete() {
	var err error
	ctx := context.Background()
	orgID := uuid2.NewString()

	// Create repo
	repo := s.createAndSyncRepository(orgID, "https://rverdile.fedorapeople.org/dummy-repos/comps/repo1/")

	// Create consumer
	consumerName := "test-consumer"
	consumerResp, err := s.cpClient.CreateConsumer(ctx, orgID, consumerName)
	assert.NoError(s.T(), err)
	require.NotNil(s.T(), consumerResp)

	// Create template (no repo needed because we don't need snapshotting to test this)
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr("test template description"),
		RepositoryUUIDS: []string{repo.UUID},
		OrgID:           utils.Ptr(orgID),
		UseLatest:       utils.Ptr(true),
		Arch:            utils.Ptr(config.X8664),
		Version:         utils.Ptr(config.El8),
	}
	tempResp, err := s.dao.Template.Create(ctx, reqTemplate)
	assert.NoError(s.T(), err)

	s.updateTemplateContentAndWait(orgID, tempResp.UUID, tempResp.RepositoryUUIDS)

	err = s.cpClient.AssociateEnvironment(ctx, candlepin_client.DevelOrgKey, tempResp.UUID, consumerResp.GetUuid())
	assert.NoError(s.T(), err)

	// Delete template
	s.deleteTemplateAndWait(orgID, tempResp)

	// Verify consumer still exists
	consumer, err := s.cpClient.FetchConsumer(ctx, consumerResp.GetUuid())
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), consumer)
	assert.Equal(s.T(), consumerName, consumer.GetName())

	// Delete consumer for cleanup
	err = s.cpClient.DeleteConsumer(ctx, consumer.GetUuid())
	assert.NoError(s.T(), err)
}

func (s *UpdateTemplateContentSuite) AssertOverrides(ctx context.Context, envId string, expected []caliri.ContentOverrideDTO) {
	existing, err := s.cpClient.FetchContentOverrides(ctx, envId)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), len(expected), len(existing))

	for i := 0; i < len(existing); i++ {
		existingDto := existing[i]
		found := false
		for j := 0; j < len(expected); j++ {
			expectedDTO := expected[j]
			if *existingDto.Name == *expectedDTO.Name && *existingDto.ContentLabel == *expectedDTO.ContentLabel {
				if *existingDto.Name == candlepin_client.OverrideNameBaseUrl && pathForUrl(s.T(), *existingDto.Value) == pathForUrl(s.T(), *expectedDTO.Value) {
					found = true
					break
				} else if *existingDto.Value == *expectedDTO.Value {
					found = true
					break
				}
			}
		}
		assert.True(s.T(), found, "Could not find override %v: %v", *existingDto.ContentLabel, *existingDto.Name)
	}
}

func (s *UpdateTemplateContentSuite) updateTemplateContentAndWait(orgId string, tempUUID string, repoConfigUUIDS []string) payloads.UpdateTemplateContentPayload {
	var err error
	payload := payloads.UpdateTemplateContentPayload{
		TemplateUUID:    tempUUID,
		RepoConfigUUIDs: repoConfigUUIDS,
	}
	task := queue.Task{
		Typename: config.UpdateTemplateContentTask,
		Payload:  payload,
		OrgId:    orgId,
	}

	taskUUID, err := s.taskClient.Enqueue(task)
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUUID)

	taskInfo, err := s.queue.Status(taskUUID)
	assert.NoError(s.T(), err)

	err = json.Unmarshal(taskInfo.Payload, &payload)
	assert.NoError(s.T(), err)

	return payload
}

func (s *UpdateTemplateContentSuite) deleteTemplateAndWait(orgID string, template api.TemplateResponse) {
	var err error
	payload := tasks.DeleteTemplatesPayload{
		TemplateUUID:    template.UUID,
		RepoConfigUUIDs: template.RepositoryUUIDS,
	}
	task := queue.Task{
		Typename: config.DeleteTemplatesTask,
		Payload:  payload,
		OrgId:    orgID,
	}

	taskUUID, err := s.taskClient.Enqueue(task)
	assert.NoError(s.T(), err)

	s.WaitOnTask(taskUUID)

	taskInfo, err := s.queue.Status(taskUUID)
	assert.NoError(s.T(), err)

	err = json.Unmarshal(taskInfo.Payload, &payload)
	assert.NoError(s.T(), err)
}
