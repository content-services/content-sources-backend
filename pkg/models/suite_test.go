package models

import (
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/lib/pq"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type ModelsSuite struct {
	suite.Suite
	db *gorm.DB
	tx *gorm.DB
}

const orgIdTest = "acme"
const accountIdTest = "817342"

var repoConfigTest1 = RepositoryConfiguration{
	Base: Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	Name:      "Demo Repository Config",
	Arch:      "x86_64",
	Versions:  pq.StringArray{"6", "7", "8", "9"},
	AccountID: accountIdTest,
	OrgID:     orgIdTest,
}

var repoTest1 = Repository{
	Base: Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	URL:           "https://www.redhat.com",
	LastReadTime:  nil,
	LastReadError: nil,
}

var rpmTest1 = Rpm{
	Name:        "test-package",
	Arch:        "x86_64",
	Version:     "1.0.0",
	Release:     "123",
	Epoch:       1,
	Summary:     "Test package summary",
	Description: "Test package summary",
}

var rpmTest2 = Rpm{
	Name:        "demo-package",
	Arch:        "noarch",
	Version:     "2.0.0",
	Release:     "321",
	Epoch:       2,
	Summary:     "Demo package summary",
	Description: "Demo package summary",
}

func (suite *ModelsSuite) SetupTest() {
	if err := db.Connect(); err != nil {
		return
	}
	suite.db = db.DB
	suite.tx = suite.db.Begin()

	// Remove the content for the 3 involved tables
	suite.tx.Where("1=1").Delete(Rpm{})
	suite.tx.Where("1=1").Delete(RepositoryConfiguration{})
	suite.tx.Where("1=1").Delete(Repository{})
}

func (s *ModelsSuite) TearDownTest() {
	s.tx.Rollback()
}

func TestRunSuiteModels(t *testing.T) {
	suite.Run(t, new(ModelsSuite))
}
