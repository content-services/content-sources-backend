package dao

import (
	"math/rand"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/stretchr/testify/assert"
)

func (suite *RepositorySuite) TestCreate() {
	name := "Updated"
	url := "http://someUrl.com"
	orgId := "111"
	accountId := "222"
	distributionArch := "x86_64"
	var err error

	t := suite.T()
	tx := suite.tx

	var foundCount int64 = -1
	foundConfig := []models.RepositoryConfiguration{}
	err = tx.Limit(1).Find(&foundConfig).Error
	assert.NoError(t, err)
	if err == nil {
		tx.Count(&foundCount)
	}
	assert.Equal(t, int64(0), foundCount)

	dao := GetRepositoryDao(tx)
	_, err = dao.Create(api.RepositoryRequest{
		Name:             &name,
		URL:              &url,
		OrgID:            &orgId,
		AccountID:        &accountId,
		DistributionArch: &distributionArch,
		DistributionVersions: &[]string{
			"7", "8", "9",
		},
	})
	assert.Nil(t, err)

	var foundRepo models.Repository
	tx.First(&foundRepo)
	assert.Equal(t, url, foundRepo.URL)
}

func (suite *RepositorySuite) TestRepositoryCreateAlreadyExists() {
	t := suite.T()
	tx := suite.tx
	org_id := "900023"
	var err error

	err = seeds.SeedRepository(suite.tx, 1)
	assert.Nil(t, err)
	var repo []models.Repository
	err = tx.Limit(1).Find(&repo).Error
	assert.Nil(t, err)

	err = seeds.SeedRepositoryConfigurations(suite.tx /*, &repo[0]*/, 1, seeds.SeedOptions{OrgID: org_id})
	assert.Nil(t, err)

	found := models.RepositoryConfiguration{}
	tx.First(&found)

	_, err = GetRepositoryDao(suite.tx).Create(api.RepositoryRequest{
		Name:      &found.Name,
		OrgID:     &found.OrgID,
		AccountID: &found.AccountID,
	})

	assert.NotNil(t, err)
	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.BadValidation)
}

func (suite *RepositorySuite) TestRepositoryCreateBlankTest() {
	t := suite.T()

	blank := ""
	name := "name"
	url := "http://foobar.com"
	OrgID := "34"
	AccountID := "34"

	blankItems := []api.RepositoryRequest{
		api.RepositoryRequest{
			Name:      &blank,
			URL:       &url,
			OrgID:     &OrgID,
			AccountID: &AccountID,
		},
		api.RepositoryRequest{
			Name:      &name,
			URL:       &blank,
			OrgID:     &OrgID,
			AccountID: &AccountID,
		},
		api.RepositoryRequest{
			Name:      &name,
			URL:       &url,
			OrgID:     &blank,
			AccountID: &AccountID,
		},
		api.RepositoryRequest{
			Name:      &name,
			URL:       &url,
			OrgID:     &OrgID,
			AccountID: &blank,
		},
	}
	for i := 0; i < len(blankItems); i++ {
		_, err := GetRepositoryDao(suite.db).Create(blankItems[i])
		assert.NotNil(t, err)
		daoError, ok := err.(*Error)
		assert.True(t, ok)
		// assert.True(t, daoError.BadValidation)
		assert.Contains(t, daoError.Message, "ERROR: null value in column")
	}
}

func (suite *RepositorySuite) TestUpdate() {
	name := "Updated"
	url := "http://someUrl.com"
	t := suite.T()
	org_id := "900023"
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx /*, &repo*/, 1, seeds.SeedOptions{OrgID: org_id})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.First(&found)

	err = GetRepositoryDao(suite.tx).Update(found.OrgID, found.UUID,
		api.RepositoryRequest{
			Name: &name,
			URL:  &url,
		})
	assert.Nil(t, err)

	suite.tx.First(&found)
	assert.Equal(t, "Updated", found.Name)
}

func (suite *RepositorySuite) TestUpdateEmpty() {
	name := "Updated"
	arch := ""
	t := suite.T()
	org_id := "900023"
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx /*, &repo*/, 1, seeds.SeedOptions{OrgID: org_id})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.First(&found)

	assert.NotEmpty(t, found.Arch)
	err = GetRepositoryDao(suite.tx).Update(found.OrgID, found.UUID,
		api.RepositoryRequest{
			Name:             &name,
			DistributionArch: &arch,
		})
	assert.Nil(t, err)

	suite.tx.First(&found)
	assert.Equal(t, name, found.Name)
	assert.Empty(t, found.Arch)
}

