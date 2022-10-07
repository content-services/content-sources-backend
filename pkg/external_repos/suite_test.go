package external_repos

import (
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type ExternalRepoSuite struct {
	suite.Suite
	db                        *gorm.DB
	tx                        *gorm.DB
	skipDefaultTransactionOld bool
}

func (s *ExternalRepoSuite) SetupTest() {
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

func (s *ExternalRepoSuite) TearDownTest() {
	s.tx.Rollback()
	s.db.SkipDefaultTransaction = s.skipDefaultTransactionOld
}
