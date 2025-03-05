package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	uuid2 "github.com/google/uuid"
	"github.com/labstack/gommon/random"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UpdateLatestSnapshotSuite struct {
	Suite
	dao      *dao.DaoRegistry
	cpClient candlepin_client.CandlepinClient
}

func TestUpdateLatestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(UpdateLatestSnapshotSuite))
}

func (s *UpdateLatestSnapshotSuite) SetupTest() {
	s.Suite.SetupTest()

	s.cpClient = candlepin_client.NewCandlepinClient()

	// Force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"

	// Force content guard setup
	config.Get().Clients.Pulp.RepoContentGuards = true
	config.Get().Clients.Pulp.GuardSubjectDn = "warlin.door"
}

func (s *UpdateLatestSnapshotSuite) TestUpdateLatestSnapshot() {
	config.Get().Features.Snapshots.Enabled = true
	err := config.ConfigureTang()
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), config.Tang)

	s.dao = dao.GetDaoRegistry(db.DB)
	ctx := context.Background()
	orgID := uuid2.NewString()

	domainName, err := s.dao.Domain.FetchOrCreateDomain(ctx, orgID)
	assert.NoError(s.T(), err)

	repo := s.createAndSyncRepository(orgID, "https://content-services.github.io/fixtures/yum/comps-modules/v1/")

	// Create template with use latest
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr("includes rpm unsigned"),
		RepositoryUUIDS: []string{repo.UUID},
		OrgID:           utils.Ptr(repo.OrgID),
		UseLatest:       utils.Ptr(true),
		Arch:            utils.Ptr(config.X8664),
		Version:         utils.Ptr(config.El8),
	}
	tempResp1, err := s.dao.Template.Create(ctx, reqTemplate)
	assert.NoError(s.T(), err)
	s.updateTemplateContentAndWait(orgID, tempResp1.UUID, []string{repo.UUID})

	host, err := pulp_client.GetPulpClientWithDomain(domainName).GetContentPath(ctx)
	require.NoError(s.T(), err)

	// Verify the correct snapshot content is being served by the template
	rpmPath := fmt.Sprintf("%v%v/templates/%v/%v/Packages/s", host, domainName, tempResp1.UUID, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)

	// Create template with date
	reqTemplate = api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr("includes rpm unsigned"),
		RepositoryUUIDS: []string{repo.UUID},
		OrgID:           utils.Ptr(repo.OrgID),
		Arch:            utils.Ptr(config.X8664),
		Version:         utils.Ptr(config.El8),
		Date:            utils.Ptr(api.EmptiableDate(time.Now())),
	}
	tempResp2, err := s.dao.Template.Create(ctx, reqTemplate)
	assert.NoError(s.T(), err)
	s.updateTemplateContentAndWait(orgID, tempResp2.UUID, []string{repo.UUID})

	// Verify the correct snapshot content is being served by the template
	rpmPath = fmt.Sprintf("%v%v/templates/%v/%v/Packages/s", host, domainName, tempResp2.UUID, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)

	// Change the URL of the repo, create a new snapshot, and update template snapshots
	repoNewURL := "https://content-services.github.io/fixtures/yum/comps-modules/v2/"
	_, err = s.dao.RepositoryConfig.Update(ctx, orgID, repo.UUID, api.RepositoryUpdateRequest{URL: &repoNewURL})
	assert.NoError(s.T(), err)

	repo, err = s.dao.RepositoryConfig.Fetch(ctx, orgID, repo.UUID)
	assert.NoError(s.T(), err)

	repoUUID, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)
	s.snapshotAndWait(s.taskClient, repo, repoUUID, true)
	s.updateLatestSnapshotAndWait(orgID, repo.UUID)

	// Verify that template1 serves the new snapshot and template2 serves the original snapshot
	rpmPath = fmt.Sprintf("%v%v/templates/%v/%v/Packages/b", host, domainName, tempResp1.UUID, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)

	rpmPath = fmt.Sprintf("%v%v/templates/%v/%v/Packages/s", host, domainName, tempResp2.UUID, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)
}

