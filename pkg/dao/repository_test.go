package dao

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type ReposSuite struct {
	suite.Suite
	savedDB *gorm.DB
}

func (suite *ReposSuite) SetupTest() {
	suite.savedDB = db.DB
	db.DB = db.DB.Begin()
	db.DB.Where("1=1").Delete(models.RepositoryConfiguration{})
}

func (suite *ReposSuite) TearDownTest() {
	//Rollback and reset db.DB
	db.DB.Rollback()
	db.DB = suite.savedDB
}

func (suite *ReposSuite) TestCreate() {
	name := "Updated"
	url := "http://someUrl.com"
	orgId := "111"
	accountId := "222"

	t := suite.T()

	found := models.RepositoryConfiguration{}
	db.DB.First(&found)

	err := GetRepositoryDao().Create(api.RepositoryRequest{
		Name:      &name,
		URL:       &url,
		OrgID:     &orgId,
		AccountID: &accountId,
	})
	assert.Nil(t, err)

	db.DB.First(&found)
	assert.Equal(t, name, found.Name)
	assert.Equal(t, url, found.URL)
	assert.Equal(t, orgId, found.OrgID)
}

func (suite *ReposSuite) TestCreateAlreadyExists() {
	t := suite.T()
	err := seeds.SeedRepositoryConfigurations(db.DB, 1, seeds.SeedOptions{})
	assert.Nil(t, err)

	found := models.RepositoryConfiguration{}
	db.DB.First(&found)

	err = GetRepositoryDao().Create(api.RepositoryRequest{
		Name:      &found.Name,
		URL:       &found.URL,
		OrgID:     &found.OrgID,
		AccountID: &found.AccountID,
	})

	assert.NotNil(t, err)
	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.BadValidation)
}

func (suite *ReposSuite) TestUpdate() {
	name := "Updated"
	url := "http://someUrl.com"
	t := suite.T()
	err := seeds.SeedRepositoryConfigurations(db.DB, 1, seeds.SeedOptions{})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	db.DB.First(&found)

	err = GetRepositoryDao().Update(found.OrgID, found.UUID,
		api.RepositoryRequest{
			Name: &name,
			URL:  &url,
		})
	assert.Nil(t, err)

	db.DB.First(&found)
	assert.Equal(t, "Updated", found.Name)
	assert.Equal(t, "http://someUrl.com", found.URL)
}

func (suite *ReposSuite) TestUpdateEmpty() {
	name := "Updated"
	arch := ""
	t := suite.T()
	err := seeds.SeedRepositoryConfigurations(db.DB, 1, seeds.SeedOptions{})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	db.DB.First(&found)

	assert.NotEmpty(t, found.Arch)
	err = GetRepositoryDao().Update(found.OrgID, found.UUID,
		api.RepositoryRequest{
			Name:             &name,
			DistributionArch: &arch,
		})
	assert.Nil(t, err)

	db.DB.First(&found)
	assert.Equal(t, name, found.Name)
	assert.Empty(t, found.Arch)
}

func (suite *ReposSuite) TestDuplicateUpdate() {
	name := "unique"
	t := suite.T()
	err := seeds.SeedRepositoryConfigurations(db.DB, 1, seeds.SeedOptions{})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	db.DB.First(&found)

	err = GetRepositoryDao().Create(api.RepositoryRequest{OrgID: &found.OrgID, AccountID: &found.AccountID, Name: &name, URL: &name})
	assert.Nil(t, err)

	err = GetRepositoryDao().Update(found.OrgID, found.UUID,
		api.RepositoryRequest{
			Name: &name,
			URL:  &name,
		})

	assert.NotNil(t, err)
	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.BadValidation)
}

