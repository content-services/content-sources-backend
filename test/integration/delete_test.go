package integration

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path"
	"path/filepath"
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
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// This is a delete integration tests without any snapshotting
type DeleteTest struct {
	Suite
	dao        *dao.DaoRegistry
	pulpClient pulp_client.PulpClient
	cpClient   candlepin_client.CandlepinClient
	ctx        context.Context
}

func (s *DeleteTest) SetupTest() {
	s.configurePulpClientCertPaths()
	s.Suite.SetupTest()
	s.ctx = context.Background()
	s.cpClient = candlepin_client.NewCandlepinClient()
	s.dao = dao.GetDaoRegistry(db.DB)
	// Force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"
	// Force content guard setup
	config.Get().Clients.Pulp.RepoContentGuards = true
	config.Get().Clients.Pulp.GuardSubjectDn = "warlin.door"
}

func (s *DeleteTest) configurePulpClientCertPaths() {
	cfg := config.Get()
	root := findProjectRoot(s.T())

	if cfg.Clients.Pulp.ClientCertPath != "" && !filepath.IsAbs(cfg.Clients.Pulp.ClientCertPath) {
		cfg.Clients.Pulp.ClientCertPath = filepath.Join(root, cfg.Clients.Pulp.ClientCertPath)
	}
	if cfg.Clients.Pulp.ClientKeyPath != "" && !filepath.IsAbs(cfg.Clients.Pulp.ClientKeyPath) {
		cfg.Clients.Pulp.ClientKeyPath = filepath.Join(root, cfg.Clients.Pulp.ClientKeyPath)
	}
	if cfg.Clients.Pulp.CACertPath != "" && !filepath.IsAbs(cfg.Clients.Pulp.CACertPath) {
		cfg.Clients.Pulp.CACertPath = filepath.Join(root, cfg.Clients.Pulp.CACertPath)
	}
}

func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root")
		}
		dir = parent
	}
}

func TestDeleteTest(t *testing.T) {
	suite.Run(t, new(DeleteTest))
}

func (s *DeleteTest) TestDeleteRepositorySnapshots() {
	// Setup the repository
	accountId := uuid2.NewString()
	repo, err := s.dao.RepositoryConfig.Create(s.ctx, api.RepositoryRequest{
		Name:      utils.Ptr(uuid2.NewString()),
		URL:       utils.Ptr("https://fixtures.pulpproject.org/rpm-unsigned/"),
		AccountID: utils.Ptr(accountId),
		OrgID:     utils.Ptr(accountId),
		Snapshot:  utils.Ptr(false),
	})
	assert.NoError(s.T(), err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	assert.NoError(s.T(), err)

	// Start the task
	taskClient := client.NewTaskClient(&s.queue)

	// Delete the repository
	taskUuid, err := taskClient.Enqueue(queue.Task{
		Typename:   config.DeleteRepositorySnapshotsTask,
		Payload:    tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repo.UUID},
		OrgId:      repo.OrgID,
		ObjectUUID: utils.Ptr(repoUuid.String()),
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
	})
	assert.NoError(s.T(), err)
	s.WaitOnTask(taskUuid)

	results, _, err := s.dao.RepositoryConfig.List(s.ctx, accountId, api.PaginationData{}, api.FilterData{
		Name: repo.Name,
	})
	assert.NoError(s.T(), err)
	assert.Empty(s.T(), results.Data)
}