func (s *UpdateLatestSnapshotSuite) TestUpdateLatestSnapshotForRedHatRepo() {
	config.Get().Features.Snapshots.Enabled = true
	err := config.ConfigureTang()
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), config.Tang)

	s.dao = dao.GetDaoRegistry(db.DB)
	ctx := context.Background()
	orgID := uuid2.NewString()

	// Create a "red hat" repository and add it to a template
	url, cancelFunc, err := ServeRandomYumRepo(nil)
	require.NoError(s.T(), err)
	defer cancelFunc()

	repoResp, err := s.rhelRepo(url, config.SubscriptionFeaturesIgnored[0])
	require.NoError(s.T(), err)

	// Start the task
	taskClient := client.NewTaskClient(&s.queue)
	uuidStr, err := uuid2.Parse(repoResp.RepositoryUUID)
	require.NoError(s.T(), err)

	s.snapshotAndWait(taskClient, *repoResp, uuidStr, true)

	// Create template with use latest
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr("includes rpm unsigned"),
		RepositoryUUIDS: []string{repoResp.UUID},
		OrgID:           utils.Ptr(orgID),
		UseLatest:       utils.Ptr(true),
		Arch:            utils.Ptr(config.X8664),
		Version:         utils.Ptr(config.El8),
	}
	tempResp, err := s.dao.Template.Create(ctx, reqTemplate)
	assert.NoError(s.T(), err)
	s.updateTemplateContentAndWait(orgID, tempResp.UUID, []string{repoResp.UUID})

	rpmPath := fmt.Sprintf("%v/giraffe-0.67-2.noarch.rpm", url)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repoResp.OrgID, Internal: identity.Internal{OrgID: repoResp.OrgID}}, 200)
	assert.NoError(s.T(), err)

	// Update the "red hat" repo with a new URL and snapshot so there are two snapshots
	opts := serveRepoOptions{
		port:         "30124",
		path:         "/" + strings.Split(url, "/")[3] + "/",
		repoSelector: "frog",
	}
	url2, cancelFunc, err := ServeRandomYumRepo(&opts)
	require.NoError(s.T(), err)
	defer cancelFunc()

	_, err = s.dao.RepositoryConfig.Update(ctx, config.RedHatOrg, repoResp.UUID, api.RepositoryUpdateRequest{URL: &url2})
	assert.NoError(s.T(), err)

	fetch, err := s.dao.RepositoryConfig.Fetch(ctx, config.RedHatOrg, repoResp.UUID)
	assert.NoError(s.T(), err)
	uuidStr, err = uuid2.Parse(fetch.RepositoryUUID)
	assert.NoError(s.T(), err)

	snapUUID := s.snapshotAndWait(taskClient, *repoResp, uuidStr, true)

	// Run update-latest-snapshot and verify template is using second snapshot
	s.updateLatestSnapshotAndWait(config.RedHatOrg, repoResp.UUID)

	tempResp, err = s.dao.Template.Fetch(ctx, orgID, tempResp.UUID, false)
	assert.NoError(s.T(), err)
	require.NotNil(s.T(), tempResp.Snapshots)
	require.NotNil(s.T(), tempResp.Snapshots[0])
	assert.Equal(s.T(), snapUUID, tempResp.Snapshots[0].UUID)

	rpmPath = fmt.Sprintf("%v/frog-0.1-1.noarch.rpm", url2)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repoResp.OrgID, Internal: identity.Internal{OrgID: repoResp.OrgID}}, 200)
	assert.NoError(s.T(), err)
}

func (s *UpdateLatestSnapshotSuite) updateTemplateContentAndWait(orgId string, tempUUID string, repoConfigUUIDS []string) payloads.UpdateTemplateContentPayload {
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

func (s *UpdateLatestSnapshotSuite) updateLatestSnapshotAndWait(orgId string, repoConfigUUID string) tasks.UpdateLatestSnapshotPayload {
	var err error
	payload := tasks.UpdateLatestSnapshotPayload{
		RepositoryConfigUUID: repoConfigUUID,
	}
	task := queue.Task{
		Typename: config.UpdateLatestSnapshotTask,
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

func (s *UpdateLatestSnapshotSuite) rhelRepo(url string, feature string) (*api.RepositoryResponse, error) {
	repo := models.Repository{
		URL:         url,
		Public:      true,
		Origin:      config.OriginRedHat,
		ContentType: config.ContentTypeRpm,
	}

	res := db.DB.Create(&repo)
	if res.Error != nil {
		return nil, res.Error
	}

	repoConfig := models.RepositoryConfiguration{
		Name:           "TestRedHatRepo:" + random.String(10),
		Label:          "test-redhat-repo-" + random.String(10),
		OrgID:          config.RedHatOrg,
		RepositoryUUID: repo.UUID,
		Snapshot:       true,
		FeatureName:    feature, // RHEL feature name
	}
	res = db.DB.Create(&repoConfig)
	if res.Error != nil {
		return nil, res.Error
	}

	repoResps := s.dao.RepositoryConfig.InternalOnly_FetchRepoConfigsForRepoUUID(context.Background(), repo.UUID)
	return &repoResps[0], nil
}
