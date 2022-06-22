package dao

import (
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/lib/pq"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type RepositorySuite struct {
	suite.Suite
	db                        *gorm.DB
	tx                        *gorm.DB
	skipDefaultTransactionOld bool
}

type RepositoryRpmSuite struct {
	suite.Suite
	db                        *gorm.DB
	tx                        *gorm.DB
	skipDefaultTransactionOld bool
	repoConfig                *models.RepositoryConfiguration
	repo                      *models.Repository
}

const orgIdTest = "acme"
const accountIdTest = "817342"

var repoConfigTest1 = models.RepositoryConfiguration{
	Base: models.Base{
		UUID:      "67eb30d9-9264-4726-9d90-8959e0945a55",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	Name:      "Demo Repository Config",
	URL:       "https://www.redhat.com",
	Arch:      "x86_64",
	Versions:  pq.StringArray{"6", "7", "8", "9"},
	AccountID: accountIdTest,
	OrgID:     orgIdTest,
}

var repoTest1 = models.Repository{
	Base: models.Base{
		UUID:      "55bc5f6b-b5e6-45cb-9953-425b6d4102a0",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	URL:             "https://www.redhat.com",
	LastReadTime:    nil,
	LastReadError:   nil,
	ReferRepoConfig: pointy.String(repoConfigTest1.Base.UUID),
}

var repoRpmTest1 = models.RepositoryRpm{
	Name:        "test-package",
	Arch:        "x86_64",
	Version:     "1.0.0",
	Release:     "123",
	Epoch:       pointy.Int32(1),
	Summary:     "Test package summary",
	Description: "Test package summary",
}

var repoRpmTest2 = models.RepositoryRpm{
	Name:        "demo-package",
	Arch:        "noarch",
	Version:     "2.0.0",
	Release:     "321",
	Epoch:       pointy.Int32(2),
	Summary:     "Demo package summary",
	Description: "Demo package summary",
}

//
// SetUp and TearDown for RepositorySuite
//

func (s *RepositorySuite) SetupTest() {
	// suite.savedDB = db.DB
	if db.DB == nil {
		db.Connect()
	}
	s.db = db.DB
	s.skipDefaultTransactionOld = s.db.SkipDefaultTransaction
	s.db.SkipDefaultTransaction = false
	s.tx = s.db.Begin()

	// Remove the content for the 3 involved tables
	s.tx.Where("1=1").Delete(models.RepositoryRpm{})
	s.tx.Where("1=1").Delete(models.Repository{})
	s.tx.Where("1=1").Delete(models.RepositoryConfiguration{})
}

func (s *RepositorySuite) TearDownTest() {
	//Rollback and reset db.DB
	s.tx.Rollback()
	s.db.SkipDefaultTransaction = s.skipDefaultTransactionOld
}

//
// SetUp and TearDown for RepositoryRpmSuite
//

func (s *RepositoryRpmSuite) SetupTest() {
	// suite.savedDB = db.DB
	if db.DB == nil {
		db.Connect()
	}
	s.db = db.DB
	s.skipDefaultTransactionOld = s.db.SkipDefaultTransaction
	s.db.SkipDefaultTransaction = false
	s.tx = s.db.Begin()

	// Remove the content for the 3 involved tables
	s.tx.Where("1=1").Delete(models.RepositoryRpm{})
	s.tx.Where("1=1").Delete(models.Repository{})
	s.tx.Where("1=1").Delete(models.RepositoryConfiguration{})

	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	s.tx.Create(repoConfig)
	repo.ReferRepoConfig = pointy.String(repoConfig.Base.UUID)
	s.tx.Create(repo)

	s.repoConfig = repoConfig
	s.repo = repo
}

func (s *RepositoryRpmSuite) TearDownTest() {
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

func TestRepositoryRpmSuite(t *testing.T) {
	suite.Run(t, new(RepositoryRpmSuite))
}