func (suite *ReposSuite) TestUpdateNotFound() {
	name := "unique"
	t := suite.T()
	err := seeds.SeedRepositoryConfigurations(db.DB, 1, seeds.SeedOptions{})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	db.DB.First(&found)

	err = GetRepositoryDao().Update("Wrong OrgID!! zomg hacker", found.UUID,
		api.RepositoryRequest{
			Name: &name,
			URL:  &name,
		})

	assert.NotNil(t, err)
	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

func (suite *ReposSuite) TestFetch() {
	t := suite.T()
	err := seeds.SeedRepositoryConfigurations(db.DB, 1, seeds.SeedOptions{})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	db.DB.First(&found)

	fetched, err := GetRepositoryDao().Fetch(found.OrgID, found.UUID)
	assert.Nil(t, err)
	assert.Equal(t, found.UUID, fetched.UUID)
	assert.Equal(t, found.Name, fetched.Name)
}

func (suite *ReposSuite) TestFetchNotFound() {
	t := suite.T()
	err := seeds.SeedRepositoryConfigurations(db.DB, 1, seeds.SeedOptions{})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	db.DB.First(&found)

	_, err = GetRepositoryDao().Fetch("bad org id", found.UUID)
	assert.NotNil(t, err)
	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

func (suite *ReposSuite) TestList() {
	t := suite.T()
	repoConfig := models.RepositoryConfiguration{}
	orgID := "1028"
	var total int64

	err := seeds.SeedRepositoryConfigurations(db.DB, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	result := db.DB.Where("org_id = ?", orgID).Find(&repoConfig).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(1), total)

	response, total, err := GetRepositoryDao().List(orgID, 100, 0)
	assert.Nil(t, err)
	assert.Equal(t, repoConfig.Name, response.Data[0].Name)
	assert.Equal(t, repoConfig.URL, response.Data[0].URL)
	assert.Equal(t, int64(1), total)
}

func (suite *ReposSuite) TestListNoRepositories() {
	t := suite.T()
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	orgID := "1028"
	limit := 100
	offset := 0
	var total int64

	result := db.DB.Where("org_id = ?", orgID).Find(&repoConfigs).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(0), total)

	response, total, err := GetRepositoryDao().List(orgID, limit, offset)
	assert.Nil(t, err)
	assert.Empty(t, response.Data)
	assert.Equal(t, int64(0), total)
}

func (suite *ReposSuite) TestListPageLimit() {
	t := suite.T()
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	orgID := "1028"
	limit := 10
	offset := 0
	var total int64

	err := seeds.SeedRepositoryConfigurations(db.DB, 20, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	result := db.DB.Where("org_id = ?", orgID).Find(&repoConfigs).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(20), total)

	response, total, err := GetRepositoryDao().List(orgID, limit, offset)
	assert.Nil(t, err)
	assert.Equal(t, len(response.Data), limit)
	assert.Equal(t, int64(20), total)
}

func (suite *ReposSuite) TestDelete() {
	t := suite.T()

	err := seeds.SeedRepositoryConfigurations(db.DB, 1, seeds.SeedOptions{})
	assert.Nil(t, err)

	repoConfig := models.RepositoryConfiguration{}
	result := db.DB.First(&repoConfig)
	assert.Nil(t, result.Error)

	err = GetRepositoryDao().Delete(repoConfig.OrgID, repoConfig.UUID)
	assert.Nil(t, err)

	result = db.DB.First(&repoConfig)
	assert.Error(t, result.Error)

}

func (suite *ReposSuite) TestDeleteNotFound() {
	t := suite.T()

	err := seeds.SeedRepositoryConfigurations(db.DB, 1, seeds.SeedOptions{})
	assert.Nil(t, err)

	found := models.RepositoryConfiguration{}
	result := db.DB.First(&found)
	assert.Nil(t, result.Error)

	err = GetRepositoryDao().Delete("bad org id", found.UUID)
	assert.NotNil(t, err)
	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	result = db.DB.First(&found)
	assert.Nil(t, result.Error)

}

func TestReposSuite(t *testing.T) {
	suite.Run(t, new(ReposSuite))
}
