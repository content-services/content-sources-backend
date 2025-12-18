package jobs

import (
	"os"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type DisableSnapshotForEpelReposSuite struct {
	suite.Suite
	db *gorm.DB
	tx *gorm.DB
}

func TestDisableSnapshotForEpelReposSuite(t *testing.T) {
	suite.Run(t, new(DisableSnapshotForEpelReposSuite))
}

func (s *DisableSnapshotForEpelReposSuite) SetupTest() {
	os.Setenv("EPEL_ORG_ID_SKIP", "-123")
	if db.DB == nil {
		if err := db.Connect(); err != nil {
			s.FailNow(err.Error())
		}
	}
	s.db = db.DB
	s.tx = s.db.Begin()
}

func (s *DisableSnapshotForEpelReposSuite) TearDownTest() {
	s.tx.Rollback()
}

// createEPELRepository creates a repository with an EPEL URL, or returns existing one if it already exists
func (s *DisableSnapshotForEpelReposSuite) createEPELRepository(url string) *models.Repository {
	cleanedURL := models.CleanupURL(url)
	repo := &models.Repository{}

	// Check if repository with this URL and origin already exists
	err := s.tx.Where("url = ? AND origin = ?", cleanedURL, config.OriginExternal).First(repo).Error
	if err == nil {
		// Repository already exists, return it
		return repo
	}

	// Repository doesn't exist, create it
	repo = &models.Repository{
		URL:    cleanedURL,
		Origin: config.OriginExternal,
	}
	err = s.tx.Create(repo).Error
	require.NoError(s.T(), err)
	return repo
}

// createRepositoryConfiguration creates a repository configuration with a unique name
func (s *DisableSnapshotForEpelReposSuite) createRepositoryConfiguration(orgID, repoUUID string, snapshot bool) *models.RepositoryConfiguration {
	// Generate a unique name to avoid constraint violations
	uniqueName := "Test Repo Config " + uuid.New().String()
	repoConfig := &models.RepositoryConfiguration{
		Name:           uniqueName,
		OrgID:          orgID,
		RepositoryUUID: repoUUID,
		Snapshot:       snapshot,
	}
	err := s.tx.Create(repoConfig).Error
	require.NoError(s.T(), err)
	return repoConfig
}

// createTemplate creates a template for an organization
func (s *DisableSnapshotForEpelReposSuite) createTemplate(orgID string) *models.Template {
	template := &models.Template{
		Name:    "Test Template",
		OrgID:   orgID,
		Version: config.El9,
		Arch:    config.X8664,
	}
	err := s.tx.Create(template).Error
	require.NoError(s.T(), err)
	return template
}

func (s *DisableSnapshotForEpelReposSuite) TestDisableSnapshotForEpelRepos_NoOrgsFound() {
	// Test when no organizations match the criteria
	// Set db.DB to use the transaction for the test
	originalDB := db.DB
	db.DB = s.tx
	defer func() { db.DB = originalDB }()

	DisableSnapshotForEpelRepos([]string{})

	// Verify no errors occurred (function should log and return)
	// This test mainly ensures the function doesn't panic
}

func (s *DisableSnapshotForEpelReposSuite) TestDisableSnapshotForEpelRepos_UpdatesOrgsWithoutTemplates() {
	// Set db.DB to use the transaction for the test
	originalDB := db.DB
	db.DB = s.tx
	defer func() { db.DB = originalDB }()

	// Create test data:
	// Org1: Has EPEL repo with snapshot=true, no template -> should be updated
	// Org2: Has EPEL repo with snapshot=true, has template -> should NOT be updated
	// Org3: Has EPEL repo with snapshot=false -> should NOT be updated
	// Org4: Has non-EPEL repo with snapshot=true, no template -> should NOT be updated

	org1 := seeds.RandomOrgId()
	org2 := seeds.RandomOrgId()
	org3 := seeds.RandomOrgId()
	org4 := seeds.RandomOrgId()

	// Org1: EPEL repo with snapshot=true, no template
	epelRepo1 := s.createEPELRepository("https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/")
	repoConfig1 := s.createRepositoryConfiguration(org1, epelRepo1.UUID, true)

	// Org2: EPEL repo with snapshot=true, has template
	epelRepo2 := s.createEPELRepository("https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/")
	repoConfig2 := s.createRepositoryConfiguration(org2, epelRepo2.UUID, true)
	_ = s.createTemplate(org2)

	// Org3: EPEL repo with snapshot=false
	epelRepo3 := s.createEPELRepository("https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/")
	repoConfig3 := s.createRepositoryConfiguration(org3, epelRepo3.UUID, false)

	// Org4: Non-EPEL repo with snapshot=true, no template
	nonEpelRepo := &models.Repository{
		URL:    "https://example.com/repo/",
		Origin: config.OriginExternal,
	}
	require.NoError(s.T(), s.tx.Create(nonEpelRepo).Error)
	repoConfig4 := s.createRepositoryConfiguration(org4, nonEpelRepo.UUID, true)

	// Run the function
	DisableSnapshotForEpelRepos([]string{})

	// Verify results
	var updatedRepoConfig1 models.RepositoryConfiguration
	err := s.tx.First(&updatedRepoConfig1, "uuid = ?", repoConfig1.UUID).Error
	require.NoError(s.T(), err)
	assert.False(s.T(), updatedRepoConfig1.Snapshot, "Org1 repo should have snapshot disabled")

	var updatedRepoConfig2 models.RepositoryConfiguration
	err = s.tx.First(&updatedRepoConfig2, "uuid = ?", repoConfig2.UUID).Error
	require.NoError(s.T(), err)
	assert.True(s.T(), updatedRepoConfig2.Snapshot, "Org2 repo should still have snapshot enabled (has template)")

	var updatedRepoConfig3 models.RepositoryConfiguration
	err = s.tx.First(&updatedRepoConfig3, "uuid = ?", repoConfig3.UUID).Error
	require.NoError(s.T(), err)
	assert.False(s.T(), updatedRepoConfig3.Snapshot, "Org3 repo should still have snapshot disabled (was already false)")

	var updatedRepoConfig4 models.RepositoryConfiguration
	err = s.tx.First(&updatedRepoConfig4, "uuid = ?", repoConfig4.UUID).Error
	require.NoError(s.T(), err)
	assert.True(s.T(), updatedRepoConfig4.Snapshot, "Org4 repo should still have snapshot enabled (not EPEL)")
}

func (s *DisableSnapshotForEpelReposSuite) TestDisableSnapshotForEpelRepos_SkipsRedHatAndCommunityOrgs() {
	// Set db.DB to use the transaction for the test
	originalDB := db.DB
	db.DB = s.tx
	defer func() { db.DB = originalDB }()

	// Create EPEL repos for RedHat and Community orgs
	redHatEpelRepo := s.createEPELRepository("https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/")
	redHatRepoConfig := s.createRepositoryConfiguration(config.RedHatOrg, redHatEpelRepo.UUID, true)

	communityEpelRepo := s.createEPELRepository("https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/")
	communityRepoConfig := s.createRepositoryConfiguration(config.CommunityOrg, communityEpelRepo.UUID, true)

	// Run the function
	DisableSnapshotForEpelRepos([]string{})

	// Verify RedHat and Community orgs are skipped
	var updatedRedHatRepoConfig models.RepositoryConfiguration
	err := s.tx.First(&updatedRedHatRepoConfig, "uuid = ?", redHatRepoConfig.UUID).Error
	require.NoError(s.T(), err)
	assert.True(s.T(), updatedRedHatRepoConfig.Snapshot, "RedHat org repo should still have snapshot enabled (should be skipped)")

	var updatedCommunityRepoConfig models.RepositoryConfiguration
	err = s.tx.First(&updatedCommunityRepoConfig, "uuid = ?", communityRepoConfig.UUID).Error
	require.NoError(s.T(), err)
	assert.True(s.T(), updatedCommunityRepoConfig.Snapshot, "Community org repo should still have snapshot enabled (should be skipped)")
}

func (s *DisableSnapshotForEpelReposSuite) TestDisableSnapshotForEpelRepos_MultipleReposPerOrg() {
	// Set db.DB to use the transaction for the test
	originalDB := db.DB
	db.DB = s.tx
	defer func() { db.DB = originalDB }()

	// Create an org with multiple EPEL repos
	orgID := seeds.RandomOrgId()

	epelRepo1 := s.createEPELRepository("https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/")
	repoConfig1 := s.createRepositoryConfiguration(orgID, epelRepo1.UUID, true)

	epelRepo2 := s.createEPELRepository("https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/")
	repoConfig2 := s.createRepositoryConfiguration(orgID, epelRepo2.UUID, true)

	epelRepo3 := s.createEPELRepository("https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/")
	repoConfig3 := s.createRepositoryConfiguration(orgID, epelRepo3.UUID, true)

	// Run the function
	DisableSnapshotForEpelRepos([]string{})

	// Verify all repos for this org are updated
	var updatedRepoConfig1 models.RepositoryConfiguration
	err := s.tx.First(&updatedRepoConfig1, "uuid = ?", repoConfig1.UUID).Error
	require.NoError(s.T(), err)
	assert.False(s.T(), updatedRepoConfig1.Snapshot, "Repo1 should have snapshot disabled")

	var updatedRepoConfig2 models.RepositoryConfiguration
	err = s.tx.First(&updatedRepoConfig2, "uuid = ?", repoConfig2.UUID).Error
	require.NoError(s.T(), err)
	assert.False(s.T(), updatedRepoConfig2.Snapshot, "Repo2 should have snapshot disabled")

	var updatedRepoConfig3 models.RepositoryConfiguration
	err = s.tx.First(&updatedRepoConfig3, "uuid = ?", repoConfig3.UUID).Error
	require.NoError(s.T(), err)
	assert.False(s.T(), updatedRepoConfig3.Snapshot, "Repo3 should have snapshot disabled")
}

func (s *DisableSnapshotForEpelReposSuite) TestDisableSnapshotForEpelRepos_MultipleOrgs() {
	// Set db.DB to use the transaction for the test
	originalDB := db.DB
	db.DB = s.tx
	defer func() { db.DB = originalDB }()

	// Create multiple orgs, some with templates, some without
	org1 := seeds.RandomOrgId()
	org2 := seeds.RandomOrgId()
	org3 := seeds.RandomOrgId()

	// Org1: EPEL repo, no template
	epelRepo1 := s.createEPELRepository("https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/")
	repoConfig1 := s.createRepositoryConfiguration(org1, epelRepo1.UUID, true)

	// Org2: EPEL repo, has template (should be skipped)
	epelRepo2 := s.createEPELRepository("https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/")
	repoConfig2 := s.createRepositoryConfiguration(org2, epelRepo2.UUID, true)
	_ = s.createTemplate(org2)

	// Org3: EPEL repo, no template
	epelRepo3 := s.createEPELRepository("https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/")
	repoConfig3 := s.createRepositoryConfiguration(org3, epelRepo3.UUID, true)

	// Run the function
	DisableSnapshotForEpelRepos([]string{})

	// Verify org1 and org3 are updated, org2 is not
	var updatedRepoConfig1 models.RepositoryConfiguration
	err := s.tx.First(&updatedRepoConfig1, "uuid = ?", repoConfig1.UUID).Error
	require.NoError(s.T(), err)
	assert.False(s.T(), updatedRepoConfig1.Snapshot, "Org1 repo should have snapshot disabled")

	var updatedRepoConfig2 models.RepositoryConfiguration
	err = s.tx.First(&updatedRepoConfig2, "uuid = ?", repoConfig2.UUID).Error
	require.NoError(s.T(), err)
	assert.True(s.T(), updatedRepoConfig2.Snapshot, "Org2 repo should still have snapshot enabled (has template)")

	var updatedRepoConfig3 models.RepositoryConfiguration
	err = s.tx.First(&updatedRepoConfig3, "uuid = ?", repoConfig3.UUID).Error
	require.NoError(s.T(), err)
	assert.False(s.T(), updatedRepoConfig3.Snapshot, "Org3 repo should have snapshot disabled")
}

func (s *DisableSnapshotForEpelReposSuite) TestDisableSnapshotForEpelRepos_SkipsOrgInEPEL_ORG_ID_SKIP() {
	// Set db.DB to use the transaction for the test
	originalDB := db.DB
	db.DB = s.tx
	defer func() { db.DB = originalDB }()

	// Create an org ID and set it in EPEL_ORG_ID_SKIP
	orgID := seeds.RandomOrgId()
	os.Setenv("EPEL_ORG_ID_SKIP", orgID+",123")

	// Create EPEL repo with snapshot=true for this org
	epelRepo := s.createEPELRepository("https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/")
	repoConfig := s.createRepositoryConfiguration(orgID, epelRepo.UUID, true)

	// Run the function
	DisableSnapshotForEpelRepos([]string{})

	// Verify the repo is NOT changed (snapshot should still be true)
	var updatedRepoConfig models.RepositoryConfiguration
	err := s.tx.First(&updatedRepoConfig, "uuid = ?", repoConfig.UUID).Error
	require.NoError(s.T(), err)
	assert.True(s.T(), updatedRepoConfig.Snapshot, "Org repo should still have snapshot enabled (org ID is in EPEL_ORG_ID_SKIP)")
}
