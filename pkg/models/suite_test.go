package models

import (
	"math/rand"
	"strconv"
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

type RepositoryRpmSuite struct {
	*ModelsSuite
}

type RepositoryPackageGroupSuite struct {
	*ModelsSuite
}

type RepositoryEnvironmentSuite struct {
	*ModelsSuite
}

// Not using seeds.RandomOrgId to avoid cycle dependency
var orgIDTest = strconv.Itoa(rand.Intn(99999999))
var accountIdTest = strconv.Itoa(rand.Intn(99999999))

var repoConfigTest1 = RepositoryConfiguration{
	Base: Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	Name:      "Demo Repository Config",
	Arch:      config.X8664,
	Versions:  pq.StringArray{config.El7, config.El8, config.El9},
	AccountID: accountIdTest,
	OrgID:     orgIDTest,
}

var repoTest1 = Repository{
	Base: Base{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	URL:                    "https://www.redhat.com",
	LastIntrospectionTime:  nil,
	LastIntrospectionError: nil,
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

var packageGroupTest1 = PackageGroup{
	ID:          "test-package-group",
	Name:        "test-package-group",
	Description: "",
	PackageList: []string(nil),
}

var environmentTest1 = Environment{
	ID:          "test-environment",
	Name:        "test-environment",
	Description: "",
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

func TestModelsSuite(t *testing.T) {
	suite.Run(t, new(ModelsSuite))
}
