package seeds

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/openlyinc/pointy"
	"gorm.io/gorm"
)

type SeedOptions struct {
	OrgID    string
	Arch     *string
	Versions *[]string
}

const (
	batchSize = 500
)

func randomURL() string {
	return fmt.Sprintf("https://%s.com/%s", RandStringBytes(20), RandStringBytes(5))
}

func randomRepositoryRpmName() string {
	return RandStringBytes(12)
}

var (
	archs []string = []string{
		"x86_64",
		"noarch",
	}
)

func randomRepositoryRpmArch() string {
	return archs[rand.Int()%len(archs)]
}

func SeedRepositoryConfigurations(db *gorm.DB, size int, options SeedOptions) error {
	var repos []models.Repository
	var repoConfigurations []models.RepositoryConfiguration

	for i := 0; i < size; i++ {
		repo := models.Repository{
			URL:           randomURL(),
			LastReadTime:  nil,
			LastReadError: nil,
		}
		repos = append(repos, repo)
	}
	if err := db.Create(&repos).Error; err != nil {
		return err
	}

	arch := "x86_64"
	if options.Arch != nil && *options.Arch != "" {
		arch = *options.Arch
	}
	versions := []string{"9"}
	if options.Versions != nil {
		versions = *options.Versions
	}
	for i := 0; i < size; i++ {
		repoConfig := models.RepositoryConfiguration{
			Name:           fmt.Sprintf("%s - %s - %s", RandStringBytes(2), "TestRepo", RandStringBytes(10)),
			Versions:       versions,
			Arch:           arch,
			AccountID:      fmt.Sprintf("%d", rand.Intn(9999)),
			OrgID:          options.OrgID,
			RepositoryUUID: repos[i].UUID,
		}
		repoConfigurations = append(repoConfigurations, repoConfig)
	}
	if err := db.Create(&repoConfigurations).Error; err != nil {
		return errors.New("could not save seed")
	}
	return nil
}

func randomLastRead() (lastReadTime *time.Time, lastReadError *string) {
	if rand.Int()%2 == 0 {
		return nil, nil
	}
	var readTime *time.Time = &time.Time{}
	*readTime = time.Now()
	var readError *string = pointy.String("Random error")
	return readTime, readError
}

func SeedRepository(db *gorm.DB, size int) error {
	var repos []models.Repository

	// Add size randome Repository entries
	countRecords := 0
	for i := 0; i < size; i++ {
		lastReadTime, lastReadError := randomLastRead()
		repo := models.Repository{
			URL:           randomURL(),
			LastReadTime:  lastReadTime,
			LastReadError: lastReadError,
		}
		repos = append(repos, repo)
		if len(repos) >= batchSize {
			if r := db.Create(repos); r != nil && r.Error != nil {
				return r.Error
			}
			countRecords += len(repos)
			repos = []models.Repository{}
			fmt.Printf("repoConfig: %d        \r", countRecords)
		}
	}

	// Add remaining records
	if len(repos) > 0 {
		if r := db.Create(repos); r != nil && r.Error != nil {
			return r.Error
		}
		countRecords += len(repos)
		fmt.Printf("repoConfig: %d        \r", countRecords)
	}

	return nil
}

// SeedRpms Populate database with random package information
// db The database descriptor.
// size The number of rpm packages per repository to be generated.
func SeedRpms(db *gorm.DB, repo *models.Repository, size int) error {
	if db == nil {
		return fmt.Errorf("db cannot be nil")
	}
	if repo == nil {
		return fmt.Errorf("repo cannot be nil")
	}
	if size < 0 {
		return fmt.Errorf("size cannot be lower than 0")
	}
	if size == 0 {
		return nil
	}
	var rpms []models.Rpm

	var repositories_rpms []map[string]interface{}

	// For each repo add 'size' rpm random packages
	for i := 0; i < size; i++ {
		rpm := models.Rpm{
			Name:     randomRepositoryRpmName(),
			Arch:     randomRepositoryRpmArch(),
			Version:  fmt.Sprintf("%d.%d.%d", rand.Int()%6, rand.Int()%16, rand.Int()%64),
			Release:  fmt.Sprintf("%d", rand.Int()%128),
			Epoch:    0,
			Checksum: RandStringBytes(64),
		}
		rpms = append(rpms, rpm)
	}

	if err := db.Create(rpms).Error; err != nil {
		return err
	}

	for _, rpm := range rpms {
		repositories_rpms = append(repositories_rpms, map[string]interface{}{
			"repository_uuid": repo.Base.UUID,
			"rpm_uuid":        rpm.Base.UUID,
		})
	}
	if err := db.Table(models.TableNameRpmsRepositories).Create(&repositories_rpms).Error; err != nil {
		return err
	}

	return nil
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
