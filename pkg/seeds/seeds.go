package seeds

import (
	"errors"
	"fmt"
	"math/rand"
	"strconv"

	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

type SeedOptions struct {
	OrgID string
}

func SeedRepositoryConfigurations(db *gorm.DB, size int, options SeedOptions) error {
	var repos []models.RepositoryConfiguration

	for i := 0; i < size; i++ {
		repoConfig := models.RepositoryConfiguration{
			Name:      fmt.Sprintf("%s - %s - %s", RandStringBytes(2), "TestRepo", RandStringBytes(10)),
			URL:       fmt.Sprintf("https://%s.com/%s", RandStringBytes(20), RandStringBytes(5)),
			Versions:  createVersionArray(i),
			Arch:      createArch(i),
			AccountID: strconv.Itoa(rand.Intn(9999)),
			OrgID:     createOrgId(options.OrgID, i),
		}

		repos = append(repos, repoConfig)
	}
	result := db.Create(&repos)
	if result.Error != nil {
		return errors.New("could not save seed")
	}
	return nil
}

func createOrgId(existingOrgId string, index int) string {
	orgId := "4234"
	if existingOrgId != "" {
		orgId = existingOrgId
	} else {
		// Only add random numbers if no specific existingOrgId is populated
		if index < 15 || index > 915 {
			orgId = strconv.Itoa(rand.Intn(9999))
		}
	}
	return orgId
}

func createVersionArray(index int) []string {
	versionArray := []string{}
	length := 0
	if index%2 == 0 {
		versionArray = []string{"7"}
	}
	if index%14 == 0 {
		length = 1
	}
	if index%17 == 0 {
		length = 2
	}
	if index%19 == 0 {
		length = 3
	}
	for k := 0; k < length; k++ {
		versionArray = append(versionArray, fmt.Sprintf("%d", 8+k))
	}
	return versionArray
}

func createArch(index int) string {
	arch := "x86_64"
	if index%3 == 0 {
		arch = ""
	}
	if index%20 == 0 {
		arch = "s390x"
	}
	return arch
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
