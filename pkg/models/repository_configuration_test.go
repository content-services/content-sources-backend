package models

import (
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/lib/pq"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

func smallRepo(suite *ModelsSuite) Repository {
	tx := suite.tx
	t := suite.T()

	repo := Repository{
		URL: "http://example.com",
	}
	result := tx.Create(&repo)
	assert.Nil(t, result.Error)
	return repo
}

func (suite *ModelsSuite) TestRepositoryConfigurationCreate() {
	var err error
	tx := suite.tx
	t := suite.T()

	var repoConfig = RepositoryConfiguration{
		Name:           "foo",
		AccountID:      "1",
		OrgID:          "1",
		Versions:       []string{config.El7, config.El8, config.El9},
		RepositoryUUID: smallRepo(suite).Base.UUID,
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

func (suite *ModelsSuite) TestCreateInvalidVersion() {
	var repoConfig = RepositoryConfiguration{
		Name:           "foo",
		AccountID:      "1",
		OrgID:          "1",
		Versions:       []string{"redhat linux 3.14"},
		RepositoryUUID: smallRepo(suite).Base.UUID,
	}
	res := suite.tx.Create(&repoConfig)
	assert.NotNil(suite.T(), res.Error)
	assert.True(suite.T(), strings.Contains(res.Error.Error(), "version"))
}

func (suite *ModelsSuite) TestCreateDuplicateVersion() {
	var repoConfig = RepositoryConfiguration{
		Name:           "duplicateVersions",
		AccountID:      "1",
		OrgID:          "1",
		Versions:       []string{config.El7, config.El7, config.El8},
		RepositoryUUID: smallRepo(suite).Base.UUID,
	}
	res := suite.tx.Create(&repoConfig)
	assert.Nil(suite.T(), res.Error)
	var found = RepositoryConfiguration{}
	res = suite.tx.First(&found, "uuid = ?", repoConfig.UUID)
	assert.Nil(suite.T(), res.Error)
	assert.Equal(suite.T(), pq.StringArray{config.El7, config.El8}, found.Versions)

	found.Versions = []string{config.El7, config.El7, config.El8, config.El8, config.El9}
	res = suite.tx.Updates(&found)
	assert.Nil(suite.T(), res.Error)
	assert.Equal(suite.T(), pq.StringArray{config.El7, config.El8, config.El9}, found.Versions)
}

func (suite *ModelsSuite) TestCreateInvalidArch() {
	var repoConfig = RepositoryConfiguration{
		Name:           "foo",
		AccountID:      "1",
		OrgID:          "1",
		Arch:           "68000",
		RepositoryUUID: smallRepo(suite).Base.UUID,
	}
	res := suite.tx.Create(&repoConfig)
	assert.NotNil(suite.T(), res.Error)
	log.Error().Msg(res.Error.Error())
	assert.True(suite.T(), strings.Contains(res.Error.Error(), "arch"))
}
