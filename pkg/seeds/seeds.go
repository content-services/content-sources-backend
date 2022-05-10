package seeds

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

func SeedRepositoryConfigurations(db *gorm.DB, size int) error {
	var repos []models.RepositoryConfiguration

	for i := 0; i < size; i++ {
		repoConfig := models.RepositoryConfiguration{
			Name:      fmt.Sprintf("%s - %s - %s", RandStringBytes(2), "TestRepo", RandStringBytes(10)),
			URL:       fmt.Sprintf("https://%s.com/%s", RandStringBytes(20), RandStringBytes(5)),
			Version:   "9",
			Arch:      "x86_64",
			AccountID: fmt.Sprintf("%d", rand.Intn(9999)),
			OrgID:     fmt.Sprintf("%d", rand.Intn(9999)),
		}
		repos = append(repos, repoConfig)
	}
	result := db.Create(&repos)
	if result.Error != nil {
		return errors.New("could not save seed")
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
