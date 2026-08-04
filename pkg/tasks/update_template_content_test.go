package tasks

import (
	"context"
	"fmt"
	"testing"
	"time"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	candlepin_client "github.com/content-services/content-sources-backend/pkg/clients/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/helpers"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UpdateTemplateContentSuite struct {
	suite.Suite
}

func TestUpdateTemplateContentSuiteSuite(t *testing.T) {
	suite.Run(t, new(UpdateTemplateContentSuite))
}

func (s *UpdateTemplateContentSuite) TestGetDistributionPath() {
	repoUUID := "repo-uuid"
	templateUUID := "template-uuid"
	url := "http://example.com/red/hat/repo/path/"
	expectedRhPath := fmt.Sprintf("templates/%v/%v", templateUUID, "red/hat/repo/path")
	expectedCustomPath := fmt.Sprintf("templates/%v/%v", templateUUID, repoUUID)
	expectedName := templateUUID + "/" + repoUUID

	repo := api.RepositoryResponse{UUID: repoUUID, URL: url, OrgID: config.RedHatOrg}
	distPath, distName, err := getDistPathAndName(repo, templateUUID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), expectedRhPath, distPath)
	assert.Equal(s.T(), expectedName, distName)

	repo = api.RepositoryResponse{UUID: repoUUID, URL: url, OrgID: "12345"}
	distPath, _, err = getDistPathAndName(repo, templateUUID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), expectedCustomPath, distPath)
	assert.Equal(s.T(), expectedName, distName)
}