// Tests that a red hat repo can be deleted via this task.
//
//	Note that a user shouldn't be able to do this, but this may be done
//	via the "external_repos import-repos"  command
func (s *DeleteTest) TestDeleteRedHatRepositorySnapshots() {
	ctx := context.Background()
	daoReg := dao.GetDaoRegistry(db.DB)

	// Setup the repository
	orgID := uuid2.NewString()
	repoResp, _, err := s.createAndSyncRhelOrEpelRepo(true)
	require.NoError(s.T(), err)

	// Start the task
	taskClient := client.NewTaskClient(&s.queue)

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

	// Lookup the template snapshot path in pulp
	pClient := pulp_client.GetPulpClientWithDomain(config.RedHatDomainName)
	repoPath, err := url.Parse(repoResp.URL)
	assert.NoError(s.T(), err)
	templateSnapPath := path.Join("templates", tempResp.UUID, repoPath.Path)

	// verify distribution exists
	dist, err := pClient.FindDistributionByPath(ctx, templateSnapPath)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), dist)

	// Lookup the content
	cpClient := candlepin_client.NewCandlepinClient()
	env, err := cpClient.FetchEnvironment(ctx, tempResp.UUID)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), 1, len(env.EnvironmentContent))

	// Mark as deleted
	err = daoReg.RepositoryConfig.SoftDelete(ctx, repoResp.OrgID, repoResp.UUID)
	assert.NoError(s.T(), err)

	// Delete the repository
	taskUuid, err := taskClient.Enqueue(queue.Task{
		Typename:   config.DeleteRepositorySnapshotsTask,
		Payload:    tasks.DeleteRepositorySnapshotsPayload{RepoConfigUUID: repoResp.UUID},
		OrgId:      repoResp.OrgID,
		ObjectUUID: &repoResp.UUID,
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
	})
	assert.NoError(s.T(), err)
	s.WaitOnTask(taskUuid)

	// Check that distribution was deleted
	dist, err = pClient.FindDistributionByPath(ctx, templateSnapPath)
	assert.NoError(s.T(), err)
	assert.Nil(s.T(), dist) // Shouldn't exist anymore

	// Content should no longer be in the environment
	env, err = cpClient.FetchEnvironment(ctx, tempResp.UUID)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), 0, len(env.EnvironmentContent))
}

