package db

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/slices"
	pg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

// GetUrl Get database config and return url
func GetUrl() string {
	dbConfig := config.Get().Database
	connectStr := fmt.Sprintf(
		"user=%s password=%s dbname=%s host=%s port=%d",
		dbConfig.User,
		dbConfig.Password,
		dbConfig.Name,
		dbConfig.Host,
		dbConfig.Port,
	)

	var sslStr string
	if dbConfig.CACertPath == "" {
		sslStr = " sslmode=disable"
	} else {
		sslStr = fmt.Sprintf(" sslmode=verify-full sslrootcert=%s", dbConfig.CACertPath)
	}
	return connectStr + sslStr
}

// Connect initializes global database connection, DB
func Connect() error {
	var err error

	dbURL := GetUrl()
	DB, err = gorm.Open(pg.Open(dbURL), &gorm.Config{
		Logger: NewDBLogger(
			DBLogConfig{
				SlowThreshold:              config.Get().Database.SlowQueryDuration,
				LogLevel:                   zeroLogToGormLevel(config.DBLevel()),
				IgnoreRecordNotFoundError:  true,
				IgnoreContextCanceledError: true,
				Colorful:                   config.Get().Logging.Color,
				zeroLogger:                 log.Logger,
			},
		),
	})

	if err != nil {
		return err
	}
	DB.CreateBatchSize = config.DefaultPagedRpmInsertsLimit

	sqlDb, err := DB.DB()
	if err != nil {
		return err
	}
	sqlDb.SetMaxOpenConns(config.Get().Database.PoolLimit)
	return nil
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

	err = checkLatestMigrationFile()
	if err != nil {
		return err
	}

	var step int
	if steps != nil {
		step = steps[0]
	}

	switch direction {
	case "up":
		if step > 0 {
			err = m.Steps(step)
		} else {
			err = m.Up()
		}
	case "down":
		if step > 0 {
			step *= -1
			err = m.Steps(step)
		} else {
			err = m.Down()
		}
	}

	if err != nil && err == migrate.ErrNoChange {
		log.Debug().Msg("No new migrations.")
		return nil
	} else if err != nil {
		log.Error().Err(err).Msg("Failed to migrate:")
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
	migrationFileNames, err := getMigrationFiles()
	if err != nil {
		return 0, err
	}
	version, _, _ := m.Version()
	var previousMigrationIndex int
	var datetimes []int

	for _, name := range migrationFileNames {
		nameArr := strings.Split(name, "_")
		datetime, _ := strconv.Atoi(nameArr[0])
		datetimes = append(datetimes, datetime)
	}
	if version > math.MaxInt {
		return 0, fmt.Errorf("invalid version: %d", version)
	}
	previousMigrationIndex = (sort.IntSlice(datetimes).Search(int(version))) - 1
	if previousMigrationIndex == -1 {
		return 0, err
	} else {
		return datetimes[previousMigrationIndex], err
	}
}

const LatestMigrationFile = "./db/migrations.latest"

func checkLatestMigrationFile() error {
	migrationFileNames, err := getMigrationFiles()
	if err != nil {
		return err
	}
	last := migrationFileNames[len(migrationFileNames)-1]
	nameArr := strings.Split(last, "_")
	expectedLatest, err := os.ReadFile(LatestMigrationFile)
	if err != nil {
		return err
	}
	datetime := nameArr[0]
	trimmed := strings.TrimSpace(string(expectedLatest))
	if datetime != trimmed {
		return fmt.Errorf("latest migration from %v (%v) does not match found latest file (%v)", LatestMigrationFile, trimmed, datetime)
	}
	return nil
}

func getMigrationFiles() ([]string, error) {
	var f *os.File
	f, err := os.Open("./db/migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer f.Close()

	migrationFileNames, err := f.Readdirnames(0)
	if err != nil {
		return nil, fmt.Errorf("failed to read filenames: %v", err)
	}
	slices.Sort(migrationFileNames)

	return migrationFileNames, nil
}
