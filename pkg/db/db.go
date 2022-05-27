package db

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	pg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

// GetUrl Get database config and return url
func GetUrl() string {
	dbConfig := config.Get().Database
	return fmt.Sprintf(
		"user=%s password=%s dbname=%s host=%s port=%d sslmode=disable",
		dbConfig.User,
		dbConfig.Password,
		dbConfig.Name,
		dbConfig.Host,
		dbConfig.Port,
	)
}

// Connect initializes global database connection, DB
func Connect() error {
	var err error
	dbURL := GetUrl()
	DB, err = gorm.Open(pg.Open(dbURL), &gorm.Config{})
	return err
}

// Close closes global database connection, DB
func Close() error {
	var sqlDB *sql.DB
	var err error

	sqlDB, err = DB.DB()
	if err != nil {
		return err
	}

	if err = sqlDB.Close(); err != nil {
		return err
	}
	return err
}

// setupMigration connect to the DB and driver, returns pointer to migration instance.
func setupMigration(dbURL string) (*migrate.Migrate, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("could not connect to database: %w", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, fmt.Errorf("could not get database driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://./db/migrations",
		"postgres", driver)
	if err != nil {
		return nil, fmt.Errorf("could not create migration instance: %w", err)
	}

	return m, err
}

// MigrateDB runs migrations up or down with amount to run. Omit "steps" to run all migrations.
func MigrateDB(dbURL string, direction string, steps ...int) error {
	m, err := setupMigration(dbURL)
	if err != nil {
		return fmt.Errorf("migration setup failed: %w", err)
	}

	var step int
	if steps != nil {
		step = steps[0]
	}

	if direction == "up" {
		if step > 0 {
			err = m.Steps(step)
		} else {
			err = m.Up()
		}
	} else if direction == "down" {
		if step > 0 {
			step *= -1
			err = m.Steps(step)
		} else {
			err = m.Down()
		}
	}

	if err != nil {
		// Force back to previous migration version. If errors running version 1,
		// drop everything (which would just be the schema_migrations table).
		// This is safe if migrations are wrapped in transaction.
		previousMigrationVersion, err := getPreviousMigrationVersion(m)
		if err != nil {
			return err
		}
		if previousMigrationVersion == 0 {
			if err = m.Drop(); err != nil {
				return err
			}
		} else {
			if err = m.Force(previousMigrationVersion); err != nil {
				return err
			}
		}
	}
	return err
}

func getPreviousMigrationVersion(m *migrate.Migrate) (int, error) {
	var f *os.File
	f, err := os.Open("./db/migrations")
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %v", err)
	}
	defer f.Close()

	migrationFileNames, _ := f.Readdirnames(0)
	version, _, _ := m.Version()
	var previousMigrationIndex int
	var datetimes []int

	for _, name := range migrationFileNames {
		nameArr := strings.Split(name, "_")
		datetime, _ := strconv.Atoi(nameArr[0])
		datetimes = append(datetimes, datetime)
	}
	previousMigrationIndex = sort.IntSlice(datetimes).Search(int(version)) - 1
	if previousMigrationIndex == -1 {
		return 0, err
	} else {
		return datetimes[previousMigrationIndex], err
	}
}