func (s *DeleteTest) TestDeleteSnapshot() {
	t := s.T()

	// Set up
	config.Get().Features.Snapshots.Enabled = true
	err := config.ConfigureTang()
	assert.NoError(t, err)
	assert.NotNil(t, config.Tang)
	orgID := uuid2.NewString()
	taskClient := client.NewTaskClient(&s.queue)
	domain, err := s.dao.Domain.FetchOrCreateDomain(s.ctx, orgID)
	assert.NoError(t, err)
	s.pulpClient = pulp_client.GetPulpClientWithDomain(domain)
	assert.NotNil(t, s.pulpClient)

	// Create a repo and two snapshots for it
	repo := s.createAndSyncRepository(orgID, "https://fedorapeople.org/groups/katello/fakerepos/zoo/")
	_, err = s.dao.RepositoryConfig.Update(s.ctx, orgID, repo.UUID, api.RepositoryUpdateRequest{
		URL: utils.Ptr("https://content-services.github.io/fixtures/yum/comps-modules/v1/"),
	})
	assert.NoError(t, err)
	repo, err = s.dao.RepositoryConfig.Fetch(s.ctx, orgID, repo.UUID)
	assert.NoError(t, err)
	s.snapshotAndWait(taskClient, repo, dao.UuidifyString(repo.RepositoryUUID), true)
	repoSnaps, _, err := s.dao.Snapshot.List(s.ctx, orgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(t, err)
	assert.Len(t, repoSnaps.Data, 2)

	// Create a template that uses the repo and verify that is serves the correct content
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr(""),
		RepositoryUUIDS: []string{repo.UUID},
		OrgID:           utils.Ptr(orgID),
		UseLatest:       utils.Ptr(true),
		Arch:            utils.Ptr(config.X8664),
		Version:         utils.Ptr(config.El8),
	}
	tempResp, err := s.dao.Template.Create(s.ctx, reqTemplate)
	assert.NoError(t, err)

	s.updateTemplateContentAndWait(orgID, tempResp.UUID, []string{repo.UUID})
	rpms, _, err := s.dao.Rpm.ListTemplateRpms(s.ctx, orgID, tempResp.UUID, "", api.PaginationData{})
	assert.NoError(t, err)
	assert.Len(t, rpms, 7)

	host, err := pulp_client.GetPulpClientWithDomain(domain).GetContentPath()
	assert.NoError(s.T(), err)
	rpmPath := fmt.Sprintf("%v%v/%v/latest/Packages/l", host, domain, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 404)
	assert.NoError(s.T(), err)

	// Soft delete one of the snapshots
	otherSnapUUID := repoSnaps.Data[1].UUID
	snap, err := s.dao.Snapshot.FetchModel(s.ctx, repoSnaps.Data[0].UUID, true)
	assert.NoError(t, err)
	tempSnaps, _, err := s.dao.Snapshot.ListByTemplate(s.ctx, orgID, tempResp, "", api.PaginationData{Limit: -1})
	assert.NoError(t, err)
	assert.Len(t, tempSnaps.Data, 1)
	assert.Equal(t, tempSnaps.Data[0].UUID, snap.UUID)
	err = s.dao.Snapshot.SoftDelete(s.ctx, snap.UUID)
	assert.NoError(t, err)

	// Start and wait for the delete snapshot task
	requestID := uuid2.NewString()
	taskUUID, err := taskClient.Enqueue(queue.Task{
		Typename: config.DeleteSnapshotsTask,
		Payload: utils.Ptr(payloads.DeleteSnapshotsPayload{
			RepoUUID:       repo.UUID,
			SnapshotsUUIDs: []string{snap.UUID},
		}),
		OrgId:      orgID,
		AccountId:  orgID,
		ObjectUUID: utils.Ptr(repo.UUID),
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		RequestID:  requestID,
	})
	assert.NoError(t, err)
	s.WaitOnTask(taskUUID)

	// Verify the snapshot is deleted
	repoSnaps, _, err = s.dao.Snapshot.List(s.ctx, orgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(t, err)
	assert.Len(t, repoSnaps.Data, 1)
	assert.Equal(t, repoSnaps.Data[0].UUID, otherSnapUUID)
	deletedSnap, err := s.dao.Snapshot.FetchModel(s.ctx, snap.UUID, true)
	assert.Error(t, err)
	assert.Equal(t, models.Snapshot{}, deletedSnap)

	// Verify correct pulp deletion/cleanup
	resp1, _ := s.pulpClient.FindDistributionByPath(s.ctx, snap.DistributionPath)
	assert.Nil(t, resp1)
	_, err = s.pulpClient.FindRpmPublicationByVersion(s.ctx, snap.VersionHref)
	assert.Error(t, err)
	_, err = s.pulpClient.GetRpmRepositoryVersion(s.ctx, snap.VersionHref)
	assert.Error(t, err)

	// Verify template uses the other snapshot and serves the correct content
	tempSnaps, _, err = s.dao.Snapshot.ListByTemplate(s.ctx, orgID, tempResp, "", api.PaginationData{Limit: -1})
	assert.NoError(t, err)
	assert.Len(t, tempSnaps.Data, 1)
	assert.Equal(t, tempSnaps.Data[0].UUID, otherSnapUUID)
	assert.NoError(t, err)
	rpms, _, err = s.dao.Rpm.ListTemplateRpms(s.ctx, orgID, tempResp.UUID, "", api.PaginationData{})
	assert.NoError(t, err)
	assert.Len(t, rpms, 8)

	// Verify that LastSnapshotUUID and LastSnapshotTaskUUID are updated on the repo config
	repo, err = s.dao.RepositoryConfig.Fetch(s.ctx, orgID, repo.UUID)
	assert.NoError(t, err)
	assert.Equal(t, otherSnapUUID, repo.LastSnapshotUUID)
	assert.Equal(t, "", repo.LastSnapshotTaskUUID)
	assert.Nil(t, repo.LastSnapshotTask)

	// Verify that the /latest has been updated and serves the correct content
	rpmPath = fmt.Sprintf("%v%v/%v/latest/Packages/w", host, domain, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: repo.OrgID, Internal: identity.Internal{OrgID: repo.OrgID}}, 200)
	assert.NoError(s.T(), err)
}

