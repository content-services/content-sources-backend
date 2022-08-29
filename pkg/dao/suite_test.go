package dao

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/lib/pq"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type RepositoryConfigSuite struct {
	suite.Suite
	db                        *gorm.DB
	tx                        *gorm.DB
	skipDefaultTransactionOld bool
}

type RpmSuite struct {
	suite.Suite
	db                        *gorm.DB
	tx                        *gorm.DB
	skipDefaultTransactionOld bool
	repoConfig                *models.RepositoryConfiguration
	repo                      *models.Repository
	repoPrivate               *models.Repository
}

type RepositorySuite struct {
	suite.Suite
	db                        *gorm.DB
	tx                        *gorm.DB
	skipDefaultTransactionOld bool
	repoConfig                *models.RepositoryConfiguration
	repo                      *models.Repository
	repoPrivate               *models.Repository
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
	Status:                       config.StatusValid,
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
	Status:                       config.StatusValid,
}

var repoConfigTest1 = models.RepositoryConfiguration{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	Name:           "Demo Repository Config",
	Arch:           "x86_64",
	Versions:       pq.StringArray{config.El7, config.El8},
	AccountID:      accountIdTest,
	OrgID:          orgIDTest,
	RepositoryUUID: repoPublicTest.Base.UUID,
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

//
// SetUp and TearDown for RepositoryConfigSuite
//

func (s *RepositoryConfigSuite) SetupTest() {
	if db.DB == nil {
		if err := db.Connect(); err != nil {
			s.FailNow(err.Error())
		}
	}
	s.db = db.DB
	s.skipDefaultTransactionOld = s.db.SkipDefaultTransaction
	s.db.SkipDefaultTransaction = false
	s.tx = s.db.Begin()
}

func (s *RepositoryConfigSuite) TearDownTest() {
	s.tx.Rollback()
	s.db.SkipDefaultTransaction = s.skipDefaultTransactionOld
}

//
// SetUp and TearDown for RpmSuite
//

func (s *RpmSuite) SetupTest() {
	if db.DB == nil {
		if err := db.Connect(); err != nil {
			s.FailNow(err.Error())
		}
	}
	s.db = db.DB.Session(&gorm.Session{
		SkipDefaultTransaction: false,
	})
	s.tx = s.db.Begin()

	repo := repoPublicTest.DeepCopy()
	if err := s.tx.Create(repo).Error; err != nil {
		s.FailNow("Preparing Repository record: %w", err)
	}
	s.repo = repo

	repoPrivate := repoPrivateTest.DeepCopy()
	if err := s.tx.Create(repoPrivate).Error; err != nil {
		s.FailNow("Preparing private Repository record: %w", err)
	}
	s.repoPrivate = repoPrivate

	repoConfig := repoConfigTest1.DeepCopy()
	repoConfig.RepositoryUUID = repo.Base.UUID
	if err := s.tx.Create(repoConfig).Error; err != nil {
		s.FailNow("Preparing RepositoryConfiguration record: %w", err)
	}
	s.repoConfig = repoConfig
}

func (s *RpmSuite) TearDownTest() {
	//Rollback and reset db.DB
	s.tx.Rollback()
	s.db.SkipDefaultTransaction = s.skipDefaultTransactionOld
}

//
// SetUp and TearDown for RepositoryRpmSuite
//
func (s *RepositorySuite) SetupTest() {
	if db.DB == nil {
		if err := db.Connect(); err != nil {
			s.FailNow(err.Error())
		}
	}
	s.db = db.DB.Session(&gorm.Session{
		SkipDefaultTransaction: false,
		Logger: logger.New(
			log.New(os.Stderr, "\r\n", log.LstdFlags),
			logger.Config{
				LogLevel: logger.Info,
			}),
	})
	s.tx = s.db.Begin()

	repo := repoPublicTest.DeepCopy()
	if err := s.tx.Create(repo).Error; err != nil {
		s.FailNow("Preparing Repository record UUID=" + repo.UUID)
	}
	s.repo = repo

	repoConfig := repoConfigTest1.DeepCopy()
	repoConfig.RepositoryUUID = repo.Base.UUID
	if err := s.tx.Create(repoConfig).Error; err != nil {
		s.FailNow("Preparing RepositoryConfiguration record UUID=" + repoConfig.UUID)
	}
	s.repoConfig = repoConfig

	repoPrivate := repoPrivateTest.DeepCopy()
	if err := s.tx.Create(&repoPrivate).Error; err != nil {
		s.FailNow(err.Error())
	}
	s.repoPrivate = repoPrivate
}

func (s *RepositorySuite) TearDownTest() {
	//Rollback and reset db.DB
	s.tx.Rollback()
	s.db.SkipDefaultTransaction = s.skipDefaultTransactionOld
}

//
// TestDaoSuite Launch all the test suites for dao package
//
func TestRepositoryConfigSuite(t *testing.T) {
	suite.Run(t, new(RepositoryConfigSuite))
}

func TestRpmSuite(t *testing.T) {
	suite.Run(t, new(RpmSuite))
}

func TestRepositorySuite(t *testing.T) {
	suite.Run(t, new(RepositorySuite))
}
