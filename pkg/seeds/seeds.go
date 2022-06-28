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
	OrgID    string
	Arch     *string
	Versions *[]string
}

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
	result := db.Create(&repos)
	if result.Error != nil {
		return errors.New("could not save seed")
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

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
