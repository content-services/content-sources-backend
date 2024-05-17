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
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/tasks/worker"
	uuid2 "github.com/google/uuid"
	"github.com/openlyinc/pointy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UpdateTemplateContentSuite struct {
	Suite
	dao        *dao.DaoRegistry
	queue      queue.PgQueue
	taskClient client.TaskClient
	cpClient   candlepin_client.CandlepinClient
}

func (s *UpdateTemplateContentSuite) SetupTest() {
	s.Suite.SetupTest()

	wkrQueue, err := queue.NewPgQueue(db.GetUrl())
	require.NoError(s.T(), err)
	s.queue = wkrQueue

	s.taskClient = client.NewTaskClient(&s.queue)
	s.cpClient = candlepin_client.NewCandlepinClient()
	require.NoError(s.T(), err)

	wrk := worker.NewTaskWorkerPool(&wkrQueue, m.NewMetrics(prometheus.NewRegistry()))
	wrk.RegisterHandler(config.RepositorySnapshotTask, tasks.SnapshotHandler)
	wrk.RegisterHandler(config.DeleteRepositorySnapshotsTask, tasks.DeleteSnapshotHandler)
	wrk.RegisterHandler(config.UpdateTemplateContentTask, tasks.UpdateTemplateContentHandler)
	wrk.HeartbeatListener()

	wkrCtx := context.Background()
	go (wrk).StartWorkers(wkrCtx)
	go func() {
		<-wkrCtx.Done()
		wrk.Stop()
	}()
	// Force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"

	// Force content guard setup
	config.Get().Clients.Pulp.CustomRepoContentGuards = true
	config.Get().Clients.Pulp.GuardSubjectDn = "warlin.door"
}

func TestCandlepinContentUpdateSuite(t *testing.T) {
	suite.Run(t, new(UpdateTemplateContentSuite))
}

func (s *UpdateTemplateContentSuite) TestCreateCandlepinContent() {
	s.dao = dao.GetDaoRegistry(db.DB)
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
		Name:            pointy.Pointer(fmt.Sprintf("test template %v", rand.Int())),
		Description:     pointy.Pointer("includes rpm unsigned"),
		RepositoryUUIDS: []string{repo1.UUID},
		OrgID:           pointy.Pointer(repo1.OrgID),
	}
	tempResp, err := s.dao.Template.Create(ctx, reqTemplate)
	assert.NoError(s.T(), err)

	distPath1 := fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v", config.Get().Clients.Pulp.Server, domainName, tempResp.UUID, repo1.UUID)
	distPath2 := fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v", config.Get().Clients.Pulp.Server, domainName, tempResp.UUID, repo2.UUID)
	distPath3 := fmt.Sprintf("%v/pulp/content/%s/templates/%v/%v", config.Get().Clients.Pulp.Server, domainName, tempResp.UUID, repo3.UUID)

	repo1UrlOverride := caliri.ContentOverrideDTO{
		Name:         pointy.Pointer("baseurl"),
		ContentLabel: pointy.Pointer(repo1.Label),
		Value:        pointy.Pointer(distPath1),
	}
	repo1CaOverride := caliri.ContentOverrideDTO{
		Name:         pointy.Pointer("sslcacert"),
		ContentLabel: pointy.Pointer(repo1.Label),
		Value:        pointy.Pointer(" "),
	}

	repo2UrlOverride := caliri.ContentOverrideDTO{
		Name:         pointy.Pointer("baseurl"),
		ContentLabel: pointy.Pointer(repo2.Label),
		Value:        pointy.Pointer(distPath2),
	}
	repo2CaOverride := caliri.ContentOverrideDTO{
		Name:         pointy.Pointer("sslcacert"),
		ContentLabel: pointy.Pointer(repo2.Label),
		Value:        pointy.Pointer(" "),
	}

	// Update template with new repository
	payload := s.updateTemplateContentAndWait(orgID, tempResp.UUID, []string{repo1.UUID})

	// Verify correct distribution has been created in pulp
	err = s.getRequest(distPath1, identity.Identity{OrgID: repo1.OrgID, Internal: identity.Internal{OrgID: repo1.OrgID}}, 200)
	assert.NoError(s.T(), err)

	// Verify Candlepin contents for initial template
	ownerKey := candlepin_client.DevelOrgKey
	productID := candlepin_client.GetProductID(ownerKey)

	require.NotNil(s.T(), payload.PoolID)
	poolID := payload.PoolID

	pool, err := s.cpClient.FetchPool(ctx, ownerKey, productID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), *poolID, pool.GetId())
	assert.Equal(s.T(), productID, pool.GetProductId())

	product, err := s.cpClient.FetchProduct(ctx, ownerKey, candlepin_client.GetProductID(ownerKey))
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

	s.AssertOverrides(ctx, environmentID, []caliri.ContentOverrideDTO{repo1UrlOverride, repo1CaOverride})

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

	s.AssertOverrides(ctx, environmentID, []caliri.ContentOverrideDTO{repo1UrlOverride, repo1CaOverride, repo2UrlOverride, repo2CaOverride})

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

	s.AssertOverrides(ctx, environmentID, []caliri.ContentOverrideDTO{repo1UrlOverride, repo1CaOverride})
}

func pathForUrl(t *testing.T, urlIn string) string {
	fullUrl, err := url.Parse(urlIn)
	assert.NoError(t, err)
	return fullUrl.Path
}

func (s *UpdateTemplateContentSuite) AssertOverrides(ctx context.Context, envId string, expected []caliri.ContentOverrideDTO) {
	existing, err := s.cpClient.FetchContentPathOverrides(ctx, envId)
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