func (s *DeleteTest) TestDeleteRedHatSnapshot() {
	t := s.T()

	config.Get().Clients.Pulp.DownloadPolicy = "immediate"
	config.Get().Features.Snapshots.Enabled = true
	err := config.ConfigureTang()
	assert.NoError(t, err)
	assert.NotNil(t, config.Tang)
	domain, err := s.dao.Domain.FetchOrCreateDomain(s.ctx, config.RedHatOrg)
	assert.NoError(t, err)
	s.pulpClient = pulp_client.GetPulpClientWithDomain(domain)
	assert.NotNil(t, s.pulpClient)
	s.dao = dao.GetDaoRegistry(db.DB)

	// Create and snapshot a "RHEL" repo
	repo, _, err := s.createAndSyncRhelOrEpelRepo(true)
	require.NoError(t, err)

	// Update the repo so there are 2 snapshots
	opts := serveRepoOptions{
		port:         "30124",
		path:         "/" + strings.Split(repo.URL, "/")[3] + "/",
		repoSelector: "frog",
	}
	url2, cancelFunc, err := ServeRandomYumRepo(&opts)
	require.NoError(t, err)
	defer cancelFunc()
	_, err = s.dao.RepositoryConfig.Update(s.ctx, config.RedHatOrg, repo.UUID, api.RepositoryUpdateRequest{URL: &url2})
	assert.NoError(t, err)
	updatedRepo, err := s.dao.RepositoryConfig.Fetch(s.ctx, config.RedHatOrg, repo.UUID)
	assert.NoError(t, err)
	uuidStr, err := uuid2.Parse(updatedRepo.RepositoryUUID)
	assert.NoError(t, err)

	// Start the task
	taskClient := client.NewTaskClient(&s.queue)
	require.NoError(t, err)
	s.snapshotAndWait(taskClient, *repo, uuidStr, true)

	// Confirm there are 2 snapshots
	repoSnaps, _, err := s.dao.Snapshot.List(s.ctx, config.RedHatOrg, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.Len(s.T(), repoSnaps.Data, 2)

	// Create a template that uses the repo's older snapshot
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr(""),
		RepositoryUUIDS: []string{repo.UUID},
		OrgID:           utils.Ptr(config.RedHatOrg),
		UseLatest:       utils.Ptr(false),
		Date:            utils.Ptr(api.EmptiableDate(time.Now().AddDate(0, 0, -1))),
		Arch:            utils.Ptr(config.X8664),
		Version:         utils.Ptr(config.El8),
	}
	tempResp, err := s.dao.Template.Create(s.ctx, reqTemplate)
	assert.NoError(t, err)
	s.updateTemplateContentAndWait(config.RedHatOrg, tempResp.UUID, []string{repo.UUID})
	host, err := pulp_client.GetPulpClientWithDomain(domain).GetContentPath()
	assert.NoError(s.T(), err)

	olderSnap := repoSnaps.Data[1].UUID
	newerSnap := repoSnaps.Data[0].UUID

	// Verify the template is using the older snapshot
	templates, err := s.dao.Template.InternalOnlyGetTemplatesForSnapshots(s.ctx, []string{olderSnap})
	assert.NoError(t, err)
	assert.Len(t, templates, 1)
	tempSnaps, _, err := s.dao.Snapshot.ListByTemplate(s.ctx, config.RedHatOrg, tempResp, "", api.PaginationData{Limit: -1})
	assert.NoError(t, err)
	assert.Len(t, tempSnaps.Data, 1)
	assert.Equal(t, tempSnaps.Data[0].UUID, olderSnap)

	// Verify the /latest serves correct content
	rpmPath := fmt.Sprintf("%v%v/%v/latest/Packages/z", host, domain, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: config.RedHatOrg, Internal: identity.Internal{OrgID: config.RedHatOrg}}, 404)
	assert.NoError(s.T(), err)
	rpmPath = fmt.Sprintf("%v%v/%v/latest/Packages/f", host, domain, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: config.RedHatOrg, Internal: identity.Internal{OrgID: config.RedHatOrg}}, 200)
	assert.NoError(s.T(), err)

	// Soft-delete the older snapshot
	err = s.dao.Snapshot.SoftDelete(s.ctx, olderSnap)
	assert.NoError(t, err)
	olderSnapModel, err := s.dao.Snapshot.FetchModel(s.ctx, olderSnap, true)
	assert.NoError(t, err)

	// Start and wait for the delete-snapshots task
	requestID := uuid2.NewString()
	taskUUID, err := taskClient.Enqueue(queue.Task{
		Typename: config.DeleteSnapshotsTask,
		Payload: utils.Ptr(payloads.DeleteSnapshotsPayload{
			RepoUUID:       repo.UUID,
			SnapshotsUUIDs: []string{olderSnap},
		}),
		OrgId:      config.RedHatOrg,
		AccountId:  config.RedHatOrg,
		ObjectUUID: utils.Ptr(repo.UUID),
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		RequestID:  requestID,
	})
	assert.NoError(t, err)
	s.WaitOnTask(taskUUID)

	// Verify the snapshot is deleted
	repoSnaps, _, err = s.dao.Snapshot.List(s.ctx, config.RedHatOrg, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(t, err)
	assert.Len(t, repoSnaps.Data, 1)
	assert.Equal(t, repoSnaps.Data[0].UUID, newerSnap)
	deletedSnap, err := s.dao.Snapshot.FetchModel(s.ctx, olderSnap, true)
	assert.Error(t, err)
	assert.Equal(t, models.Snapshot{}, deletedSnap)

	// Verify pulp deletion/cleanup
	resp1, _ := s.pulpClient.FindDistributionByPath(s.ctx, olderSnapModel.DistributionPath)
	assert.Nil(t, resp1)
	_, err = s.pulpClient.FindRpmPublicationByVersion(s.ctx, olderSnapModel.VersionHref)
	assert.Error(t, err)
	_, err = s.pulpClient.GetRpmRepositoryVersion(s.ctx, olderSnapModel.VersionHref)
	assert.Error(t, err)

	// Verify template uses the newer snapshot
	templates, err = s.dao.Template.InternalOnlyGetTemplatesForSnapshots(s.ctx, []string{newerSnap})
	assert.NoError(t, err)
	assert.Len(t, templates, 1)
	tempSnaps, _, err = s.dao.Snapshot.ListByTemplate(s.ctx, config.RedHatOrg, tempResp, "", api.PaginationData{Limit: -1})
	assert.NoError(t, err)
	assert.Len(t, tempSnaps.Data, 1)
	assert.Equal(t, tempSnaps.Data[0].UUID, newerSnap)

	// Verify the /latest serves the correct content
	rpmPath = fmt.Sprintf("%v%v/%v/latest/Packages/z", host, domain, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: config.RedHatOrg, Internal: identity.Internal{OrgID: config.RedHatOrg}}, 404)
	assert.NoError(s.T(), err)
	rpmPath = fmt.Sprintf("%v%v/%v/latest/Packages/f", host, domain, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: config.RedHatOrg, Internal: identity.Internal{OrgID: config.RedHatOrg}}, 200)
	assert.NoError(s.T(), err)
}

