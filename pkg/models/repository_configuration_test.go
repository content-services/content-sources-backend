package models

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/stretchr/testify/assert"
)

func TestCreate(t *testing.T) {
	var repoConfig = RepositoryConfiguration{
		Name:      "foo",
		URL:       "https://example.com",
		AccountID: "1",
		OrgID:     "1",
	}
	var result = RepositoryConfiguration{}

	db.DB.Create(&repoConfig)
	uuid := repoConfig.UUID
	db.DB.First(&result, "uuid = ?", uuid)
	assert.NotEmpty(t, result)
}
