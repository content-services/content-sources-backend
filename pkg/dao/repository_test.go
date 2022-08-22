package dao

import (
	"strconv"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *RepositorySuite) TestCreate() {
	name := "Updated"
	url := "http://someUrl.com"
	orgId := seeds.RandomOrgId()
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

	toCreate := api.RepositoryRequest{
		Name:             &name,
		URL:              &url,
		OrgID:            &orgId,
		AccountID:        &accountId,
		DistributionArch: &distributionArch,
		DistributionVersions: &[]string{
			config.El9,
		},
	}

	dao := GetRepositoryDao(tx)
	created, err := dao.Create(toCreate)
	assert.Nil(t, err)

	foundRepo, err := dao.Fetch(orgId, created.UUID)
	assert.Nil(t, err)
	assert.Equal(t, url, foundRepo.URL)
}

func (suite *RepositorySuite) TestRepositoryCreateAlreadyExists() {
	t := suite.T()
	tx := suite.tx
	org_id := "900023"
	var err error

	err = seeds.SeedRepository(tx, 1)
	assert.NoError(t, err)
	var repo []models.Repository
	err = tx.Limit(1).Find(&repo).Error
	assert.NoError(t, err)

	err = seeds.SeedRepositoryConfigurations(tx, 1, seeds.SeedOptions{OrgID: org_id})
	assert.NoError(t, err)

	found := models.RepositoryConfiguration{}
	tx.First(&found)

	// Force failure on creating duplicate
	_, err = GetRepositoryDao(tx).Create(api.RepositoryRequest{
		Name:      &found.Name,
		OrgID:     &found.OrgID,
		AccountID: &found.AccountID,
	})

	assert.Error(t, err)
	if err != nil {
		daoError, ok := err.(*Error)
		assert.True(t, ok)
		if ok {
			assert.True(t, daoError.BadValidation)
		}
	}
}

func (suite *RepositorySuite) TestRepositoryCreateBlank() {
	t := suite.T()
	tx := suite.tx

	blank := ""
	name := "name"
	url := "http://foobar.com"
	OrgID := seeds.RandomOrgId()

	type testCases struct {
		given    api.RepositoryRequest
		expected string
	}
	blankItems := []testCases{
		{
			given: api.RepositoryRequest{
				Name:  &blank,
				URL:   &url,
				OrgID: &OrgID,
			},
			expected: "Name cannot be blank.",
		},
		{
			given: api.RepositoryRequest{
				Name:  &name,
				URL:   &blank,
				OrgID: &OrgID,
			},
			expected: "URL cannot be blank.",
		},
		{
			given: api.RepositoryRequest{
				Name:  &name,
				URL:   &url,
				OrgID: &blank,
			},
			expected: "Org ID cannot be blank.",
		},
	}
	tx.SavePoint("testrepositorycreateblanktest")
	for i := 0; i < len(blankItems); i++ {
		_, err := GetRepositoryDao(tx).Create(blankItems[i].given)
		assert.NotNil(t, err)
		if blankItems[i].expected == "" {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
			if err != nil {
				daoError, ok := err.(*Error)
				assert.True(t, ok)
				assert.True(t, daoError.BadValidation)
				assert.Contains(t, daoError.Message, blankItems[i].expected)
			}
		}
		tx.RollbackTo("testrepositorycreateblanktest")
	}
}

func (suite *RepositorySuite) TestBulkCreate() {
	t := suite.T()
	tx := suite.tx

	orgID := "1"

	amountToCreate := 15

	requests := make([]api.RepositoryRequest, amountToCreate)
	for i := 0; i < amountToCreate; i++ {
		name := "repo_" + strconv.Itoa(i)
		url := "https://repo_" + strconv.Itoa(i)
		requests[i] = api.RepositoryRequest{
			Name:  &name,
			URL:   &url,
			OrgID: &orgID,
		}
	}

	rr, err := GetRepositoryDao(tx).BulkCreate(requests)
	assert.Nil(t, err)
	assert.Equal(t, amountToCreate, len(rr))

	for i := 0; i < amountToCreate; i++ {
		var foundRepoConfig models.RepositoryConfiguration
		result := tx.Where("name = ? ", requests[i].Name).Find(&foundRepoConfig)
		assert.Nil(t, result.Error)
		assert.NotEmpty(t, foundRepoConfig.UUID)
	}
}

