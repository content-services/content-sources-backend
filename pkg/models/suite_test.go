package models

import (
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
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
	Arch:      config.X8664,
	Versions:  pq.StringArray{config.El7, config.El8, config.El9},
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
	Name:     "test-package",
	Arch:     "x86_64",
	Version:  "1.0.0",
	Release:  "123",
	Epoch:    0,
	Summary:  "Test package summary",
	Checksum: "SHA256:b8229cf1a40dc02282aff718811b97f2330bcc62ad7657a885d18fb4cc1cdf29",
}

func (suite *ModelsSuite) SetupTest() {
	if err := db.Connect(); err != nil {
		return
	}
	suite.db = db.DB
	suite.tx = suite.db.Begin()
}

func (s *ModelsSuite) TearDownTest() {
	s.tx.Rollback()
}

func TestRunSuiteModels(t *testing.T) {
	suite.Run(t, new(ModelsSuite))
}
