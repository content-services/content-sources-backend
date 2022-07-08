package models

import (
	"github.com/stretchr/testify/assert"
)

func (suite *ModelsSuite) TestRepositoryConfigurationCreate() {
	var err error
	tx := suite.tx
	t := suite.T()

	var repo = Repository{
		URL: "https://example.com",
	}
	err = tx.Create(&repo).Error
	assert.Nil(t, err)

	var repoConfig = RepositoryConfiguration{
		Name:           "foo",
		AccountID:      "1",
		OrgID:          "1",
		Versions:       []string{"1", "2", "3"},
		RepositoryUUID: repo.Base.UUID,
	}
	err = tx.Create(&repoConfig).Error
	assert.Nil(t, err)

	var found = RepositoryConfiguration{}
	err = tx.First(&found, "uuid = ?", repoConfig.UUID).Error
	assert.Nil(t, err)
	assert.NotEmpty(t, found.UUID)
	assert.Equal(t, repoConfig.Name, found.Name)
	assert.Equal(t, repoConfig.AccountID, found.AccountID)
	assert.Equal(t, repoConfig.OrgID, found.OrgID)
	assert.Equal(t, repoConfig.Versions, found.Versions)
	assert.Equal(t, repoConfig.RepositoryUUID, found.RepositoryUUID)
}