func (s *UpdateTemplateContentSuite) TestNormalizeExtendedReleaseLabel() {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "e4s label with minor version",
			input:    "rhel-8.6-for-x86_64-appstream-e4s-rpms",
			expected: "rhel-8-for-x86_64-appstream-e4s-rpms",
		},
		{
			name:     "eus label with minor version",
			input:    "rhel-9.4-for-x86_64-appstream-eus-rpms",
			expected: "rhel-9-for-x86_64-appstream-eus-rpms",
		},
		{
			name:     "regular rhel label without extended release",
			input:    "rhel-8-for-x86_64-appstream-rpms",
			expected: "rhel-8-for-x86_64-appstream-rpms",
		},
		{
			name:     "already normalized e4s label",
			input:    "rhel-9-for-x86_64-appstream-e4s-rpms",
			expected: "rhel-9-for-x86_64-appstream-e4s-rpms",
		},
		{
			name:     "custom repository label",
			input:    "custom-repo-label",
			expected: "custom-repo-label",
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			result := normalizeExtendedReleaseLabel(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func (s *UpdateTemplateContentSuite) TestForeignPartnerWithoutLastSnapshotUsesOwnerDomain() {
	ctx := context.Background()
	reg := dao.GetMockDaoRegistry(s.T())
	pulp := pulp_client.NewMockPulpClient(s.T())
	contentGuards := config.Get().Clients.Pulp.RepoContentGuards
	config.Get().Clients.Pulp.RepoContentGuards = true
	s.T().Cleanup(func() { config.Get().Clients.Pulp.RepoContentGuards = contentGuards })

	viewerOrg := "template-owner"
	ownerOrg := "partner-owner"
	ownerDomain := "partner-domain"
	repoUUID := uuid.NewString()
	templateUUID := uuid.NewString()
	publicationHref := "/pulp/publications/" + uuid.NewString()
	distHref := "/pulp/distributions/" + uuid.NewString()
	distPath := customTemplateSnapshotPath(templateUUID, repoUUID)

	repo := api.RepositoryResponse{UUID: repoUUID, OrgID: ownerOrg, Partner: true, Origin: config.OriginUpload, LastSnapshot: nil}
	snapshot := models.Snapshot{Base: models.Base{UUID: uuid.NewString()}, RepositoryConfigurationUUID: repoUUID, PublicationHref: publicationHref, Published: true}

	reg.RepositoryConfig.On("FetchWithoutOrgID", ctx, repoUUID, false).Return(repo, nil)
	reg.Domain.On("Fetch", ctx, ownerOrg).Return(ownerDomain, nil)
	pulp.On("WithDomain", ownerDomain).Return(pulp)
	publication := zest.NullableString{}
	publication.Set(&publicationHref)
	pulp.On("FindDistributionByPath", ctx, distPath).Return(&zest.RpmRpmDistributionResponse{PulpHref: &distHref, Publication: publication}, nil)

	task := UpdateTemplateContent{
		orgId:               viewerOrg,
		domainName:          "viewer-domain",
		rhDomainName:        "redhat-domain",
		communityDomainName: "community-domain",
		daoReg:              reg.ToDaoRegistry(),
		pulpClient:          pulp,
		payload:             &payloads.UpdateTemplateContentPayload{TemplateUUID: templateUUID},
		ctx:                 ctx,
	}
	distributions := map[string]string{}
	require.NoError(s.T(), task.handleReposAdded([]string{repoUUID}, []models.Snapshot{snapshot}, distributions))
	_, found := distributions[repoUUID]
	assert.True(s.T(), found)
}

func (s *UpdateTemplateContentSuite) TestForeignPartnerUnpublishedSnapshotFailsClosed() {
	ctx := context.Background()
	reg := dao.GetMockDaoRegistry(s.T())
	pulp := pulp_client.NewMockPulpClient(s.T())

	viewerOrg := "template-owner"
	ownerOrg := "partner-owner"
	repoUUID := uuid.NewString()
	templateUUID := uuid.NewString()
	repo := api.RepositoryResponse{UUID: repoUUID, OrgID: ownerOrg, Partner: true, Origin: config.OriginUpload}
	snapshot := models.Snapshot{
		Base:                        models.Base{UUID: uuid.NewString()},
		RepositoryConfigurationUUID: repoUUID,
		PublicationHref:             "/pulp/publications/" + uuid.NewString(),
		Published:                   false,
	}

	reg.RepositoryConfig.On("FetchWithoutOrgID", ctx, repoUUID, false).Return(repo, nil)
	reg.Domain.On("Fetch", ctx, ownerOrg).Return("partner-domain", nil)
	pulp.On("WithDomain", "partner-domain").Return(pulp)

	task := UpdateTemplateContent{
		orgId:      viewerOrg,
		daoReg:     reg.ToDaoRegistry(),
		pulpClient: pulp,
		payload:    &payloads.UpdateTemplateContentPayload{TemplateUUID: templateUUID},
		ctx:        ctx,
	}

	err := task.handleReposAdded([]string{repoUUID}, []models.Snapshot{snapshot}, map[string]string{})
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "unpublished snapshot")
}

func (s *UpdateTemplateContentSuite) TestSameOrgPartnerKeepsContentGuard() {
	ctx := context.Background()
	pulp := pulp_client.NewMockPulpClient(s.T())
	contentGuards := config.Get().Clients.Pulp.RepoContentGuards
	config.Get().Clients.Pulp.RepoContentGuards = true
	s.T().Cleanup(func() { config.Get().Clients.Pulp.RepoContentGuards = contentGuards })

	orgID := "partner-owner"
	publicationHref := "/pulp/publications/" + uuid.NewString()
	guardHref := "/pulp/content-guards/" + uuid.NewString()
	distHref := "/pulp/distributions/" + uuid.NewString()
	publication := zest.NullableString{}
	publication.Set(&publicationHref)
	contentGuard := zest.NullableString{}
	contentGuard.Set(&guardHref)

	pulp.On("CreateOrUpdateGuardsForOrg", ctx, orgID).Return(guardHref, nil)
	pulp.On("FindDistributionByPath", ctx, "template-path").Return(&zest.RpmRpmDistributionResponse{
		PulpHref:     &distHref,
		Publication:  publication,
		ContentGuard: contentGuard,
	}, nil)

	pdh := helpers.NewPulpDistributionHelper(ctx, pulp)
	_, _, err := pdh.CreateOrUpdateTemplateDistribution(
		api.RepositoryResponse{UUID: uuid.NewString(), OrgID: orgID, Partner: true},
		models.Snapshot{Base: models.Base{UUID: uuid.NewString()}, PublicationHref: publicationHref},
		orgID,
		"template-name",
		"template-path",
	)
	require.NoError(s.T(), err)
}

func (s *UpdateTemplateContentSuite) TestForeignPartnerUploadContentUsesOwnerDomain() {
	ctx := context.Background()
	reg := dao.GetMockDaoRegistry(s.T())
	cp := candlepin_client.NewMockCandlepinClient(s.T())

	viewerOrg := "template-owner"
	ownerOrg := "partner-owner"
	repoUUID := uuid.NewString()
	repo := api.RepositoryResponse{
		UUID:              repoUUID,
		Name:              "Partner upload",
		Label:             "partner-upload",
		OrgID:             ownerOrg,
		Origin:            config.OriginUpload,
		Partner:           true,
		LastSnapshot:      nil,
		LatestSnapshotURL: "",
	}

	reg.RepositoryConfig.On("FetchWithoutOrgID", ctx, repoUUID, false).Return(repo, nil)
	reg.Domain.On("Fetch", ctx, ownerOrg).Return("partner-owner-domain", nil)
	cp.On("FetchContentsByLabel", ctx, viewerOrg, []string{}).Return([]caliri.ContentDTO{}, nil)

	task := UpdateTemplateContent{
		orgId:      viewerOrg,
		domainName: "viewer-domain",
		daoReg:     reg.ToDaoRegistry(),
		cpClient:   cp,
		payload:    &payloads.UpdateTemplateContentPayload{TemplateUUID: uuid.NewString(), RepoConfigUUIDs: []string{repoUUID}},
		ctx:        ctx,
	}

	content, _, _, err := task.getContentList("http://pulp.example/api/pulp-content")
	require.NoError(s.T(), err)
	require.Len(s.T(), content, 1)
	require.NotNil(s.T(), content[0].ContentUrl)
	assert.Equal(s.T(), "http://pulp.example/api/pulp-content/partner-owner-domain/"+repoUUID+"/latest/", *content[0].ContentUrl)
}

func (s *UpdateTemplateContentSuite) TestRunPulpMakesSnapshotUpdateDecision() {
	ctx := context.Background()
	reg := dao.GetMockDaoRegistry(s.T())
	pulp := pulp_client.NewMockPulpClient(s.T())
	contentGuards := config.Get().Clients.Pulp.RepoContentGuards
	config.Get().Clients.Pulp.RepoContentGuards = false
	s.T().Cleanup(func() { config.Get().Clients.Pulp.RepoContentGuards = contentGuards })

	viewerOrg := "template-owner"
	ownerOrg := "partner-owner"
	ownerDomain := "partner-domain"
	repoUUID := uuid.NewString()
	templateUUID := uuid.NewString()
	snapshotUUID := uuid.NewString()
	publicationHref := "/pulp/publications/" + uuid.NewString()
	distHref := "/pulp/distributions/" + uuid.NewString()
	distPath := customTemplateSnapshotPath(templateUUID, repoUUID)
	templateDate := time.Now().Add(-time.Hour)

	repo := api.RepositoryResponse{UUID: repoUUID, OrgID: ownerOrg, Partner: true, Origin: config.OriginUpload}
	snapshot := models.Snapshot{
		Base:                        models.Base{UUID: snapshotUUID},
		RepositoryConfigurationUUID: repoUUID,
		PublicationHref:             publicationHref,
		Published:                   true,
	}
	publication := zest.NullableString{}
	publication.Set(&publicationHref)
	distribution := &zest.RpmRpmDistributionResponse{PulpHref: &distHref, Publication: publication}

	reg.Template.On("GetRepoChanges", ctx, templateUUID, []string{repoUUID}).
		Return([]string(nil), []string(nil), []string{repoUUID}, []string{repoUUID}, nil)
	reg.Snapshot.On("FetchSnapshotsModelByDateAndRepository", ctx, viewerOrg, mock.MatchedBy(func(req api.ListSnapshotByDateRequest) bool {
		return req.Date.Equal(templateDate) && len(req.RepositoryUUIDS) == 1 && req.RepositoryUUIDS[0] == repoUUID
	})).Return([]models.Snapshot{snapshot}, nil)
	reg.RepositoryConfig.On("FetchWithoutOrgID", ctx, repoUUID, false).Return(repo, nil)
	reg.Domain.On("Fetch", ctx, ownerOrg).Return(ownerDomain, nil)
	pulp.On("WithDomain", ownerDomain).Return(pulp)
	// The selected snapshot already backs the distribution, so the task must not issue a Pulp update.
	pulp.On("FindDistributionByPath", ctx, distPath).Return(distribution, nil).Twice()
	reg.Template.On("UpdateDistributionHrefs", ctx, templateUUID, []string{repoUUID}, []models.Snapshot{snapshot}, map[string]string{repoUUID: distHref}).Return(nil)
	reg.Template.On("UpdateSnapshots", ctx, templateUUID, []string{repoUUID}, []models.Snapshot{snapshot}).Return(nil)

	task := UpdateTemplateContent{
		orgId:               viewerOrg,
		domainName:          "viewer-domain",
		rhDomainName:        "redhat-domain",
		communityDomainName: "community-domain",
		template:            api.TemplateResponse{UUID: templateUUID, OrgID: viewerOrg, Date: templateDate},
		daoReg:              reg.ToDaoRegistry(),
		pulpClient:          pulp,
		payload:             &payloads.UpdateTemplateContentPayload{TemplateUUID: templateUUID, RepoConfigUUIDs: []string{repoUUID}},
		ctx:                 ctx,
	}
	require.NoError(s.T(), task.RunPulp())
}

func (s *UpdateTemplateContentSuite) TestRunPulpRemovesForeignPartnerAndPreservesBaseRepository() {
	ctx := context.Background()
	reg := dao.GetMockDaoRegistry(s.T())
	pulp := pulp_client.NewMockPulpClient(s.T())
	contentGuards := config.Get().Clients.Pulp.RepoContentGuards
	config.Get().Clients.Pulp.RepoContentGuards = false
	s.T().Cleanup(func() { config.Get().Clients.Pulp.RepoContentGuards = contentGuards })

	templateUUID := uuid.NewString()
	baseRepoUUID := uuid.NewString()
	partnerRepoUUID := uuid.NewString()
	baseSnapshot := models.Snapshot{
		Base:                        models.Base{UUID: uuid.NewString()},
		RepositoryConfigurationUUID: baseRepoUUID,
		PublicationHref:             "/pulp/publications/" + uuid.NewString(),
	}
	baseRepo := api.RepositoryResponse{
		UUID:  baseRepoUUID,
		OrgID: config.RedHatOrg,
		URL:   "https://cdn.redhat.com/content/dist/rhel9/baseos/x86_64/os/",
	}
	partnerRepo := api.RepositoryResponse{UUID: partnerRepoUUID, OrgID: "partner-owner", Partner: true, Origin: config.OriginUpload}
	partnerDistHref := "/pulp/distributions/" + uuid.NewString()
	baseDistHref := "/pulp/distributions/" + uuid.NewString()
	baseDistPath, _, err := getDistPathAndName(baseRepo, templateUUID)
	require.NoError(s.T(), err)
	publication := zest.NullableString{}
	publication.Set(&baseSnapshot.PublicationHref)

	reg.Template.On("GetRepoChanges", ctx, templateUUID, []string{baseRepoUUID}).
		Return([]string(nil), []string{partnerRepoUUID}, []string{baseRepoUUID}, []string{baseRepoUUID, partnerRepoUUID}, nil)
	reg.Snapshot.On("FetchSnapshotsModelByDateAndRepository", ctx, "template-owner", mock.Anything).
		Return([]models.Snapshot{baseSnapshot}, nil)
	reg.RepositoryConfig.On("FetchWithoutOrgID", ctx, partnerRepoUUID, false).Return(partnerRepo, nil)
	reg.Domain.On("Fetch", ctx, partnerRepo.OrgID).Return("partner-domain", nil)
	pulp.On("WithDomain", "partner-domain").Return(pulp)
	reg.Template.On("GetDistributionHref", ctx, templateUUID, partnerRepoUUID).Return(&partnerDistHref, nil)
	pulp.On("DeleteRpmDistribution", ctx, partnerDistHref).Return((*string)(nil), nil)
	reg.Template.On("DeleteTemplateRepoConfigs", ctx, templateUUID, []string{baseRepoUUID}).Return(nil)

	reg.RepositoryConfig.On("FetchWithoutOrgID", ctx, baseRepoUUID, false).Return(baseRepo, nil)
	pulp.On("WithDomain", "redhat-domain").Return(pulp)
	pulp.On("FindDistributionByPath", ctx, baseDistPath).
		Return(&zest.RpmRpmDistributionResponse{PulpHref: &baseDistHref, Publication: publication}, nil).Twice()
	reg.Template.On("UpdateDistributionHrefs", ctx, templateUUID, []string{baseRepoUUID}, []models.Snapshot{baseSnapshot}, map[string]string{baseRepoUUID: baseDistHref}).Return(nil)
	reg.Template.On("UpdateSnapshots", ctx, templateUUID, []string{baseRepoUUID}, []models.Snapshot{baseSnapshot}).Return(nil)

	taskInfo := &models.TaskInfo{Id: uuid.New(), Typename: config.UpdateTemplateContentTask}
	task := UpdateTemplateContent{
		orgId:               "template-owner",
		domainName:          "viewer-domain",
		rhDomainName:        "redhat-domain",
		communityDomainName: "community-domain",
		template:            api.TemplateResponse{UUID: templateUUID, OrgID: "template-owner", Date: time.Now()},
		daoReg:              reg.ToDaoRegistry(),
		pulpClient:          pulp,
		payload:             &payloads.UpdateTemplateContentPayload{TemplateUUID: templateUUID, RepoConfigUUIDs: []string{baseRepoUUID}},
		task:                taskInfo,
		ctx:                 ctx,
	}
	require.NoError(s.T(), task.RunPulp())
}

func (s *UpdateTemplateContentSuite) TestPartnerOverrideUsesOwnerDomainWithoutLastSnapshot() {
	ctx := context.Background()
	reg := dao.GetMockDaoRegistry(s.T())
	repoUUID := uuid.NewString()
	templateUUID := uuid.NewString()
	repo := api.RepositoryResponse{
		UUID:         repoUUID,
		OrgID:        "partner-owner",
		Partner:      true,
		Origin:       config.OriginUpload,
		Label:        "partner-label",
		LastSnapshot: nil,
	}
	reg.RepositoryConfig.On("FetchWithoutOrgID", ctx, repoUUID, false).Return(repo, nil)
	reg.Domain.On("Fetch", ctx, repo.OrgID).Return("partner-domain", nil)

	overrides, err := GenOverrideDTO(ctx, reg.ToDaoRegistry(), "template-owner", "viewer-domain", "redhat-domain", "community-domain", "/content", api.TemplateResponse{UUID: templateUUID}, []string{repoUUID})
	require.NoError(s.T(), err)
	baseURL := findOverride(overrides, "baseurl")
	require.NotNil(s.T(), baseURL)
	assert.Equal(s.T(), "/content/partner-domain/templates/"+templateUUID+"/"+repoUUID, *baseURL.Value)
}

func (s *UpdateTemplateContentSuite) TestPrepareTriggeredUpdateDecisionMatrix() {
	tests := []struct {
		name         string
		templateOrg  string
		repoOwnerOrg string
		useLatest    bool
		hasPublished bool
		associated   bool
		wantRepos    []string
		wantSkip     bool
	}{
		{name: "foreign fixed removes without published", templateOrg: "viewer", repoOwnerOrg: "owner", wantRepos: []string{"base"}, associated: true},
		{name: "owner fixed retains without published", templateOrg: "owner", repoOwnerOrg: "owner", wantRepos: []string{"base", "partner"}, associated: true},
		{name: "owner use latest removes without published", templateOrg: "owner", repoOwnerOrg: "owner", useLatest: true, wantRepos: []string{"base"}, associated: true},
		{name: "foreign fixed retains with published", templateOrg: "viewer", repoOwnerOrg: "owner", hasPublished: true, wantRepos: []string{"base", "partner"}, associated: true},
		{name: "use latest defers when publication reappears", templateOrg: "viewer", repoOwnerOrg: "owner", useLatest: true, hasPublished: true, wantRepos: []string{"base", "partner"}, wantSkip: true, associated: true},
		{name: "already removed is never re-added", templateOrg: "viewer", repoOwnerOrg: "owner", wantRepos: []string{"base"}},
	}

	for _, tc := range tests {
		s.T().Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			reg := dao.GetMockDaoRegistry(t)
			trigger := "partner"
			repositories := []string{"base"}
			if tc.associated {
				repositories = append(repositories, trigger)
			}
			reg.RepositoryConfig.On("FetchWithoutOrgID", ctx, trigger, false).
				Return(api.RepositoryResponse{UUID: trigger, OrgID: tc.repoOwnerOrg, Partner: true}, nil)
			reg.Snapshot.On("HasPublishedSnapshot", ctx, trigger).Return(tc.hasPublished, nil)

			task := UpdateTemplateContent{
				ctx:      ctx,
				daoReg:   reg.ToDaoRegistry(),
				template: api.TemplateResponse{UUID: "template", OrgID: tc.templateOrg, UseLatest: tc.useLatest, RepositoryUUIDS: repositories},
				payload:  &payloads.UpdateTemplateContentPayload{TemplateUUID: "template", TriggeredByRepositoryUUID: &trigger},
			}
			skip, err := task.prepareTriggeredUpdate()
			require.NoError(t, err)
			require.Equal(t, tc.wantSkip, skip)
			require.Equal(t, tc.wantRepos, task.payload.RepoConfigUUIDs)
			require.Equal(t, tc.wantRepos, task.template.RepositoryUUIDS)
		})
	}
}

