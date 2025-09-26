package dao

import (
	"log"
	"os"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DaoSuite struct {
	suite.Suite
	db                        *gorm.DB
	tx                        *gorm.DB
	skipDefaultTransactionOld bool
}

var orgIDTest = seeds.RandomOrgId()
var accountIdTest = seeds.RandomAccountId()
var timestamp = time.Now()

var repoPublicTest = models.Repository{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	URL:                          "https://www.public.example.com",
	Public:                       true,
	LastIntrospectionTime:        &timestamp,
	LastIntrospectionUpdateTime:  &timestamp,
	LastIntrospectionSuccessTime: &timestamp,
	LastIntrospectionError:       nil,
	LastIntrospectionStatus:      config.StatusValid,
	PackageCount:                 525600,
	FailedIntrospectionsCount:    5,
}

var repoPrivateTest = models.Repository{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	URL:                          "https://www.private.example.com",
	Public:                       false,
	LastIntrospectionTime:        &timestamp,
	LastIntrospectionUpdateTime:  &timestamp,
	LastIntrospectionSuccessTime: &timestamp,
	LastIntrospectionError:       nil,
	LastIntrospectionStatus:      config.StatusValid,
	PackageCount:                 108,
	FailedIntrospectionsCount:    5,
}

var repoConfigTest1 = models.RepositoryConfiguration{
	Base: models.Base{
		UUID:      uuid.NewString(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	Name:                 "Demo Repository Config",
	Arch:                 "x86_64",
	Versions:             pq.StringArray{config.El7, config.El8},
	AccountID:            accountIdTest,
	OrgID:                orgIDTest,
	RepositoryUUID:       repoPublicTest.UUID,
	GpgKey:               "foo",
	MetadataVerification: true,
	LastSnapshotUUID:     "last-snap-id",
}

var repoRpmTest1 = models.Rpm{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	Name:     "test-package",
	Arch:     "x86_64",
	Version:  "1.0.0",
	Release:  "123",
	Epoch:    1,
	Summary:  "Test package summary",
	Checksum: "SHA1:442884394e5faccbb5a9ae945b293fc6dcec1c92",
}

var repoRpmTest2 = models.Rpm{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	Name:     "demo-package",
	Arch:     "noarch",
	Version:  "2.0.0",
	Release:  "321",
	Epoch:    2,
	Summary:  "Demo package summary",
	Checksum: "SHA1:6799a487f8eaf5c6ad6aba43e1dc4503e69e75bd",
}

var repoPackageGroupTest1 = models.PackageGroup{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	ID:          "test-package-group-id",
	Name:        "test-package-group",
	Description: "description",
	PackageList: []string{"test-package"},
}

var repoPackageGroupTest2 = models.PackageGroup{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	ID:          "demo-package-group-id",
	Name:        "demo-package-group",
	Description: "description",
	PackageList: []string{"demo-package"},
}

var repoEnvironmentTest1 = models.Environment{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	ID:          "test-environment-id",
	Name:        "test-environment",
	Description: "description",
}

var repoEnvironmentTest2 = models.Environment{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	ID:          "demo-environment-id",
	Name:        "demo-environment",
	Description: "description",
}

func (s *DaoSuite) TearDownTest() {
	// Rollback and reset db.DB
	s.tx.Rollback()
	s.db.SkipDefaultTransaction = s.skipDefaultTransactionOld
}

func (s *DaoSuite) SetupTest() {
	if db.DB == nil {
		if err := db.Connect(); err != nil {
			s.FailNow(err.Error())
		}
	}
	s.skipDefaultTransactionOld = db.DB.SkipDefaultTransaction
	s.db = db.DB.Session(&gorm.Session{
		SkipDefaultTransaction: false,
		Logger: logger.New(
			log.New(os.Stderr, "\r\n", log.LstdFlags),
			logger.Config{
				LogLevel: logger.Info,
			}),
	})

	s.tx = s.db.Begin()
	// s.tx = s.db
	s.SeedPreexistingRHRepo()
	s.SeedPreexistingCommunityRepo()
}

// SeedPreexistingRHRepo seeds a red hat repo with a snapshot task to verify that tests with Red Hat repo filters
// consider preexisting red hat repos not created by the test. Custom repos are not a concern because
// the org ID is unique
func (s *DaoSuite) SeedPreexistingRHRepo() {
	repoConfigs, err := seeds.SeedRepositoryConfigurations(s.tx, 1, seeds.SeedOptions{OrgID: config.RedHatOrg, Origin: utils.Ptr(config.OriginRedHat)})
	require.NoError(s.T(), err)
	_, err = seeds.SeedSnapshots(s.tx, repoConfigs[0].UUID, 1)
	require.NoError(s.T(), err)
	_, err = seeds.SeedTasks(s.tx, 1,
		seeds.TaskSeedOptions{OrgID: config.RedHatOrg,
			RepoConfigUUID: repoConfigs[0].UUID,
			Typename:       config.RepositorySnapshotTask,
			Status:         config.TaskStatusCompleted})
	require.NoError(s.T(), err)
}

func (s *DaoSuite) SeedPreexistingCommunityRepo() {
	repoConfigs, err := seeds.SeedRepositoryConfigurations(s.tx, 1, seeds.SeedOptions{OrgID: config.CommunityOrg, Origin: utils.Ptr(config.OriginCommunity)})
	require.NoError(s.T(), err)
	_, err = seeds.SeedSnapshots(s.tx, repoConfigs[0].UUID, 1)
	require.NoError(s.T(), err)
	_, err = seeds.SeedTasks(s.tx, 1,
		seeds.TaskSeedOptions{OrgID: config.CommunityOrg,
			RepoConfigUUID: repoConfigs[0].UUID,
			Typename:       config.RepositorySnapshotTask,
			Status:         config.TaskStatusCompleted})
	require.NoError(s.T(), err)
}

func (s *DaoSuite) createTestRedHatRepository(repo api.RepositoryRequest) api.RepositoryResponse {
	t := s.T()
	tx := s.tx

	var modelRepoConfig models.RepositoryConfiguration
	var modelRepository models.Repository
	ApiFieldsToModel(repo, &modelRepoConfig, &modelRepository)

	modelRepository.Origin = config.OriginRedHat
	modelRepoConfig.OrgID = config.RedHatOrg
	modelRepoConfig.Label = seeds.RandStringBytes(10)

	err := tx.Create(&modelRepository).Error
	assert.NoError(t, err)
	modelRepoConfig.RepositoryUUID = modelRepository.UUID

	err = tx.Create(&modelRepoConfig).Error
	assert.NoError(t, err)

	tx.Where("uuid = ?", modelRepoConfig.UUID).Preload("Repository").First(&modelRepoConfig)

	var repoResp api.RepositoryResponse
	ModelToApiFields(modelRepoConfig, &repoResp)

	return repoResp
}
