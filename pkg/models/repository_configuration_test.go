package models

import (
	"testing"

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

	dbConn.Create(&repoConfig)
	uuid := repoConfig.UUID
	dbConn.First(&result, "uuid = ?", uuid)
	assert.NotEmpty(t, result)
}