func (s *DeleteTest) TestDeleteSnapshotCleansUntrackedBlockingDistribution() {
	t := s.T()

	config.Get().Features.Snapshots.Enabled = true
	err := config.ConfigureTang()
	require.NoError(t, err)
	require.NotNil(t, config.Tang)
	orgID := uuid2.NewString()
	taskClient := client.NewTaskClient(&s.queue)
	domain, err := s.dao.Domain.FetchOrCreateDomain(s.ctx, orgID)
	require.NoError(t, err)
	s.pulpClient = pulp_client.GetPulpClientWithDomain(domain)
	require.NotNil(t, s.pulpClient)

	repo, err := s.dao.RepositoryConfig.Create(s.ctx, api.RepositoryRequest{
		Name:      utils.Ptr(uuid2.NewString()),
		URL:       utils.Ptr("https://fedorapeople.org/groups/katello/fakerepos/zoo/"),
		AccountID: utils.Ptr(orgID),
		OrgID:     utils.Ptr(orgID),
	})
	require.NoError(t, err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	require.NoError(t, err)
	s.snapshotAndWait(taskClient, repo, repoUuid, false)

	_, err = s.dao.RepositoryConfig.Update(s.ctx, orgID, repo.UUID, api.RepositoryUpdateRequest{
		URL: utils.Ptr("https://content-services.github.io/fixtures/yum/comps-modules/v1/"),
	})
	require.NoError(t, err)
	repo, err = s.dao.RepositoryConfig.Fetch(s.ctx, orgID, repo.UUID)
	require.NoError(t, err)
	s.snapshotAndWait(taskClient, repo, dao.UuidifyString(repo.RepositoryUUID), false)

	repoSnaps, _, err := s.dao.Snapshot.List(s.ctx, orgID, repo.UUID, api.PaginationData{
		Limit:  -1,
		SortBy: "created_at desc",
	}, api.FilterData{})
	require.NoError(t, err)
	require.Len(t, repoSnaps.Data, 2)

	olderSnapUUID, newerSnapUUID := snapshotUUIDsByAge(t, repoSnaps.Data)
	olderSnap, err := s.dao.Snapshot.FetchModel(s.ctx, olderSnapUUID, true)
	require.NoError(t, err)

	s.recreateUntrackedBlockingDistribution(olderSnap)

	err = s.dao.Snapshot.SoftDelete(s.ctx, olderSnapUUID)
	require.NoError(t, err)

	requestID := uuid2.NewString()
	taskUUID, err := taskClient.Enqueue(queue.Task{
		Typename: config.DeleteSnapshotsTask,
		Payload: utils.Ptr(payloads.DeleteSnapshotsPayload{
			RepoUUID:       repo.UUID,
			SnapshotsUUIDs: []string{olderSnapUUID},
		}),
		OrgId:      orgID,
		AccountId:  orgID,
		ObjectUUID: utils.Ptr(repo.UUID),
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		RequestID:  requestID,
	})
	require.NoError(t, err)
	s.WaitOnTask(taskUUID)

	repoSnaps, _, err = s.dao.Snapshot.List(s.ctx, orgID, repo.UUID, api.PaginationData{
		Limit:  -1,
		SortBy: "created_at desc",
	}, api.FilterData{})
	require.NoError(t, err)
	require.Len(t, repoSnaps.Data, 1)
	require.Equal(t, newerSnapUUID, repoSnaps.Data[0].UUID)
	deletedSnap, err := s.dao.Snapshot.FetchModel(s.ctx, olderSnapUUID, true)
	require.Error(t, err)
	require.Equal(t, models.Snapshot{}, deletedSnap)

	resp, err := s.pulpClient.FindDistributionByPath(s.ctx, olderSnap.DistributionPath)
	require.NoError(t, err)
	require.Nil(t, resp)

	_, err = s.pulpClient.FindRpmPublicationByVersion(s.ctx, olderSnap.VersionHref)
	require.Error(t, err)
	_, err = s.pulpClient.GetRpmRepositoryVersion(s.ctx, olderSnap.VersionHref)
	require.Error(t, err)
}

func (s *DeleteTest) recreateUntrackedBlockingDistribution(snap models.Snapshot) {
	t := s.T()

	dist, err := s.pulpClient.FindDistributionByPath(s.ctx, snap.DistributionPath)
	require.NoError(t, err)
	require.NotNil(t, dist)
	require.NotNil(t, dist.PulpHref)
	require.Equal(t, snap.DistributionHref, *dist.PulpHref)

	var contentGuardHref *string
	if dist.ContentGuard.IsSet() {
		contentGuardHref = dist.ContentGuard.Get()
	}

	deleteTaskHref, err := s.pulpClient.DeleteRpmDistribution(s.ctx, snap.DistributionHref)
	require.NoError(t, err)
	if deleteTaskHref != nil {
		_, err = s.pulpClient.PollTask(s.ctx, *deleteTaskHref)
		require.NoError(t, err)
	}

	gone, err := s.pulpClient.FindDistributionByPath(s.ctx, snap.DistributionPath)
	require.NoError(t, err)
	require.Nil(t, gone)

	createTaskHref, err := s.pulpClient.CreateRpmDistribution(
		s.ctx, snap.PublicationHref, dist.Name, snap.DistributionPath, contentGuardHref,
	)
	require.NoError(t, err)
	require.NotNil(t, createTaskHref)
	_, err = s.pulpClient.PollTask(s.ctx, *createTaskHref)
	require.NoError(t, err)

	newDist, err := s.pulpClient.FindDistributionByPath(s.ctx, snap.DistributionPath)
	require.NoError(t, err)
	require.NotNil(t, newDist)
	require.NotNil(t, newDist.PulpHref)
	assert.NotEqual(t, snap.DistributionHref, *newDist.PulpHref)
	require.True(t, newDist.Publication.IsSet())
	pub := newDist.Publication.Get()
	require.NotNil(t, pub)
	assert.Equal(t, snap.PublicationHref, *pub)
}

func snapshotUUIDsByAge(t *testing.T, snaps []api.SnapshotResponse) (olderUUID, newerUUID string) {
	t.Helper()
	require.NotEmpty(t, snaps)

	olderUUID = snaps[0].UUID
	newerUUID = snaps[0].UUID
	olderCreated := snaps[0].CreatedAt
	newerCreated := snaps[0].CreatedAt

	for _, snap := range snaps[1:] {
		if snap.CreatedAt.Before(olderCreated) {
			olderUUID = snap.UUID
			olderCreated = snap.CreatedAt
		}
		if snap.CreatedAt.After(newerCreated) {
			newerUUID = snap.UUID
			newerCreated = snap.CreatedAt
		}
	}

	return olderUUID, newerUUID
}

func (s *DeleteTest) TestDeleteCommunitySnapshot() {
	t := s.T()

	config.Get().Clients.Pulp.DownloadPolicy = "immediate"
	config.Get().Features.Snapshots.Enabled = true
	err := config.ConfigureTang()
	assert.NoError(t, err)
	assert.NotNil(t, config.Tang)
	orgID := uuid2.NewString()
	domain, err := s.dao.Domain.FetchOrCreateDomain(s.ctx, config.CommunityOrg)
	assert.NoError(t, err)
	s.pulpClient = pulp_client.GetPulpClientWithDomain(domain)
	assert.NotNil(t, s.pulpClient)
	s.dao = dao.GetDaoRegistry(db.DB)

	// Create and snapshot a "community" repo
	repo, _, err := s.createAndSyncRhelOrEpelRepo(false)
	require.NoError(t, err)

	// Update the repo so there are 2 snapshots
	opts := serveRepoOptions{
		port:         "30124",
		path:         "/" + strings.Split(repo.URL, "/")[3] + "/",
		repoSelector: "frog",
	}
	url2, cancelFunc, err := ServeRandomYumRepo(&opts)
	require.NoError(t, err)
	defer cancelFunc()
	_, err = s.dao.RepositoryConfig.Update(s.ctx, config.CommunityOrg, repo.UUID, api.RepositoryUpdateRequest{URL: &url2})
	assert.NoError(t, err)
	updatedRepo, err := s.dao.RepositoryConfig.Fetch(s.ctx, config.CommunityOrg, repo.UUID)
	assert.NoError(t, err)
	uuidStr, err := uuid2.Parse(updatedRepo.RepositoryUUID)
	assert.NoError(t, err)

	// Start the task
	taskClient := client.NewTaskClient(&s.queue)
	require.NoError(t, err)
	s.snapshotAndWait(taskClient, *repo, uuidStr, true)

	// Confirm there are 2 snapshots
	repoSnaps, _, err := s.dao.Snapshot.List(s.ctx, config.CommunityOrg, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(s.T(), err)
	assert.Len(s.T(), repoSnaps.Data, 2)

	// Create a template that uses the repo's older snapshot
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr(""),
		RepositoryUUIDS: []string{repo.UUID},
		OrgID:           utils.Ptr(orgID),
		UseLatest:       utils.Ptr(false),
		Date:            utils.Ptr(api.EmptiableDate(time.Now().AddDate(0, 0, -1))),
		Arch:            utils.Ptr(config.X8664),
		Version:         utils.Ptr(config.El8),
	}
	tempResp, err := s.dao.Template.Create(s.ctx, reqTemplate)
	assert.NoError(t, err)
	s.updateTemplateContentAndWait(orgID, tempResp.UUID, []string{repo.UUID})
	host, err := pulp_client.GetPulpClientWithDomain(domain).GetContentPath()
	assert.NoError(s.T(), err)

	olderSnap := repoSnaps.Data[1].UUID
	newerSnap := repoSnaps.Data[0].UUID

	// Verify the template is using the older snapshot
	templates, err := s.dao.Template.InternalOnlyGetTemplatesForSnapshots(s.ctx, []string{olderSnap})
	assert.NoError(t, err)
	assert.Len(t, templates, 1)
	tempSnaps, _, err := s.dao.Snapshot.ListByTemplate(s.ctx, config.CommunityOrg, tempResp, "", api.PaginationData{Limit: -1})
	assert.NoError(t, err)
	assert.Len(t, tempSnaps.Data, 1)
	assert.Equal(t, tempSnaps.Data[0].UUID, olderSnap)

	// Verify the /latest serves correct content
	rpmPath := fmt.Sprintf("%v%v/%v/latest/Packages/z", host, domain, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 404)
	assert.NoError(s.T(), err)
	rpmPath = fmt.Sprintf("%v%v/%v/latest/Packages/f", host, domain, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 200)
	assert.NoError(s.T(), err)

	// Soft-delete the older snapshot
	err = s.dao.Snapshot.SoftDelete(s.ctx, olderSnap)
	assert.NoError(t, err)
	olderSnapModel, err := s.dao.Snapshot.FetchModel(s.ctx, olderSnap, true)
	assert.NoError(t, err)

	// Start and wait for the delete-snapshots task
	requestID := uuid2.NewString()
	taskUUID, err := taskClient.Enqueue(queue.Task{
		Typename: config.DeleteSnapshotsTask,
		Payload: utils.Ptr(payloads.DeleteSnapshotsPayload{
			RepoUUID:       repo.UUID,
			SnapshotsUUIDs: []string{olderSnap},
		}),
		OrgId:      orgID,
		ObjectUUID: utils.Ptr(repo.UUID),
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
		RequestID:  requestID,
	})
	assert.NoError(t, err)
	s.WaitOnTask(taskUUID)

	// Verify the snapshot is deleted
	repoSnaps, _, err = s.dao.Snapshot.List(s.ctx, config.CommunityOrg, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(t, err)
	assert.Len(t, repoSnaps.Data, 1)
	assert.Equal(t, repoSnaps.Data[0].UUID, newerSnap)
	deletedSnap, err := s.dao.Snapshot.FetchModel(s.ctx, olderSnap, true)
	assert.Error(t, err)
	assert.Equal(t, models.Snapshot{}, deletedSnap)

	// Verify pulp deletion/cleanup
	resp1, _ := s.pulpClient.FindDistributionByPath(s.ctx, olderSnapModel.DistributionPath)
	assert.Nil(t, resp1)
	_, err = s.pulpClient.FindRpmPublicationByVersion(s.ctx, olderSnapModel.VersionHref)
	assert.Error(t, err)
	_, err = s.pulpClient.GetRpmRepositoryVersion(s.ctx, olderSnapModel.VersionHref)
	assert.Error(t, err)

	// Verify template uses the newer snapshot
	templates, err = s.dao.Template.InternalOnlyGetTemplatesForSnapshots(s.ctx, []string{newerSnap})
	assert.NoError(t, err)
	assert.Len(t, templates, 1)
	tempSnaps, _, err = s.dao.Snapshot.ListByTemplate(s.ctx, config.CommunityOrg, tempResp, "", api.PaginationData{Limit: -1})
	assert.NoError(t, err)
	assert.Len(t, tempSnaps.Data, 1)
	assert.Equal(t, tempSnaps.Data[0].UUID, newerSnap)

	// Verify the /latest serves the correct content
	rpmPath = fmt.Sprintf("%v%v/%v/latest/Packages/z", host, domain, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 404)
	assert.NoError(s.T(), err)
	rpmPath = fmt.Sprintf("%v%v/%v/latest/Packages/f", host, domain, repo.UUID)
	err = s.getRequest(rpmPath, identity.Identity{OrgID: orgID, Internal: identity.Internal{OrgID: orgID}}, 200)
	assert.NoError(s.T(), err)
}
