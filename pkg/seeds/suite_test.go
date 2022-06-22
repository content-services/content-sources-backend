package seeds

import (
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type SeedSuite struct {
	suite.Suite
	db *gorm.DB
	tx *gorm.DB
}

func getDSNWithOptions(user string, password string, dbname string, host string, port int) string {
	return fmt.Sprintf(
		"user=%s password=%s dbname=%s host=%s port=%d sslmode=disable",
		user,
		password,
		dbname,
		host,
		port,
	)
}

func getDSNWithConfig(c *config.Configuration) string {
	if c == nil {
		return ""
	}
	return getDSNWithOptions(
		c.Database.User,
		c.Database.Password,
		c.Database.Name,
		c.Database.Host,
		c.Database.Port,
	)
}

func getDSNDefault() string {
	config := config.Get()
	return getDSNWithConfig(config)
}

func getDbConnection() *gorm.DB {
	dsn := getDSNDefault()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil
	}
	return db
}

func generatePostgresDsn(c *config.Configuration) (string, error) {
	if c == nil {
		return "", fmt.Errorf("'v' argument can not be nil")
	}

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		c.Database.Host,
		c.Database.Port,
		c.Database.User,
		c.Database.Password,
		c.Database.Name,
	)
	return dsn, nil
}

// SetupSuite Initialize the test suite
func (s *SeedSuite) SetupSuite() {
	s.db = getDbConnection()
}

// TearDownSuite Finalize the test suite
func (s *SeedSuite) TearDownSuite() {
	s.db = nil
}

// SetupTest Prepare the unit test
func (s *SeedSuite) SetupTest() {
	// Remove the content for the 3 involved tables
	s.tx = s.db.Begin()

	s.tx.Where("1=1").Delete(models.RepositoryRpm{})
	s.tx.Where("1=1").Delete(models.Repository{})
	s.tx.Where("1=1").Delete(models.RepositoryConfiguration{})
}

// TearDownTest Clean up the unit test
func (s *SeedSuite) TearDownTest() {
	s.tx.Rollback()
}

// TestSeedSuite Launch the test suite
func TestSeedSuite(t *testing.T) {
	suite.Run(t, new(SeedSuite))
}
