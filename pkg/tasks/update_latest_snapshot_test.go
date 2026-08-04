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
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestUpdateLatestSnapshotPartnerUsesPublishedSnapshotAcrossOrganizations(t *testing.T) {
	ctx := context.Background()
	reg := dao.GetMockDaoRegistry(t)
	pulp := pulp_client.NewMockPulpClient(t)
	contentGuards := config.Get().Clients.Pulp.RepoContentGuards
	config.Get().Clients.Pulp.RepoContentGuards = false
	t.Cleanup(func() { config.Get().Clients.Pulp.RepoContentGuards = contentGuards })

	taskOrg := "template-viewer-org"
	ownerOrg := "partner-owner-org"
	ownerDomain := "partner-owner-domain"
	repoUUID := uuid.NewString()
	templateUUID := uuid.NewString()
	publicationHref := "/pulp/publications/" + uuid.NewString()
	distHref := "/pulp/distributions/" + uuid.NewString()
	distPath := customTemplateSnapshotPath(templateUUID, repoUUID)
	distName := templateUUID + "/" + repoUUID

	repo := api.RepositoryResponse{UUID: repoUUID, OrgID: ownerOrg, Partner: true, Origin: config.OriginUpload}
	template := api.TemplateResponse{UUID: templateUUID, OrgID: taskOrg, UseLatest: true}
	snapshot := models.Snapshot{
		Base:                        models.Base{UUID: uuid.NewString()},
		RepositoryConfigurationUUID: repoUUID,
		PublicationHref:             publicationHref,
		Published:                   true,
	}

	reg.RepositoryConfig.On("FetchWithoutOrgID", ctx, repoUUID, false).Return(repo, nil)
	reg.Template.On("InternalOnlyGetTemplatesForRepoConfig", ctx, repoUUID, true).Return([]api.TemplateResponse{template}, nil)
	reg.Snapshot.On("HasPublishedSnapshot", ctx, repoUUID).Return(true, nil)
	reg.Snapshot.On("FetchLatestSnapshotForDistribution", ctx, repoUUID).Return(snapshot, nil)
	reg.Domain.On("Fetch", ctx, ownerOrg).Return(ownerDomain, nil)
	pulp.On("WithDomain", ownerDomain).Return(pulp)

	publication := zest.NullableString{}
	publication.Set(&publicationHref)
	distribution := &zest.RpmRpmDistributionResponse{PulpHref: &distHref, Name: distName, BasePath: distPath, Publication: publication}
	pulp.On("FindDistributionByPath", ctx, distPath).Return(distribution, nil).Twice()
	reg.Template.On("UpdateDistributionHrefs", ctx, templateUUID, []string{repoUUID}, []models.Snapshot{snapshot}, map[string]string{repoUUID: distHref}).Return(nil)
	reg.Template.On("UpdateSnapshots", ctx, templateUUID, []string{repoUUID}, []models.Snapshot{snapshot}).Return(nil)

	task := UpdateLatestSnapshot{
		daoReg:              reg.ToDaoRegistry(),
		ctx:                 ctx,
		orgID:               taskOrg,
		payload:             &UpdateLatestSnapshotPayload{RepositoryConfigUUID: repoUUID},
		pulpClient:          pulp,
		domainName:          "viewer-domain",
		rhDomainName:        "redhat-domain",
		communityDomainName: "community-domain",
	}
	require.NoError(t, task.Run())
}

