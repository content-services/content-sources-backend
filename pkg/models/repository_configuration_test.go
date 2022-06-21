package models

import (
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/stretchr/testify/assert"
)

// func TestReposSuite(t *testing.T) {
// 	suite.Run(t, new(ModelsSuite))
// }

// type RepoConfigSuite struct {
// 	suite.Suite
// 	savedDB *gorm.DB
// }

// func (suite *RepoConfigSuite) SetupTest() {
// 	suite.savedDB = db.DB
// 	db.DB = db.DB.Begin()
// 	db.DB.Where("1=1").Delete(RepositoryConfiguration{})
// }

// func (suite *RepoConfigSuite) TearDownTest() {
// 	//Rollback and reset db.DB
// 	db.DB.Rollback()
// 	db.DB = suite.savedDB
// }

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
