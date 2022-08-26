package dao

import (
	"strconv"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/lib/pq"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *RepositorySuite) TestCreate() {
	name := "Updated"
	url := "http://someUrl.com"
	orgID := seeds.RandomOrgId()
	accountId := seeds.RandomAccountId()
	distributionArch := "x86_64"
	var err error

	t := suite.T()
	tx := suite.tx

	var foundCount int64 = -1
	foundConfig := []models.RepositoryConfiguration{}
	err = tx.Limit(1).Find(&foundConfig).Error
	require.NoError(t, err)
	tx.Count(&foundCount)
	assert.Equal(t, int64(0), foundCount)

	toCreate := api.RepositoryRequest{
		Name:             &name,
		URL:              &url,
		OrgID:            &orgID,
		AccountID:        &accountId,
		DistributionArch: &distributionArch,
		DistributionVersions: &[]string{
			config.El9,
		},
	}

	dao := GetRepositoryDao(tx)
	created, err := dao.Create(toCreate)
	assert.Nil(t, err)

	foundRepo, err := dao.Fetch(orgID, created.UUID)
	assert.Nil(t, err)
	assert.Equal(t, url, foundRepo.URL)
}

func (suite *RepositorySuite) TestRepositoryCreateAlreadyExists() {
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepository(tx, 1)
	assert.NoError(t, err)
	var repo []models.Repository
	err = tx.Limit(1).Find(&repo).Error
	assert.NoError(t, err)

	err = seeds.SeedRepositoryConfigurations(tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.NoError(t, err)

	found := models.RepositoryConfiguration{}
	err = tx.
		First(&found, "org_id = ?", orgID).
		Error
	require.NoError(t, err)

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

	orgID := seeds.RandomOrgId()

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
		err = tx.
			Where("name = ? AND org_id = ?", requests[i].Name, orgID).
			Find(&foundRepoConfig).
			Error
		assert.NoError(t, err)
		assert.NotEmpty(t, foundRepoConfig.UUID)
	}
}

func (suite *RepositorySuite) TestBulkCreateOneFails() {
	t := suite.T()
	tx := suite.tx

	orgID := orgIDTest
	accountID := accountIdTest

	requests := []api.RepositoryRequest{
		{
			Name:      pointy.String(""),
			URL:       pointy.String("repo_2_url"),
			OrgID:     &orgID,
			AccountID: &accountID,
		},
		{
			Name:      pointy.String("repo_1"),
			URL:       pointy.String("repo_1_url"),
			OrgID:     &orgID,
			AccountID: &accountID,
		},
	}

	rr, err := GetRepositoryDao(tx).BulkCreate(requests)

	assert.Error(t, err)
	assert.Equal(t, len(requests), len(rr))
	assert.NotNil(t, rr[0].ErrorMsg)
	assert.Nil(t, rr[0].Repository)
	assert.Nil(t, rr[1].ErrorMsg)
	assert.Nil(t, rr[1].Repository)

	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.BadValidation)

	urls := []string{}
	for _, request := range requests {
		if request.URL != nil && *request.URL != "" {
			urls = append(urls, *request.URL)
		}
	}
	var count int64
	foundRepoConfig := []models.RepositoryConfiguration{}
	err = tx.Model(&models.RepositoryConfiguration{}).
		Where("repositories.url in (?)", urls).
		Where("repository_configurations.org_id = ?", orgID).
		Joins("inner join repositories on repository_configurations.repository_uuid = repositories.uuid").
		Count(&count).
		Find(&foundRepoConfig).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func (suite *RepositorySuite) TestUpdate() {
	name := "Updated"
	url := "http://someUrl.com"
	t := suite.T()
	orgID := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	err = suite.tx.
		Preload("Repository").
		First(&found, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)

	err = GetRepositoryDao(suite.tx).Update(found.OrgID, found.UUID,
		api.RepositoryRequest{
			Name: &name,
			URL:  &url,
		})
	assert.NoError(t, err)

	err = suite.tx.
		First(&found, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)
	assert.Equal(t, "Updated", found.Name)
}

func (suite *RepositorySuite) TestUpdateDuplicateVersions() {
	t := suite.T()

	err := seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{})
	duplicateVersions := []string{config.El7, config.El7}

	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.First(&found)
	err = GetRepositoryDao(suite.tx).Update(found.OrgID, found.UUID,
		api.RepositoryRequest{
			DistributionVersions: &duplicateVersions,
		})
	assert.Nil(t, err)

	res := suite.tx.Where("uuid = ?", found.UUID).First(&found)
	assert.Nil(t, res.Error)
	assert.Equal(t, pq.StringArray{config.El7}, found.Versions)
}