func (s *UpdateTemplateContentSuite) TestPrepareTriggeredUpdateFailsClosedOnPublicationCheckError() {
	ctx := context.Background()
	reg := dao.GetMockDaoRegistry(s.T())
	trigger := "partner"
	expectedErr := fmt.Errorf("database unavailable")
	reg.RepositoryConfig.On("FetchWithoutOrgID", ctx, trigger, false).
		Return(api.RepositoryResponse{UUID: trigger, OrgID: "owner", Partner: true}, nil)
	reg.Snapshot.On("HasPublishedSnapshot", ctx, trigger).Return(false, expectedErr)

	task := UpdateTemplateContent{
		ctx:      ctx,
		daoReg:   reg.ToDaoRegistry(),
		template: api.TemplateResponse{UUID: "template", OrgID: "viewer", RepositoryUUIDS: []string{"base", trigger}},
		payload:  &payloads.UpdateTemplateContentPayload{TemplateUUID: "template", TriggeredByRepositoryUUID: &trigger},
	}
	_, err := task.prepareTriggeredUpdate()
	require.ErrorIs(s.T(), err, expectedErr)
	require.Empty(s.T(), task.payload.RepoConfigUUIDs)
}

func findOverride(overrides []caliri.ContentOverrideDTO, name string) *caliri.ContentOverrideDTO {
	for i := range overrides {
		if overrides[i].Name != nil && *overrides[i].Name == name {
			return &overrides[i]
		}
	}
	return nil
}
