package dao

import (
	"log"
	"os"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/google/uuid"
	"github.com/lib/pq"
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
	Status:                       config.StatusValid,
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
	Status:                       config.StatusValid,
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
	RepositoryUUID:       repoPublicTest.Base.UUID,
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
	//Rollback and reset db.DB
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
}
