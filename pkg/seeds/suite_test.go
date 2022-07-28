package seeds

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type SeedSuite struct {
	suite.Suite
	db *gorm.DB
	tx *gorm.DB
}

// SetupSuite Initialize the test suite
func (s *SeedSuite) SetupSuite() {
	if err := db.Connect(); err != nil {
		return
	}
	s.db = db.DB
}

// TearDownSuite Finalize the test suite
func (s *SeedSuite) TearDownSuite() {
	s.db = nil
}

// SetupTest Prepare the unit test
func (s *SeedSuite) SetupTest() {
	// Remove the content for the 3 involved tables
	s.tx = s.db.Begin()
}

// TearDownTest Clean up the unit test
func (s *SeedSuite) TearDownTest() {
	s.tx.Rollback()
}

// TestSeedSuite Launch the test suite
func TestSeedSuite(t *testing.T) {
	suite.Run(t, new(SeedSuite))
}