func (suite *RepositorySuite) TestBulkCreateOneFails() {
	t := suite.T()
	tx := suite.tx

	orgID := "1"
	accountID := "2"

	requests := []api.RepositoryRequest{
		{
			Name:      pointy.String("repo_1"),
			URL:       pointy.String("repo_1_url"),
			OrgID:     &orgID,
			AccountID: &accountID,
		},
		{
			Name:  pointy.String(""),
			URL:   pointy.String("repo_2_url"),
			OrgID: &orgID,
		},
	}

	rr, err := GetRepositoryDao(tx).BulkCreate(requests)

	assert.Error(t, err)
	assert.Equal(t, 2, len(rr))

	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.BadValidation)

	foundRepoConfig := models.RepositoryConfiguration{}
	err = tx.First(&foundRepoConfig).Error
	assert.Error(t, err)
}

func (suite *RepositorySuite) TestUpdate() {
	name := "Updated"
	url := "http://someUrl.com"
	t := suite.T()
	org_id := "900023"
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: org_id})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.
		Preload("Repository").
		First(&found)

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
	org_id := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1,
		seeds.SeedOptions{OrgID: org_id, Arch: pointy.String(config.X8664)})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.Where("org_id = ?", org_id).First(&found)

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

func (suite *RepositorySuite) TestDuplicateUpdate() {
	t := suite.T()
	var err error
	tx := suite.tx
	name := "testduplicateupdate - repository"
	url := "https://testduplicate.com"

	repo := repoTest1.DeepCopy()
	repoConfig := repoConfigTest1.DeepCopy()
	var created1 api.RepositoryResponse
	var created2 api.RepositoryResponse

	created1, err = GetRepositoryDao(suite.tx).
		Create(api.RepositoryRequest{
			OrgID:     &repoConfig.OrgID,
			AccountID: &repoConfig.AccountID,
			Name:      &repoConfig.Name,
			URL:       &repo.URL,
		})
	assert.NoError(t, err)

	created2, err = GetRepositoryDao(suite.tx).
		Create(api.RepositoryRequest{
			OrgID:     &created1.OrgID,
			AccountID: &created1.AccountID,
			Name:      &name,
			URL:       &url})
	assert.NoError(t, err)

	err = GetRepositoryDao(tx).Update(
		created2.OrgID,
		created2.UUID,
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
	orgId := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgId})
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

func (suite *RepositorySuite) TestUpdateBlank() {
	t := suite.T()
	tx := suite.tx

	var err error
	name := "Updated"
	url := "http://someUrl.com"
	blank := ""
	orgID := "900023"

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.Preload("Repository").First(&found)

	type testCases struct {
		given    api.RepositoryRequest
		expected string
	}
	blankItems := []testCases{
		{
			given: api.RepositoryRequest{
				Name: &blank,
				URL:  &url,
			},
			expected: "Name cannot be blank.",
		},
		{
			given: api.RepositoryRequest{
				Name: &name,
				URL:  &blank,
			},
			expected: "URL cannot be blank.",
		},
	}
	tx.SavePoint("updateblanktest")
	for i := 0; i < len(blankItems); i++ {
		err := GetRepositoryDao(tx).Update(orgID, found.UUID, blankItems[i].given)
		assert.NotNil(t, err)
		if blankItems[i].expected == "" {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
			if err != nil {
				daoError, ok := err.(*Error)
				assert.True(t, ok)
				assert.True(t, daoError.BadValidation)
				assert.Contains(t, daoError.Message, blankItems[i].expected)
			}
		}
		tx.RollbackTo("updateblanktest")
	}
}

func (suite *RepositorySuite) TestFetch() {
	t := suite.T()
	org_id := "900023"
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: org_id})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.
		Preload("Repository").
		First(&found)

	fetched, err := GetRepositoryDao(suite.tx).Fetch(found.OrgID, found.UUID)
	assert.Nil(t, err)
	assert.Equal(t, found.UUID, fetched.UUID)
	assert.Equal(t, found.Name, fetched.Name)
	assert.Equal(t, found.Repository.URL, fetched.URL)
}