func (suite *RepositorySuite) TestUpdateEmpty() {
	name := "Updated"
	arch := ""
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()
	var err error

	// Create a RepositoryConfiguration record
	repoPublic := repoPublicTest.DeepCopy()
	err = tx.Create(&repoPublic).Error
	require.NoError(t, err)

	repoConfig := repoConfigTest1.DeepCopy()
	repoConfig.RepositoryUUID = repoPublic.UUID
	repoConfig.OrgID = orgID
	err = tx.Create(&repoConfig).Error
	require.NoError(t, err)

	// Retrieve the just created RepositoryConfiguration record
	found := models.RepositoryConfiguration{}
	err = tx.
		First(&found, "uuid = ? AND org_id = ?", repoConfig.UUID, orgID).
		Error
	require.NoError(t, err)
	assert.Equal(t, found.UUID, repoConfig.UUID)
	assert.Equal(t, found.AccountID, repoConfig.AccountID)
	assert.Equal(t, found.Arch, repoConfig.Arch)
	assert.Equal(t, found.Name, repoConfig.Name)
	assert.Equal(t, found.OrgID, repoConfig.OrgID)
	assert.Equal(t, found.RepositoryUUID, repoConfig.RepositoryUUID)
	assert.Equal(t, found.Versions, repoConfig.Versions)
	assert.NotEmpty(t, found.Arch)

	// Update the RepositoryConfiguration record using dao method
	err = GetRepositoryDao(tx).Update(found.OrgID, found.UUID,
		api.RepositoryRequest{
			Name:             &name,
			DistributionArch: &arch,
		})
	assert.NoError(t, err)

	// Check the updated data
	err = tx.
		First(&found, "uuid = ? AND org_id = ?", repoConfig.UUID, orgID).
		Error
	require.NoError(t, err)
	assert.Equal(t, name, found.Name)
	assert.Empty(t, found.Arch)
}

func (suite *RepositorySuite) TestDuplicateUpdate() {
	t := suite.T()
	var err error
	tx := suite.tx
	name := "testduplicateupdate - repository"
	url := "https://testduplicate.com"

	repo := repoPublicTest.DeepCopy()
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
	orgID := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	err = suite.tx.
		First(&found, "org_id = ?", orgID).
		Error
	require.NoError(t, err)

	err = GetRepositoryDao(suite.tx).Update("Wrong OrgID!! zomg hacker", found.UUID,
		api.RepositoryRequest{
			Name: &name,
			URL:  &name,
		})

	require.Error(t, err)
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
	orgID := orgIDTest

	repo := repoPublicTest.DeepCopy()
	err = tx.Create(&repo).Error
	require.NoError(t, err)

	repoConfig := repoConfigTest1.DeepCopy()
	repoConfig.RepositoryUUID = repo.UUID
	err = tx.Create(&repoConfig).Error
	require.NoError(t, err)

	found := models.RepositoryConfiguration{}
	err = tx.
		Preload("Repository").
		First(&found, "uuid = ? AND org_id = ?", repoConfig.UUID, orgID).
		Error
	require.NoError(t, err)

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
		assert.Error(t, err)
		if blankItems[i].expected == "" {
			assert.NoError(t, err)
		} else {
			require.Error(t, err)
			daoError, ok := err.(*Error)
			assert.True(t, ok)
			assert.True(t, daoError.BadValidation)
			assert.Contains(t, daoError.Message, blankItems[i].expected)
		}
		tx.RollbackTo("updateblanktest")
	}
}

func (suite *RepositorySuite) TestFetch() {
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	err = tx.
		Preload("Repository").
		First(&found, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)

	fetched, err := GetRepositoryDao(suite.tx).Fetch(found.OrgID, found.UUID)
	assert.Nil(t, err)
	assert.Equal(t, found.UUID, fetched.UUID)
	assert.Equal(t, found.Name, fetched.Name)
	assert.Equal(t, found.Repository.URL, fetched.URL)
}

