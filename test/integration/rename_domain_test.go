package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/jobs"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	uuid2 "github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type RenameDomainTestSuite struct {
	Suite
	dao      *dao.DaoRegistry
	cpClient candlepin_client.CandlepinClient
}

func TestRenameDomainTestSuite(t *testing.T) {
	suite.Run(t, new(RenameDomainTestSuite))
}

func (s *RenameDomainTestSuite) SetupTest() {
	s.Suite.SetupTest()

	s.cpClient = candlepin_client.NewCandlepinClient()
	s.dao = dao.GetDaoRegistry(s.db)

	// Force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"

	// Force content guard setup
	config.Get().Clients.Pulp.CustomRepoContentGuards = false
}

func (s *RenameDomainTestSuite) TestRenameDomain() {
	ctx := context.Background()
	orgID := uuid2.NewString()

	domainName, err := s.dao.Domain.FetchOrCreateDomain(ctx, orgID)
	assert.NoError(s.T(), err)

	repo1 := s.createAndSyncRepository(orgID, "https://fixtures.pulpproject.org/rpm-unsigned/")

	// Create initial template
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr(fmt.Sprintf("test template %v", rand.Int())),
		Description:     utils.Ptr("includes rpm unsigned"),
		RepositoryUUIDS: []string{repo1.UUID},
		OrgID:           utils.Ptr(orgID),
		Arch:            utils.Ptr(config.AARCH64),
		Version:         utils.Ptr(config.El8),
	}
	tempResp, err := s.dao.Template.Create(ctx, reqTemplate)
	assert.NoError(s.T(), err)
	assert.False(s.T(), tempResp.RHSMEnvironmentCreated)

	pulpClient := pulp_client.GetGlobalPulpClient()
	host, err := pulpClient.GetContentPath(ctx)
	require.NoError(s.T(), err)
	// force creation of the redhat domain in the db
	_, err = s.dao.Domain.FetchOrCreateDomain(ctx, config.RedHatOrg)
	assert.NoError(s.T(), err)

	_ = s.updateTemplateContentAndWait(orgID, tempResp.UUID, []string{repo1.UUID})

	// first rename the redhat domain
	newRHDomain := fmt.Sprintf("newrhDomain-%v", rand.Int())
	err = jobs.RenameDomain(ctx, s.db, s.dao, config.RedHatOrg, newRHDomain)
	assert.NoError(s.T(), err)

	// confirm it updated in our db
	updatedName, err := s.dao.Domain.Fetch(ctx, config.RedHatOrg)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), newRHDomain, updatedName)

	newDomainName := fmt.Sprintf("fooz-%v", rand.Int())
	err = jobs.RenameDomain(ctx, s.db, s.dao, orgID, newDomainName)
	require.NoError(s.T(), err)

	// Check candlepin env prefix was updated with new RH Domain
	env, err := s.cpClient.FetchEnvironment(ctx, tempResp.UUID)
	require.NoError(s.T(), err)
	newPrefix, err := config.EnvironmentPrefix(host, newRHDomain, tempResp.UUID)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), newPrefix, *env.ContentPrefix)

	// Check custom override was updated
	distPath1 := fmt.Sprintf("%v%s/templates/%v/%v", host, newDomainName, tempResp.UUID, repo1.UUID)
	overrides, err := s.cpClient.FetchContentOverrides(ctx, tempResp.UUID)
	require.NoError(s.T(), err)

	found := false
	for _, override := range overrides {
		if *override.Name == "baseurl" && *override.ContentLabel == repo1.Label {
			found = true
			assert.Equal(s.T(), distPath1, *override.Value)
		}
	}
	assert.True(s.T(), found)

	// check pulp domain
	newDomainHref, err := pulpClient.LookupDomain(ctx, newDomainName)
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), newDomainHref)

	oldDomainHref, err := pulpClient.LookupDomain(ctx, domainName)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), oldDomainHref)

	snapshots := []models.Snapshot{}
	err = s.db.Joins("RepositoryConfiguration").Where("org_id = ?", orgID).Find(&snapshots).Error
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), snapshots)
	for _, snapshot := range snapshots {
		assert.True(s.T(), strings.HasPrefix(snapshot.RepositoryPath, newDomainName))
		assert.True(s.T(), strings.Contains(snapshot.VersionHref, newDomainName))
		assert.True(s.T(), strings.Contains(snapshot.PublicationHref, newDomainName))
		assert.True(s.T(), strings.Contains(snapshot.DistributionHref, newDomainName))
	}

	// Check our db
	domainModel := models.Domain{}
	err = s.db.Where("org_id = ?", orgID).First(&domainModel).Error
	require.NoError(s.T(), err)
}

func (s *RenameDomainTestSuite) updateTemplateContentAndWait(orgId string, tempUUID string, repoConfigUUIDS []string) payloads.UpdateTemplateContentPayload {
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
