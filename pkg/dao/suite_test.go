package dao

import (
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/lib/pq"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type RepositorySuite struct {
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
}

const orgIdTest = "acme"
const accountIdTest = "817342"

var repoTest1 = models.Repository{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	URL:           "https://www.redhat.com",
	LastReadTime:  nil,
	LastReadError: nil,
}

var repoConfigTest1 = models.RepositoryConfiguration{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	Name:           "Demo Repository Config",
	Arch:           "x86_64",
	Versions:       pq.StringArray{"6", "7", "8", "9"},
	AccountID:      accountIdTest,
	OrgID:          orgIdTest,
	RepositoryUUID: repoTest1.Base.UUID,
}

var repoRpmTest1 = models.Rpm{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	Name:        "test-package",
	Arch:        "x86_64",
	Version:     "1.0.0",
	Release:     "123",
	Epoch:       1,
	Summary:     "Test package summary",
	Description: "Test package summary",
	Checksum:    "SHA1:442884394e5faccbb5a9ae945b293fc6dcec1c92",
}

var repoRpmTest2 = models.Rpm{
	Base: models.Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	Name:        "demo-package",
	Arch:        "noarch",
	Version:     "2.0.0",
	Release:     "321",
	Epoch:       2,
	Summary:     "Demo package summary",
	Description: "Demo package summary",
	Checksum:    "SHA1:6799a487f8eaf5c6ad6aba43e1dc4503e69e75bd",
}

//
// SetUp and TearDown for RepositorySuite
//

func (s *RepositorySuite) SetupTest() {
	if db.DB == nil {
		db.Connect()
	}
	s.db = db.DB
	s.skipDefaultTransactionOld = s.db.SkipDefaultTransaction
	s.db.SkipDefaultTransaction = false
	s.tx = s.db.Begin()

	// Remove the content for the 3 involved tables
	s.tx.Where("1=1").Delete(models.Rpm{})
	s.tx.Where("1=1").Delete(models.Repository{})
	s.tx.Where("1=1").Delete(models.RepositoryConfiguration{})
}

func (s *RepositorySuite) TearDownTest() {
	s.tx.Rollback()
	s.db.SkipDefaultTransaction = s.skipDefaultTransactionOld
}

//
// SetUp and TearDown for RepositoryRpmSuite
//

func (s *RpmSuite) SetupTest() {
	if db.DB == nil {
		db.Connect()
	}
	s.db = db.DB.Session(&gorm.Session{
		SkipDefaultTransaction: false,
	})
	s.tx = s.db.Begin()

	// Remove the content for the 3 involved tables
	s.tx.Where("1=1").Delete(models.Rpm{})
	s.tx.Where("1=1").Delete(models.Repository{})
	s.tx.Where("1=1").Delete(models.RepositoryConfiguration{})

	repo := repoTest1.DeepCopy()
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
}

func (s *RpmSuite) TearDownTest() {
	//Rollback and reset db.DB
	s.tx.Rollback()
	s.db.SkipDefaultTransaction = s.skipDefaultTransactionOld
}

//
// TestDaoSuite Launch all the test suites for dao package
//
func TestRepositorySuite(t *testing.T) {
	suite.Run(t, new(RepositorySuite))
}

func TestRpmSuite(t *testing.T) {
	suite.Run(t, new(RpmSuite))
}
