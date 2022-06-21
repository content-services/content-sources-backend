package seeds

import (
	"fmt"
	"math/rand"
	"strconv"

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

func SeedRepositoryConfigurations(db *gorm.DB, size int, options SeedOptions) error {
	var repos []models.RepositoryConfiguration

	for i := 0; i < size; i++ {
		repoConfig := models.RepositoryConfiguration{
			Name:      fmt.Sprintf("%s - %s - %s", RandStringBytes(2), "TestRepo", RandStringBytes(10)),
			URL:       fmt.Sprintf("https://%s.com/%s", RandStringBytes(20), RandStringBytes(5)),
			Versions:  createVersionArray(options.Versions),
			Arch:      createArch(options.Arch),
			AccountID: strconv.Itoa(rand.Intn(9999)),
			OrgID:     createOrgId(options.OrgID),
		}

		repos = append(repos, repoConfig)
	}
	if result := db.Create(&repos); result.Error != nil {
		return result.Error
		// return errors.New("could not save seed")
	}
	return nil
}

func createOrgId(existingOrgId string) string {
	orgId := "4234"
	if existingOrgId != "" {
		orgId = existingOrgId
	} else {
		randomNum := rand.Intn(5)
		if randomNum == 3 {
			orgId = strconv.Itoa(rand.Intn(9999))
		}
	}
	return orgId
}

func createVersionArray(existingVersionArray *[]string) []string {
	versionArray := []string{"7"}

	if existingVersionArray != nil {
		versionArray = *existingVersionArray
		return versionArray
	}

	length := rand.Intn(4)

	for k := 0; k < length; k++ {
		versionArray = append(versionArray, fmt.Sprintf("%d", 8+k))
	}
	return versionArray
}

func createArch(existingArch *string) string {
	arch := "x86_64"
	if existingArch != nil {
		arch = *existingArch
		return arch
	}
	randomNum := rand.Intn(20)
	if randomNum < 4 {
		arch = ""
	}
	if randomNum > 4 && randomNum < 6 {
		arch = "s390x"
	}
	return arch
}

func SeedRepository(db *gorm.DB, size int) error {
	var repoConfigs []models.RepositoryConfiguration
	var repos []models.Repository

	// Retrieve all the repos
	if r := db.Find(&repoConfigs); r != nil && r.Error != nil {
		return r.Error
	}

	// For each repo add 'size' rpm random packages
	countRecords := 0
	for _, repoConfig := range repoConfigs {
		referRepoConfig := pointy.String(repoConfig.UUID)
		for i := 0; i < size; i++ {
			arch := "x86_64"
			repo := models.Repository{
				URL:             fmt.Sprintf("https://%s.com/%s", RandStringBytes(12), arch),
				ReferRepoConfig: referRepoConfig,
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
	}

	// Add remaining records
	if len(repos) > 0 {
		if r := db.Create(repos); r != nil && r.Error != nil {
			return r.Error
		}
		countRecords += len(repos)
		repos = []models.Repository{}
		fmt.Printf("repoConfig: %d        \r", countRecords)
	}

	return nil
}

// SeedRepositoryRpms Populate database with random package information
// db The database descriptor.
// size The number of rpm packages per repository to be generated.
func SeedRepositoryRpms(db *gorm.DB, size int) error {
	var repos []models.Repository
	var rpms []models.RepositoryRpm

	archs := []string{
		"x86_64",
		"noarch",
	}

	// Retrieve all the repos
	if r := db.Find(&repos); r != nil && r.Error != nil {
		return r.Error
	}

	// For each repo add 'size' rpm random packages
	countRecords := 0
	for _, repo := range repos {
		for i := 0; i < size; i++ {
			rpm := models.RepositoryRpm{
				Name:      fmt.Sprintf("%s", RandStringBytes(12)),
				Arch:      archs[rand.Int()%2],
				Version:   fmt.Sprintf("%d.%d.%d", rand.Int()%6, rand.Int()%16, rand.Int()%64),
				Release:   fmt.Sprintf("%d", rand.Int()%128),
				Epoch:     nil,
				ReferRepo: repo.Base.UUID,
				// Repo:      repo,
			}
			rpms = append(rpms, rpm)
			if len(rpms) >= batchSize {
				if r := db.Create(rpms); r != nil && r.Error != nil {
					return r.Error
				}
				countRecords += len(rpms)
				rpms = []models.RepositoryRpm{}
				fmt.Printf("RepositoryRpm: %d        \r", countRecords)
			}
		}
	}

	// Add remaining records
	if len(rpms) > 0 {
		if r := db.Create(rpms); r != nil && r.Error != nil {
			return r.Error
		}
		countRecords += len(rpms)
		rpms = []models.RepositoryRpm{}
		fmt.Printf("RepositoryRpm: %d        \r", countRecords)
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