func (suite *RepositorySuite) TestFetchNotFound() {
	t := suite.T()
	org_id := "900023"
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: org_id})
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
	orgID := seeds.RandomOrgId()
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

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	result := suite.tx.
		Preload("Repository").
		Where("org_id = ?", orgID).
		Find(&repoConfig).
		Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(1), total)

	response, total, err := GetRepositoryDao(suite.tx).List(orgID, pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 1, len(response.Data))
	if len(response.Data) > 0 {
		assert.Equal(t, repoConfig.Name, response.Data[0].Name)
		assert.Equal(t, repoConfig.Repository.URL, response.Data[0].URL)
	}
}

func (suite *RepositorySuite) TestListNoRepositories() {
	t := suite.T()
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	orgID := seeds.RandomOrgId()
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
	orgID := seeds.RandomOrgId()
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

	err = seeds.SeedRepositoryConfigurations(suite.tx, 20, seeds.SeedOptions{OrgID: orgID})
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

	orgID := seeds.RandomOrgId()
	pageData := api.PaginationData{
		Limit:  20,
		Offset: 0,
	}
	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: config.El9,
	}

	var total int64
	quantity := 20

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, quantity, seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El9}}))
	response, total, err := GetRepositoryDao(suite.tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, quantity, int(total))
}

func (suite *RepositorySuite) TestListFilterArch() {
	t := suite.T()
	tx := suite.tx
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	orgID := seeds.RandomOrgId()
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
	orgID := seeds.RandomOrgId()
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

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, quantity, seeds.SeedOptions{OrgID: orgID, Arch: &x86ref}))
	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, quantity, seeds.SeedOptions{OrgID: orgID, Arch: &s390xref}))

	response, count, err := GetRepositoryDao(suite.tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(40), count)
}

func (suite *RepositorySuite) TestListFilterMultipleVersions() {
	t := suite.T()
	orgID := seeds.RandomOrgId()
	pageData := api.PaginationData{
		Limit:  20,
		Offset: 0,
	}

	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: config.El7 + "," + config.El9,
	}

	quantity := 20

	err := seeds.SeedRepositoryConfigurations(suite.tx, quantity,
		seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El7, config.El8, config.El9}})
	assert.Nil(t, err)

	//Seed data to a 2nd org to verify no crossover
	err = seeds.SeedRepositoryConfigurations(suite.tx, quantity,
		seeds.SeedOptions{OrgID: "kdksfkdf", Versions: &[]string{config.El7, config.El8, config.El9}})
	assert.Nil(t, err)

	response, count, err := GetRepositoryDao(suite.tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(quantity), count)
}

func (suite *RepositorySuite) TestListFilterSearch() {
	t := suite.T()
	tx := suite.tx
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	orgID := seeds.RandomOrgId()
	accountID := "2222"
	name := "my repo"
	url := "http://testsearchfilter.com"
	var total, quantity int64
	quantity = 1

	pageData := api.PaginationData{
		Limit:  20,
		Offset: 0,
	}

	filterData := api.FilterData{
		Search:  "testsearchfilter",
		Arch:    "",
		Version: "",
	}

	_, err := GetRepositoryDao(tx).Create(api.RepositoryRequest{
		OrgID:     &orgID,
		AccountID: &accountID,
		Name:      &name,
		URL:       &url,
	})
	assert.Nil(t, err)

	result := tx.
		Where("org_id = ?", orgID).
		Find(&repoConfigs).
		Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, quantity, total)

	response, total, err := GetRepositoryDao(tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, int(quantity), len(response.Data))
	assert.Equal(t, quantity, total)

}

func (suite *RepositorySuite) TestSavePublicUrls() {
	t := suite.T()
	repoUrls := []string{"https://somepublicRepo.com/", "https://anotherpublicRepo.com/"}
	err := GetRepositoryDao(suite.tx).SavePublicRepos(repoUrls)

	require.NoError(t, err)
	repo := models.Repository{}
	result := suite.tx.Where("url = ?", repoUrls[0]).Find(&repo)

	assert.NoError(t, result.Error)
	var count int64
	suite.tx.Model(&repo).Count(&count)
	assert.Equal(t, int64(2), count)

	err = GetRepositoryDao(suite.tx).SavePublicRepos(repoUrls)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func (suite *RepositorySuite) TestDelete() {
	t := suite.T()
	tx := suite.tx
	org_id := "900023"
	var err error

	err = seeds.SeedRepositoryConfigurations(tx, 1, seeds.SeedOptions{OrgID: org_id})
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

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: org_id})
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

