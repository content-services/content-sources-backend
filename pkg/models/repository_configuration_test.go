package models

import (
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/stretchr/testify/assert"
)

func (suite *ModelsSuite) TestRepositoryConfigurationCreate() {
	var repoConfig = RepositoryConfiguration{
		Name:      "foo",
		URL:       "https://example.com",
		AccountID: "1",
		OrgID:     "1",
		Versions:  []string{"1", "2", "3"},
	}
	var found = RepositoryConfiguration{}

	res := db.DB.Create(&repoConfig)
	assert.Nil(suite.T(), res.Error)
	db.DB.Where("url = ?", repoConfig.URL).First(&found)
	assert.NotEmpty(suite.T(), found.UUID)
	assert.Equal(suite.T(), repoConfig.Versions, found.Versions)
}