// TODO Review and probably remove this
func randomString(size int) string {
	allowedMap := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	output := make([]byte, size)
	for idx := 0; idx < size; idx++ {
		output[idx] = allowedMap[rand.Intn(len(allowedMap))]
	}
	return string(output)
}

// TODO Review and probably remove this
func randomURL() string {
	someTLD := []string{
		".org",
		".com",
		".es",
		".pt",
	}
	return "https://www." + randomString(20) + someTLD[rand.Intn(len(someTLD))]
}

func (suite *RepositorySuite) TestDuplicateUpdate() {
	t := suite.T()
	var err error
	tx := suite.tx
	name := "testduplicateupdate - repository"
	url := "https://testduplicate.com"

	repo := repoTest1.DeepCopy()
	repoConfig := repoConfigTest1.DeepCopy()
	var created1 api.RepositoryResponse

	created1, err = GetRepositoryDao(suite.tx).
		Create(api.RepositoryRequest{
			OrgID:     &repoConfig.OrgID,
			AccountID: &repoConfig.AccountID,
			Name:      &repoConfig.Name,
			URL:       &repo.URL,
		})
	assert.NoError(t, err)

	_, err = GetRepositoryDao(suite.tx).
		Create(api.RepositoryRequest{
			OrgID:     &created1.OrgID,
			AccountID: &created1.AccountID,
			Name:      &name,
			URL:       &url})
	assert.NoError(t, err)

	err = GetRepositoryDao(tx).Update(
		created1.OrgID,
		created1.UUID,
		api.RepositoryRequest{
			Name: &created1.Name,
			URL:  &created1.URL,
		})
	assert.Error(t, err)

	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.BadValidation)
}

func (suite *RepositorySuite) TestUpdateNotFound() {
	name := "unique"
	t := suite.T()
	org_id := "900023"
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx /*, &repo*/, 1, seeds.SeedOptions{OrgID: org_id})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.First(&found)

	err = GetRepositoryDao(suite.tx).Update("Wrong OrgID!! zomg hacker", found.UUID,
		api.RepositoryRequest{
			Name: &name,
			URL:  &name,
		})

	assert.NotNil(t, err)
	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

func (suite *RepositorySuite) TestFetch() {
	t := suite.T()
	org_id := "900023"
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx /*, &repo*/, 1, seeds.SeedOptions{OrgID: org_id})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.First(&found)

	fetched, err := GetRepositoryDao(suite.tx).Fetch(found.OrgID, found.UUID)
	assert.Nil(t, err)
	assert.Equal(t, found.UUID, fetched.UUID)
	assert.Equal(t, found.Name, fetched.Name)
}

func (suite *RepositorySuite) TestFetchNotFound() {
	t := suite.T()
	org_id := "900023"
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx /*, &repo*/, 1, seeds.SeedOptions{OrgID: org_id})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.First(&found)

	_, err = GetRepositoryDao(suite.tx).Fetch("bad org id", found.UUID)
	assert.NotNil(t, err)
	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

func (suite *RepositorySuite) TestList() {
	t := suite.T()
	repoConfig := models.RepositoryConfiguration{}
	orgID := "1028"
	var total int64
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: "",
	}
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx /*, &repo*/, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	result := suite.tx.Where("org_id = ?", orgID).Find(&repoConfig).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(1), total)

	response, total, err := GetRepositoryDao(suite.tx).List(orgID, pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, repoConfig.Name, response.Data[0].Name)
	assert.Equal(t, int64(1), total)
}

func (suite *RepositorySuite) TestListNoRepositories() {
	t := suite.T()
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	orgID := "1028"
	var total int64
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: "",
	}

	result := suite.tx.Where("org_id = ?", orgID).Find(&repoConfigs).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(0), total)

	response, total, err := GetRepositoryDao(suite.tx).List(orgID, pageData, filterData)
	assert.Nil(t, err)
	assert.Empty(t, response.Data)
	assert.Equal(t, int64(0), total)
}

func (suite *RepositorySuite) TestListPageLimit() {
	t := suite.T()
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	orgID := "1028"
	pageData := api.PaginationData{
		Limit:  10,
		Offset: 0,
	}
	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: "",
	}

	var total int64
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx /*, &repo*/, 20, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	result := suite.tx.Where("org_id = ?", orgID).Find(&repoConfigs).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(20), total)

	response, total, err := GetRepositoryDao(suite.tx).List(orgID, pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, len(response.Data), pageData.Limit)
	assert.Equal(t, int64(20), total)
}

