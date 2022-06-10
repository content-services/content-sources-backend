package seeds

import (
	"fmt"
	"os"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

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

func TestSeed(t *testing.T) {
	var dsn string
	var err error
	var db *gorm.DB

	os.Setenv("CONFIG_PATH", "../../configs")

	config.Load()
	cfg := config.Get()

	dsn, err = generatePostgresDsn(cfg)
	assert.Nil(t, err, "Error generating dsn string")

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	assert.Nil(t, err, "Error connecting to database")

	err = SeedRepositoryConfigurations(db, 10)
	assert.Nil(t, err, "Error seeding RepositoryConfigurations")

	err = SeedRepositoryRpms(db, 10)
	assert.Nil(t, err, "Error seeding RepositoryRpm")
}