func TestUpdateLatestSnapshotWithoutPublishedSnapshotsEnqueuesCleanupForEveryUseLatestTemplate(t *testing.T) {
	ctx := context.Background()
	reg := dao.GetMockDaoRegistry(t)
	mockQueue := queue.NewMockQueue(t)
	var taskQueue queue.Queue = mockQueue

	repoUUID := uuid.NewString()
	ownerOrg := "partner-owner"
	parentID := uuid.New()
	templates := []api.TemplateResponse{
		{UUID: uuid.NewString(), OrgID: ownerOrg, UseLatest: true},
		{UUID: uuid.NewString(), OrgID: "foreign-org", UseLatest: true},
	}
	repo := api.RepositoryResponse{UUID: repoUUID, OrgID: ownerOrg, Partner: true}
	parent := models.TaskInfo{
		Id:        parentID,
		AccountId: "account-id",
		RequestID: "request-id",
	}

	reg.RepositoryConfig.On("FetchWithoutOrgID", ctx, repoUUID, false).Return(repo, nil)
	reg.Template.On("InternalOnlyGetTemplatesForRepoConfig", ctx, repoUUID, true).Return(templates, nil)
	reg.Snapshot.On("HasPublishedSnapshot", ctx, repoUUID).Return(false, nil)

	var children []*queue.Task
	mockQueue.On("Enqueue", mock.Anything).Run(func(args mock.Arguments) {
		enqueued, ok := args.Get(0).(*queue.Task)
		require.True(t, ok)
		children = append(children, enqueued)
	}).Return(uuid.New(), nil).Twice()

	task := UpdateLatestSnapshot{
		daoReg:  reg.ToDaoRegistry(),
		ctx:     ctx,
		orgID:   ownerOrg,
		payload: &UpdateLatestSnapshotPayload{RepositoryConfigUUID: repoUUID},
		queue:   &taskQueue,
		task:    &parent,
	}
	require.NoError(t, task.Run())
	require.Len(t, children, len(templates))

	for i, child := range children {
		require.Equal(t, config.UpdateTemplateContentTask, child.Typename)
		require.Equal(t, templates[i].OrgID, child.OrgId)
		require.Equal(t, []uuid.UUID{parentID}, child.Dependencies)
		require.Equal(t, "account-id", child.AccountId)
		require.Equal(t, "request-id", child.RequestID)
		require.Equal(t, templates[i].UUID, *child.ObjectUUID)
		payload, ok := child.Payload.(payloads.UpdateTemplateContentPayload)
		require.True(t, ok)
		require.Equal(t, templates[i].UUID, payload.TemplateUUID)
		require.Empty(t, payload.RepoConfigUUIDs)
		require.NotNil(t, payload.TriggeredByRepositoryUUID)
		require.Equal(t, repoUUID, *payload.TriggeredByRepositoryUUID)
	}
	reg.Snapshot.AssertNotCalled(t, "FetchLatestSnapshotForDistribution", mock.Anything, mock.Anything)
}

func TestUpdateLatestSnapshotPublicationCheckErrorDoesNotEnqueueCleanup(t *testing.T) {
	ctx := context.Background()
	reg := dao.GetMockDaoRegistry(t)
	mockQueue := queue.NewMockQueue(t)
	var taskQueue queue.Queue = mockQueue
	repoUUID := uuid.NewString()
	expectedErr := fmt.Errorf("database unavailable")

	reg.RepositoryConfig.On("FetchWithoutOrgID", ctx, repoUUID, false).
		Return(api.RepositoryResponse{UUID: repoUUID, OrgID: "owner", Partner: true}, nil)
	reg.Template.On("InternalOnlyGetTemplatesForRepoConfig", ctx, repoUUID, true).
		Return([]api.TemplateResponse{{UUID: uuid.NewString(), OrgID: "foreign", UseLatest: true}}, nil)
	reg.Snapshot.On("HasPublishedSnapshot", ctx, repoUUID).Return(false, expectedErr)

	task := UpdateLatestSnapshot{
		daoReg: reg.ToDaoRegistry(), ctx: ctx, orgID: "owner",
		payload: &UpdateLatestSnapshotPayload{RepositoryConfigUUID: repoUUID},
		queue:   &taskQueue, task: &models.TaskInfo{Id: uuid.New()},
	}
	err := task.Run()
	require.ErrorIs(t, err, expectedErr)
	mockQueue.AssertNotCalled(t, "Enqueue", mock.Anything)
}

func TestRepositoryDomain(t *testing.T) {
	ctx := context.Background()
	reg := dao.GetMockDaoRegistry(t)
	viewerOrg := "viewer-org"
	viewerDomain := "viewer-domain"
	rhDomain := "redhat-domain"
	communityDomain := "community-domain"

	tests := []struct {
		name string
		repo api.RepositoryResponse
		want string
	}{
		{name: "red hat", repo: api.RepositoryResponse{OrgID: config.RedHatOrg}, want: rhDomain},
		{name: "community", repo: api.RepositoryResponse{OrgID: config.CommunityOrg}, want: communityDomain},
		{name: "owned custom", repo: api.RepositoryResponse{OrgID: viewerOrg}, want: viewerDomain},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repositoryDomain(ctx, reg.ToDaoRegistry(), tc.repo, viewerOrg, viewerDomain, rhDomain, communityDomain)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	reg.Domain.On("Fetch", ctx, "partner-owner").Return("partner-domain", nil)
	got, err := repositoryDomain(ctx, reg.ToDaoRegistry(), api.RepositoryResponse{OrgID: "partner-owner", Partner: true}, viewerOrg, viewerDomain, rhDomain, communityDomain)
	require.NoError(t, err)
	require.Equal(t, "partner-domain", got)
}