func (suite *RepositorySuite) TestListFilterVersion() {
	t := suite.T()

	orgID := "1028"
	pageData := api.PaginationData{
		Limit:  20,
		Offset: 0,
	}
	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: "9",
	}

	var total int64

	quantity := 20

	assert.Nil(t, seeds.SeedRepositoryConfigurations(db.DB, quantity, seeds.SeedOptions{OrgID: orgID, Versions: &[]string{"9"}}))

	response, total, err := GetRepositoryDao(suite.tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, quantity, int(total))
}

func (suite *RepositorySuite) TestListFilterArch() {
	t := suite.T()
	tx := suite.tx
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	orgID := "4234"
	pageData := api.PaginationData{
		Limit:  20,
		Offset: 0,
	}

	filterData := api.FilterData{
		Search:  "",
		Arch:    "s390x",
		Version: "",
	}

	var total int64

	quantity := 20

	err := seeds.SeedRepositoryConfigurations(tx, quantity, seeds.SeedOptions{OrgID: orgID, Arch: &filterData.Arch})
	assert.Nil(t, err)

	result := tx.
		Where("org_id = ? AND arch = ?", orgID, filterData.Arch).
		Find(&repoConfigs).
		Count(&total)

	assert.Nil(t, result.Error)
	assert.Equal(t, int64(quantity), total)

	response, total, err := GetRepositoryDao(tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(quantity), total)
}

func (suite *RepositorySuite) TestListFilterMultipleArch() {
	t := suite.T()
	orgID := "4234"
	pageData := api.PaginationData{
		Limit:  20,
		Offset: 0,
	}

	filterData := api.FilterData{
		Search:  "",
		Arch:    "x86_64,s390x",
		Version: "",
	}

	quantity := 20

	x86ref := "x86_64"
	s390xref := "s390x"

	assert.Nil(t, seeds.SeedRepositoryConfigurations(db.DB, quantity, seeds.SeedOptions{OrgID: orgID, Arch: &x86ref}))
	assert.Nil(t, seeds.SeedRepositoryConfigurations(db.DB, quantity, seeds.SeedOptions{OrgID: orgID, Arch: &s390xref}))

	response, count, err := GetRepositoryDao(suite.tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(40), count)
}

func (suite *RepositorySuite) TestListFilterMultipleVersions() {
	t := suite.T()
	orgID := "4234"
	pageData := api.PaginationData{
		Limit:  20,
		Offset: 0,
	}

	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: "7,9",
	}

	quantity := 20

	err := seeds.SeedRepositoryConfigurations(suite.tx, quantity, seeds.SeedOptions{OrgID: orgID, Versions: &[]string{"7", "8", "9"}})
	assert.Nil(t, err)

	response, count, err := GetRepositoryDao(suite.tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(quantity), count)
}

func (suite *RepositorySuite) TestDelete() {
	t := suite.T()
	tx := suite.tx
	org_id := "900023"
	var err error

	err = seeds.SeedRepositoryConfigurations(tx, 1 /*, &repo*/, seeds.SeedOptions{OrgID: org_id})
	assert.Nil(t, err)

	repoConfig := models.RepositoryConfiguration{}
	err = tx.First(&repoConfig).Error
	assert.Nil(t, err)

	err = GetRepositoryDao(tx).Delete(repoConfig.OrgID, repoConfig.UUID)
	assert.Nil(t, err)

	repoConfig2 := models.RepositoryConfiguration{}
	err = tx.Where("org_id = ? AND uuid = ?", repoConfig.OrgID, repoConfig.UUID).
		First(&repoConfig2).Error
	assert.NotNil(t, err)
	if err != nil {
		assert.Equal(t, "record not found", err.Error())
	}
}

func (suite *RepositorySuite) TestDeleteNotFound() {
	t := suite.T()
	org_id := "900023"
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx /* &repo,*/, 1, seeds.SeedOptions{OrgID: org_id})
	assert.Nil(t, err)

	found := models.RepositoryConfiguration{}
	result := suite.tx.First(&found)
	assert.Nil(t, result.Error)

	err = GetRepositoryDao(suite.tx).Delete("bad org id", found.UUID)
	assert.NotNil(t, err)
	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	result = suite.tx.First(&found)
	assert.Nil(t, result.Error)
}