func (suite *RepositorySuite) TestFetchNotFound() {
	t := suite.T()
	orgID := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	err = suite.tx.
		First(&found, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)

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
	accountID := seeds.RandomAccountId()
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
	tx := suite.tx
	var count int64
	repoUrls := []string{
		"https://somepublicRepo.com/",
		"https://anotherpublicRepo.com/",
	}

	// Create the two Repository records
	err := GetRepositoryDao(tx).SavePublicRepos(repoUrls)
	require.NoError(t, err)
	repo := []models.Repository{}
	err = tx.
		Model(&models.Repository{}).
		Where("url in (?)", repoUrls).
		Count(&count).
		Find(&repo).
		Error
	require.NoError(t, err)
	assert.Equal(t, int64(len(repo)), count)

	// Repeat to check clause on conflict
	err = GetRepositoryDao(suite.tx).SavePublicRepos(repoUrls)
	assert.NoError(t, err)
	err = tx.
		Model(&models.Repository{}).
		Where("url in (?)", repoUrls).
		Count(&count).
		Find(&repo).
		Error
	require.NoError(t, err)
	assert.Equal(t, int64(len(repo)), count)
}

func (suite *RepositorySuite) TestDelete() {
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepositoryConfigurations(tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	repoConfig := models.RepositoryConfiguration{}
	err = tx.
		First(&repoConfig, "org_id = ?", orgID).
		Error
	require.NoError(t, err)

	err = GetRepositoryDao(tx).Delete(repoConfig.OrgID, repoConfig.UUID)
	assert.NoError(t, err)

	repoConfig2 := models.RepositoryConfiguration{}
	err = tx.
		First(&repoConfig2, "org_id = ? AND uuid = ?", repoConfig.OrgID, repoConfig.UUID).
		Error
	require.Error(t, err)
	assert.Equal(t, "record not found", err.Error())
}

func (suite *RepositorySuite) TestDeleteNotFound() {
	t := suite.T()
	orgID := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	found := models.RepositoryConfiguration{}
	err = suite.tx.
		First(&found, "org_id = ?", orgID).
		Error
	require.NoError(t, err)

	err = GetRepositoryDao(suite.tx).Delete("bad org id", found.UUID)
	assert.Error(t, err)
	daoError, ok := err.(*Error)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	err = suite.tx.
		First(&found, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)
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
	orgId := seeds.RandomOrgId()
	err := seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgId})
	assert.NoError(t, err)

	mockExtRDao := mockExternalResource{}
	dao := repositoryDaoImpl{
		db:        suite.tx,
		extResDao: &mockExtRDao,
	}

	repoConfig := models.RepositoryConfiguration{}
	err = suite.tx.
		Preload("Repository").
		First(&repoConfig, "org_id = ?", orgId).
		Error
	require.NoError(t, err)
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
	require.NoError(t, err)
	assert.False(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.Contains(t, response.Name.Error, "already exists.")
	assert.False(t, response.URL.Valid)
	assert.False(t, response.URL.Skipped)
	assert.Contains(t, response.URL.Error, "already exists.")

	response, err = dao.ValidateParameters(orgId, parameters[1])
	require.NoError(t, err)
	assert.False(t, response.Name.Valid)
	assert.True(t, response.Name.Skipped)
	assert.False(t, response.URL.Valid)
	assert.True(t, response.URL.Skipped)

	response, err = dao.ValidateParameters(orgId, parameters[2])
	require.NoError(t, err)
	assert.False(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.Contains(t, response.Name.Error, "blank")
	assert.False(t, response.URL.Valid)
	assert.False(t, response.URL.Skipped)
	assert.Contains(t, response.URL.Error, "blank")

	mockExtRDao.Mock.On("ValidRepoMD", "http://foobar.com").Return(200, nil)
	response, err = dao.ValidateParameters(orgId, parameters[3])
	require.NoError(t, err)
	assert.True(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.True(t, response.URL.Valid)
	assert.True(t, response.URL.MetadataPresent)
	assert.False(t, response.URL.Skipped)

	mockExtRDao.Mock.On("ValidRepoMD", "http://badrepo.com").Return(404, nil)
	response, err = dao.ValidateParameters(orgId, parameters[4])
	require.NoError(t, err)
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
	require.NoError(t, err)
	assert.True(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.True(t, response.URL.Valid)
	assert.Equal(t, response.URL.HTTPCode, 0)
	assert.False(t, response.URL.MetadataPresent)
	assert.Contains(t, response.URL.Error, "Timeout")
	assert.False(t, response.URL.Skipped)
}