type MockTimeoutError struct {
	Message string
	Timeout bool
}

func (e MockTimeoutError) Error() string {
	return e.Message
}

func (suite *RepositorySuite) TestValidateParameters() {
	t := suite.T()
	orgId := "900023"
	err := seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgId})
	assert.NoError(t, err)

	mockExtRDao := mockExternalResource{}
	dao := repositoryDaoImpl{
		db:        suite.tx,
		extResDao: &mockExtRDao,
	}

	repoConfig := models.RepositoryConfiguration{}
	result := suite.tx.Where("org_id = ?", orgId).Preload("Repository").First(&repoConfig)
	assert.NoError(t, result.Error)
	parameters := []api.RepositoryValidationRequest{
		{ // Duplicated name and url
			Name: &repoConfig.Name,
			URL:  &repoConfig.Repository.URL,
		}, { // Not providing any name or url
			Name: nil,
			URL:  nil,
		}, { // Blank names
			Name: pointy.String(""),
			URL:  pointy.String(""),
		}, { // Providing a valid url & name
			Name: pointy.String("Some Other Name"),
			URL:  pointy.String("http://foobar.com"),
		}, { // Providing a bad url that doesn't have a repo
			Name: pointy.String("Some bad repo!"),
			URL:  pointy.String("http://badrepo.com"),
		}, { // Providing a timed out url
			Name: pointy.String("Some Timeout repo!"),
			URL:  pointy.String("http://timemeout.com"),
		},
	}

	response, err := dao.ValidateParameters(orgId, parameters[0])
	assert.NoError(t, err)
	assert.False(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.Contains(t, response.Name.Error, "already exists.")
	assert.False(t, response.URL.Valid)
	assert.False(t, response.URL.Skipped)
	assert.Contains(t, response.URL.Error, "already exists.")

	response, err = dao.ValidateParameters(orgId, parameters[1])
	assert.NoError(t, err)
	assert.False(t, response.Name.Valid)
	assert.True(t, response.Name.Skipped)
	assert.False(t, response.URL.Valid)
	assert.True(t, response.URL.Skipped)

	response, err = dao.ValidateParameters(orgId, parameters[2])
	assert.NoError(t, err)
	assert.False(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.Contains(t, response.Name.Error, "blank")
	assert.False(t, response.URL.Valid)
	assert.False(t, response.URL.Skipped)
	assert.Contains(t, response.URL.Error, "blank")

	mockExtRDao.Mock.On("ValidRepoMD", "http://foobar.com").Return(200, nil)
	response, err = dao.ValidateParameters(orgId, parameters[3])
	assert.NoError(t, err)
	assert.True(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.True(t, response.URL.Valid)
	assert.True(t, response.URL.MetadataPresent)
	assert.False(t, response.URL.Skipped)

	mockExtRDao.Mock.On("ValidRepoMD", "http://badrepo.com").Return(404, nil)
	response, err = dao.ValidateParameters(orgId, parameters[4])
	assert.NoError(t, err)
	assert.True(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.True(t, response.URL.Valid) //Even if the metadata isn't present, the URL itself is valid
	assert.Equal(t, response.URL.HTTPCode, 404)
	assert.False(t, response.URL.MetadataPresent)
	assert.False(t, response.URL.Skipped)

	timeoutErr := MockTimeoutError{
		Message: " (Client.Timeout exceeded while awaiting headers)",
		Timeout: true,
	}

	mockExtRDao.Mock.On("ValidRepoMD", "http://timemeout.com").Return(0, timeoutErr)
	response, err = dao.ValidateParameters(orgId, parameters[5])
	assert.NoError(t, err)
	assert.True(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.True(t, response.URL.Valid)
	assert.Equal(t, response.URL.HTTPCode, 0)
	assert.False(t, response.URL.MetadataPresent)
	assert.Contains(t, response.URL.Error, "Timeout")
	assert.False(t, response.URL.Skipped)

}
