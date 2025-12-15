package integration

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"path"
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
	"github.com/rs/zerolog/log"
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
	taskClient := client.NewTaskClient(s.queue)

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
	taskClient := client.NewTaskClient(s.queue)

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
	taskClient := client.NewTaskClient(s.queue)
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

	host, err := pulp_client.GetPulpClientWithDomain(domain).GetContentPath(s.ctx)
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
	taskClient := client.NewTaskClient(s.queue)
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
	host, err := pulp_client.GetPulpClientWithDomain(domain).GetContentPath(s.ctx)
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

func (s *DeleteTest) TestDeleteCommunitySnapshot() {
	t := s.T()

	config.Get().Clients.Pulp.DownloadPolicy = "immediate"
	config.Get().Features.Snapshots.Enabled = true
	config.Get().Features.CommunityRepos.Enabled = true
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
	taskClient := client.NewTaskClient(s.queue)
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
	host, err := pulp_client.GetPulpClientWithDomain(domain).GetContentPath(s.ctx)
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

func (s *DeleteTest) WaitOnTask(taskUuid uuid2.UUID) {
	// Poll until the task is complete
	taskInfo, err := s.queue.Status(taskUuid)
	assert.NoError(s.T(), err)
	for {
		if taskInfo.Status == config.TaskStatusRunning || taskInfo.Status == config.TaskStatusPending {
			log.Logger.Error().Msg("SLEEPING")
			time.Sleep(1 * time.Second)
		} else {
			break
		}
		taskInfo, err = s.queue.Status(taskUuid)
		assert.NoError(s.T(), err)
	}
	if taskInfo.Error != nil {
		assert.Nil(s.T(), *taskInfo.Error)
	}

	assert.Equal(s.T(), config.TaskStatusCompleted, taskInfo.Status)
}
