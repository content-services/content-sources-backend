package dao

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/test"
	mockExt "github.com/content-services/content-sources-backend/pkg/test/mocks/mock_external"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type RepositoryConfigSuite struct {
	*DaoSuite
}

func (suite *RepositoryConfigSuite) SetupTest() {
	if db.DB == nil {
		if err := db.Connect(); err != nil {
			suite.FailNow(err.Error())
		}
	}
	suite.db = db.DB
	suite.skipDefaultTransactionOld = suite.db.SkipDefaultTransaction
	suite.db.SkipDefaultTransaction = false
	suite.tx = suite.db.Begin()
}

func TestRepositoryConfigSuite(t *testing.T) {
	m := DaoSuite{}
	r := RepositoryConfigSuite{DaoSuite: &m}
	suite.Run(t, &r)
}

func (suite *RepositoryConfigSuite) TestCreate() {
	name := "Updated"
	url := "http://example.com/"
	orgID := seeds.RandomOrgId()
	accountId := seeds.RandomAccountId()
	distributionArch := "x86_64"
	gpgKey := "foo"
	metadataVerification := true
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
		GpgKey:               &gpgKey,
		MetadataVerification: &metadataVerification,
	}

	dao := GetRepositoryConfigDao(tx)
	created, err := dao.Create(toCreate)
	assert.Nil(t, err)

	foundRepo, err := dao.Fetch(orgID, created.UUID)
	assert.Nil(t, err)
	assert.Equal(t, url, foundRepo.URL)
}

func (suite *RepositoryConfigSuite) TestCreateTwiceWithNoSlash() {
	toCreate := api.RepositoryRequest{
		Name:             pointy.String(""),
		URL:              pointy.String("something-no-slash"),
		OrgID:            pointy.String("123"),
		AccountID:        pointy.String("123"),
		DistributionArch: pointy.String(""),
		DistributionVersions: &[]string{
			config.El9,
		},
	}
	dao := GetRepositoryConfigDao(suite.tx)
	_, err := dao.Create(toCreate)
	assert.ErrorContains(suite.T(), err, "Name cannot be blank")

	dao = GetRepositoryConfigDao(suite.tx)
	_, err = dao.Create(toCreate)
	assert.ErrorContains(suite.T(), err, "Name cannot be blank")
}

func (suite *RepositoryConfigSuite) TestRepositoryCreateAlreadyExists() {
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepository(tx, 1, seeds.SeedOptions{})
	assert.NoError(t, err)
	var repo []models.Repository
	err = tx.Limit(1).Find(&repo).Error
	assert.NoError(t, err)

	err = seeds.SeedRepositoryConfigurations(tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.NoError(t, err)

	found := models.RepositoryConfiguration{}
	err = tx.
		Preload("Repository").
		First(&found, "org_id = ?", orgID).
		Error
	require.NoError(t, err)

	// Force failure on creating duplicate
	_, err = GetRepositoryConfigDao(tx).Create(api.RepositoryRequest{
		Name:      &found.Name,
		URL:       &found.Repository.URL,
		OrgID:     &found.OrgID,
		AccountID: &found.AccountID,
	})
	assert.Error(t, err)
	if err != nil {
		daoError, ok := err.(*ce.DaoError)
		assert.True(t, ok)
		if ok {
			assert.True(t, daoError.BadValidation)
		}
	}
}

func (suite *RepositoryConfigSuite) TestRepositoryCreateBlank() {
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
		_, err := GetRepositoryConfigDao(tx).Create(blankItems[i].given)
		assert.NotNil(t, err)
		if blankItems[i].expected == "" {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
			if err != nil {
				daoError, ok := err.(*ce.DaoError)
				assert.True(t, ok)
				assert.True(t, daoError.BadValidation)
				assert.Contains(t, daoError.Message, blankItems[i].expected)
			}
		}
		tx.RollbackTo("testrepositorycreateblanktest")
	}
}

func (suite *RepositoryConfigSuite) TestBulkCreateCleanupURL() {
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()
	var repository models.Repository

	err := seeds.SeedRepository(tx, 1, seeds.SeedOptions{})
	require.NoError(t, err)
	tx.First(&repository)
	assert.NotEmpty(t, repository)
	urlNoSlash := repository.URL[0 : len(repository.URL)-1]

	// create repository without trailing slash to see that URL is cleaned up before query for repository
	request := []api.RepositoryRequest{
		{
			Name:  pointy.String("repo"),
			URL:   pointy.String(urlNoSlash),
			OrgID: pointy.String(orgID),
		},
	}

	rr, errs := GetRepositoryConfigDao(tx).BulkCreate(request)
	require.Empty(t, errs)
	assert.Equal(t, repository.URL, rr[0].URL)
}

func (suite *RepositoryConfigSuite) TestBulkCreate() {
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

	rr, errs := GetRepositoryConfigDao(tx).BulkCreate(requests)
	assert.Empty(t, errs)
	assert.Equal(t, amountToCreate, len(rr))

	for i := 0; i < amountToCreate; i++ {
		var foundRepoConfig models.RepositoryConfiguration
		err := tx.
			Where("name = ? AND org_id = ?", requests[i].Name, orgID).
			Find(&foundRepoConfig).
			Error
		assert.NoError(t, err)
		assert.NotEmpty(t, foundRepoConfig.UUID)
	}
}

func (suite *RepositoryConfigSuite) TestBulkCreateOneFails() {
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

	rr, errs := GetRepositoryConfigDao(tx).BulkCreate(requests)

	assert.NotEmpty(t, errs)
	assert.Empty(t, rr)
	assert.NotNil(t, errs[0])
	assert.Contains(t, errs[0].Error(), "Name")
	assert.Nil(t, errs[1])

	daoError, ok := errs[0].(*ce.DaoError)
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
	err := tx.Model(&models.RepositoryConfiguration{}).
		Where("repositories.url in (?)", urls).
		Where("repository_configurations.org_id = ?", orgID).
		Joins("inner join repositories on repository_configurations.repository_uuid = repositories.uuid").
		Count(&count).
		Find(&foundRepoConfig).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func (suite *RepositoryConfigSuite) TestUpdateWithSlash() {
	suite.updateTest("http://example.com/zoom/")
}

func (suite *RepositoryConfigSuite) TestUpdateNoSlash() {
	suite.updateTest("http://example.com/zoomnoslash")
}

func (suite *RepositoryConfigSuite) updateTest(url string) {
	name := "Updated"
	t := suite.T()
	var err error

	createResp, err := GetRepositoryConfigDao(suite.tx).Create(api.RepositoryRequest{
		Name:  pointy.String("NotUpdated"),
		URL:   &url,
		OrgID: pointy.String("MyGreatOrg"),
	})
	assert.Nil(t, err)

	err = GetRepositoryConfigDao(suite.tx).Update(createResp.OrgID, createResp.UUID,
		api.RepositoryRequest{
			Name: &name,
			URL:  &url,
		})
	assert.NoError(t, err)

	found := models.RepositoryConfiguration{}
	err = suite.tx.
		First(&found, "org_id = ?", createResp.OrgID).
		Error
	assert.NoError(t, err)
	assert.Equal(t, "Updated", found.Name)
}

func (suite *RepositoryConfigSuite) TestUpdateDuplicateVersions() {
	t := suite.T()

	err := seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{})
	duplicateVersions := []string{config.El7, config.El7}

	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.First(&found)
	err = GetRepositoryConfigDao(suite.tx).Update(found.OrgID, found.UUID,
		api.RepositoryRequest{
			DistributionVersions: &duplicateVersions,
		})
	assert.Nil(t, err)

	res := suite.tx.Where("uuid = ?", found.UUID).First(&found)
	assert.Nil(t, res.Error)
	assert.Equal(t, pq.StringArray{config.El7}, found.Versions)
}

func (suite *RepositoryConfigSuite) TestUpdateEmpty() {
	name := "Updated"
	arch := ""
	versions := []string{}
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
	assert.Equal(t, found.GpgKey, repoConfig.GpgKey)
	assert.Equal(t, found.MetadataVerification, repoConfig.MetadataVerification)
	assert.NotEmpty(t, found.Arch)

	// Update the RepositoryConfiguration record using dao method
	err = GetRepositoryConfigDao(tx).Update(found.OrgID, found.UUID,
		api.RepositoryRequest{
			Name:                 &name,
			DistributionArch:     &arch,
			DistributionVersions: &versions,
		})
	assert.NoError(t, err)

	// Check the updated data
	err = tx.
		First(&found, "uuid = ? AND org_id = ?", repoConfig.UUID, orgID).
		Error
	require.NoError(t, err)
	assert.Equal(t, name, found.Name)
	assert.Equal(t, found.Arch, config.ANY_ARCH)
}

func (suite *RepositoryConfigSuite) TestDuplicateUpdate() {
	t := suite.T()
	var err error
	tx := suite.tx
	name := "testduplicateupdate - repository"
	url := "https://testduplicate.com"

	repo := repoPublicTest.DeepCopy()
	repoConfig := repoConfigTest1.DeepCopy()
	var created1 api.RepositoryResponse
	var created2 api.RepositoryResponse

	created1, err = GetRepositoryConfigDao(suite.tx).
		Create(api.RepositoryRequest{
			OrgID:     &repoConfig.OrgID,
			AccountID: &repoConfig.AccountID,
			Name:      &repoConfig.Name,
			URL:       &repo.URL,
		})
	assert.NoError(t, err)

	created2, err = GetRepositoryConfigDao(suite.tx).
		Create(api.RepositoryRequest{
			OrgID:     &created1.OrgID,
			AccountID: &created1.AccountID,
			Name:      &name,
			URL:       &url})
	assert.NoError(t, err)

	err = GetRepositoryConfigDao(tx).Update(
		created2.OrgID,
		created2.UUID,
		api.RepositoryRequest{
			Name: &created1.Name,
			URL:  pointy.String("https://testduplicate2.com"),
		})
	assert.Error(t, err)

	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.BadValidation)
}

func (suite *RepositoryConfigSuite) TestUpdateNotFound() {
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

	err = GetRepositoryConfigDao(suite.tx).Update("Wrong OrgID!! zomg hacker", found.UUID,
		api.RepositoryRequest{
			Name: &name,
			URL:  &name,
		})

	require.Error(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

func (suite *RepositoryConfigSuite) TestUpdateBlank() {
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
		err := GetRepositoryConfigDao(tx).Update(orgID, found.UUID, blankItems[i].given)
		assert.Error(t, err)
		if blankItems[i].expected == "" {
			assert.NoError(t, err)
		} else {
			require.Error(t, err)
			daoError, ok := err.(*ce.DaoError)
			assert.True(t, ok)
			assert.True(t, daoError.BadValidation)
			assert.Contains(t, daoError.Message, blankItems[i].expected)
		}
		tx.RollbackTo("updateblanktest")
	}
}

func (suite *RepositoryConfigSuite) TestFetch() {
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

	fetched, err := GetRepositoryConfigDao(suite.tx).Fetch(found.OrgID, found.UUID)
	assert.Nil(t, err)
	assert.Equal(t, found.UUID, fetched.UUID)
	assert.Equal(t, found.Name, fetched.Name)
	assert.Equal(t, found.Repository.URL, fetched.URL)
}

func (suite *RepositoryConfigSuite) TestFetchByRepo() {
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

	fetched, err := GetRepositoryConfigDao(suite.tx).FetchByRepoUuid(found.OrgID, found.RepositoryUUID)
	assert.Nil(t, err)
	assert.Equal(t, found.UUID, fetched.UUID)
	assert.Equal(t, found.Name, fetched.Name)
	assert.Equal(t, found.Repository.URL, fetched.URL)
}

func (suite *RepositoryConfigSuite) TestFetchNotFound() {
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

	_, err = GetRepositoryConfigDao(suite.tx).Fetch("bad org id", found.UUID)
	assert.NotNil(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	_, err = GetRepositoryConfigDao(suite.tx).Fetch(orgID, "bad uuid")
	assert.NotNil(t, err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

func (suite *RepositoryConfigSuite) TestInternalOnly_ListRepoConfigsByUUID() {
	t := suite.T()
	numberOfRepos := 10 // Tested with up to 10,000 results

	// Create a Repository record
	repoPublic := repoPublicTest.DeepCopy()
	err := suite.tx.Create(&repoPublic).Error
	require.NoError(t, err)

	// Creat a repositoryConfig referencing above repository
	repoConfig := repoConfigTest1.DeepCopy()
	repoConfig.RepositoryUUID = repoPublic.UUID

	for i := 0; i < numberOfRepos; i++ {
		// Make 10 repoConfigs referring to the same repositoryUUID
		repoConfig.OrgID = seeds.RandomOrgId()
		err = suite.tx.Create(&repoConfig).Error
		assert.Nil(t, err)
	}

	results := GetRepositoryConfigDao(suite.tx).InternalOnly_ListRepoConfigsByUUID(repoConfig.RepositoryUUID)

	// Confirm all 10 repoConfigs are returned
	assert.Equal(t, numberOfRepos, len(results))
}

func (suite *RepositoryConfigSuite) TestList() {
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

	response, total, err := GetRepositoryConfigDao(suite.tx).List(orgID, pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 1, len(response.Data))
	if len(response.Data) > 0 {
		assert.Equal(t, repoConfig.Name, response.Data[0].Name)
		assert.Equal(t, repoConfig.Repository.URL, response.Data[0].URL)
	}
}

func (suite *RepositoryConfigSuite) TestListNoRepositories() {
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

	response, total, err := GetRepositoryConfigDao(suite.tx).List(orgID, pageData, filterData)
	assert.Nil(t, err)
	assert.Empty(t, response.Data)
	assert.Equal(t, int64(0), total)
}

func (suite *RepositoryConfigSuite) TestListPageLimit() {
	t := suite.T()
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	orgID := seeds.RandomOrgId()
	pageData := api.PaginationData{
		Limit:  10,
		Offset: 0,
		SortBy: "name",
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

	response, total, err := GetRepositoryConfigDao(suite.tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, len(response.Data), pageData.Limit)
	assert.Equal(t, int64(20), total)

	// Asserts that the first item is lower (alphabetically a-z) than the last item.
	firstItem := strings.ToLower(response.Data[0].Name)
	lastItem := strings.ToLower(response.Data[len(response.Data)-1].Name)
	assert.Equal(t, -1, strings.Compare(firstItem, lastItem))
}

func (suite *RepositoryConfigSuite) TestListFilterName() {
	t := suite.T()
	orgID := seeds.RandomOrgId()
	filterData := api.FilterData{}

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, 2, seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El9}}))
	allRepoResp, _, err := GetRepositoryConfigDao(suite.tx).List(orgID, api.PaginationData{}, api.FilterData{})
	assert.NoError(t, err)
	filterData.Name = allRepoResp.Data[0].Name

	response, total, err := GetRepositoryConfigDao(suite.tx).List(orgID, api.PaginationData{}, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, 1, int(total))

	assert.Equal(t, filterData.Name, response.Data[0].Name)
}

func (suite *RepositoryConfigSuite) TestListFilterUrl() {
	t := suite.T()
	orgID := seeds.RandomOrgId()
	filterData := api.FilterData{}

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, 2, seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El9}}))
	allRepoResp, _, err := GetRepositoryConfigDao(suite.tx).List(orgID, api.PaginationData{}, api.FilterData{})
	assert.NoError(t, err)
	filterData.URL = allRepoResp.Data[0].URL

	response, total, err := GetRepositoryConfigDao(suite.tx).List(orgID, api.PaginationData{}, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, 1, int(total))

	assert.Equal(t, filterData.URL, response.Data[0].URL)

	// Test that it works with urls missing a trailing slash
	filterData.URL = filterData.URL[:len(filterData.URL)-1]
	response, total, err = GetRepositoryConfigDao(suite.tx).List(orgID, api.PaginationData{}, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, 1, int(total))
}

func (suite *RepositoryConfigSuite) TestListFilterVersion() {
	t := suite.T()

	orgID := seeds.RandomOrgId()
	pageData := api.PaginationData{
		Limit:  20,
		Offset: 0,
		SortBy: "name:desc",
	}
	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: config.El9,
	}

	var total int64
	quantity := 20

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, quantity, seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El9}}))
	response, total, err := GetRepositoryConfigDao(suite.tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, quantity, int(total))

	// Asserts that list is sorted by name z-a
	firstItem := strings.ToLower(response.Data[0].Name)
	lastItem := strings.ToLower(response.Data[len(response.Data)-1].Name)
	assert.True(t, firstItem > lastItem)
}

func (suite *RepositoryConfigSuite) TestListFilterArch() {
	t := suite.T()
	tx := suite.tx
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	orgID := seeds.RandomOrgId()
	pageData := api.PaginationData{
		Limit:  20,
		Offset: 0,
		SortBy: "url",
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

	response, total, err := GetRepositoryConfigDao(tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(quantity), total)

	// Asserts that list is sorted by url a-z
	firstItem := strings.ToLower(response.Data[0].URL)
	lastItem := strings.ToLower(response.Data[len(response.Data)-1].URL)
	assert.True(t, firstItem < lastItem)
}

func (suite *RepositoryConfigSuite) TestListFilterStatus() {
	t := suite.T()
	orgID := seeds.RandomOrgId()
	pageData := api.PaginationData{
		Limit:  30,
		Offset: 0,
		SortBy: "status",
	}

	filterData := api.FilterData{
		Search: "",
		Status: config.StatusValid + "," + config.StatusPending,
	}

	quantity := 30

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, quantity/3,
		seeds.SeedOptions{OrgID: orgID, Status: pointy.String(config.StatusValid)}))

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, quantity/3,
		seeds.SeedOptions{OrgID: orgID, Status: pointy.String(config.StatusInvalid)}))

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, quantity/3,
		seeds.SeedOptions{OrgID: orgID, Status: pointy.String(config.StatusPending)}))

	response, count, err := GetRepositoryConfigDao(suite.tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, 20, len(response.Data))
	assert.Equal(t, int64(20), count)

	// Asserts that list is sorted by status a-z
	firstItem := strings.ToLower(response.Data[0].Status)
	lastItem := strings.ToLower(response.Data[len(response.Data)-1].Status)
	assert.True(t, firstItem < lastItem)
}

func (suite *RepositoryConfigSuite) TestListFilterMultipleArch() {
	t := suite.T()
	orgID := seeds.RandomOrgId()
	pageData := api.PaginationData{
		Limit:  20,
		Offset: 0,
		SortBy: "distribution_arch",
	}

	filterData := api.FilterData{
		Search:  "",
		Arch:    "x86_64,s390x",
		Version: "",
	}

	quantity := 20

	x86ref := "x86_64"
	s390xref := "s390x"

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, 10, seeds.SeedOptions{OrgID: orgID, Arch: &s390xref}))
	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, 30, seeds.SeedOptions{OrgID: orgID, Arch: &x86ref}))

	response, count, err := GetRepositoryConfigDao(suite.tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(40), count)

	// By setting SortBy to "arch asc" we now expect the first page arches to be half and half s390x/x86_64
	firstItem := response.Data[0].DistributionArch
	lastItem := response.Data[len(response.Data)-1].DistributionArch

	assert.Equal(t, firstItem, "s390x")
	assert.Equal(t, lastItem, "x86_64")
}

func (suite *RepositoryConfigSuite) TestListFilterMultipleVersions() {
	t := suite.T()
	orgID := seeds.RandomOrgId()
	pageData := api.PaginationData{
		Limit:  20,
		Offset: 0,
		SortBy: "distribution_versions",
	}

	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: config.El7 + "," + config.El9,
	}

	quantity := 20

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, quantity/2,
		seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El7, config.El8, config.El9}}))

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, quantity/2,
		seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El7}}))

	// Seed data to a 2nd org to verify no crossover
	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, quantity,
		seeds.SeedOptions{OrgID: "kdksfkdf", Versions: &[]string{config.El7, config.El8, config.El9}}))

	response, count, err := GetRepositoryConfigDao(suite.tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(quantity), count)

	// By setting the above seed values and SortBy to "version asc", we expect the first page to contain 10 versions of length 1 and 10 versions of length 3
	firstItem := len(response.Data[0].DistributionVersions)
	lastItem := len(response.Data[len(response.Data)-1].DistributionVersions)

	assert.Equal(t, firstItem, 1)
	assert.Equal(t, lastItem, 3)
}

func (suite *RepositoryConfigSuite) TestListFilterSearch() {
	t := suite.T()
	tx := suite.tx
	repoConfigs := make([]models.RepositoryConfiguration, 0)
	orgID := seeds.RandomOrgId()
	accountID := seeds.RandomAccountId()
	name := "my repo"
	url := "http://testsearchfilter.example.com"
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

	_, err := GetRepositoryConfigDao(tx).Create(api.RepositoryRequest{
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

	response, total, err := GetRepositoryConfigDao(tx).List(orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, int(quantity), len(response.Data))
	assert.Equal(t, quantity, total)
}

func (suite *RepositoryConfigSuite) TestSavePublicUrls() {
	t := suite.T()
	tx := suite.tx
	var count int64
	repoUrls := []string{
		"https://somepublicRepo.example.com/",
		"https://anotherpublicRepo.example.com/",
	}

	// Create the two Repository records
	err := GetRepositoryConfigDao(tx).SavePublicRepos(repoUrls)
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
	err = GetRepositoryConfigDao(suite.tx).SavePublicRepos(repoUrls)
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

func (suite *RepositoryConfigSuite) TestDelete() {
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

	err = GetRepositoryConfigDao(tx).Delete(repoConfig.OrgID, repoConfig.UUID)
	assert.NoError(t, err)

	repoConfig2 := models.RepositoryConfiguration{}
	err = tx.
		First(&repoConfig2, "org_id = ? AND uuid = ?", repoConfig.OrgID, repoConfig.UUID).
		Error
	require.Error(t, err)
	assert.Equal(t, "record not found", err.Error())
}

func (suite *RepositoryConfigSuite) TestDeleteNotFound() {
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

	err = GetRepositoryConfigDao(suite.tx).Delete("bad org id", found.UUID)
	assert.Error(t, err)
	daoError, ok := err.(*ce.DaoError)
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

func (suite *RepositoryConfigSuite) TestValidateParameters() {
	t := suite.T()
	mockYumRepo, dao, repoConfig := suite.setupValidationTest()

	// Duplicated name and url
	parameters := api.RepositoryValidationRequest{
		Name: &repoConfig.Name,
		URL:  &repoConfig.Repository.URL,
		UUID: &repoConfig.UUID,
	}

	mockYumRepo.Mock.On("Repomd").Return(&yum.Repomd{}, 200, nil)
	mockYumRepo.Mock.On("Signature").Return(test.RepomdSignature(), 200, nil)
	response, err := dao.ValidateParameters(repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)

	assert.False(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.Contains(t, response.Name.Error, "already exists.")
	assert.False(t, response.URL.Valid)
	assert.False(t, response.URL.Skipped)
	assert.Contains(t, response.URL.Error, "already exists.")

	// Test again with an edit
	mockYumRepo.Mock.On("Repomd").Return(&yum.Repomd{}, 200, nil)
	mockYumRepo.Mock.On("Signature").Return(test.RepomdSignature(), 200, nil)
	response, err = dao.ValidateParameters(repoConfig.OrgID, parameters, []string{*parameters.UUID})
	assert.NoError(t, err)

	assert.True(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.True(t, response.URL.Valid)
	assert.True(t, response.URL.MetadataPresent)
	assert.False(t, response.URL.Skipped)
}

func (suite *RepositoryConfigSuite) TestValidateParametersNoNameUrl() {
	t := suite.T()
	_, dao, repoConfig := suite.setupValidationTest()

	// Not providing any name or url
	parameters := api.RepositoryValidationRequest{
		Name: nil,
		URL:  nil,
	}
	response, err := dao.ValidateParameters(repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)

	assert.False(t, response.Name.Valid)
	assert.True(t, response.Name.Skipped)
	assert.False(t, response.URL.Valid)
	assert.True(t, response.URL.Skipped)
}

func (suite *RepositoryConfigSuite) TestValidateParametersBlankValues() {
	t := suite.T()
	_, dao, repoConfig := suite.setupValidationTest()

	// Blank values
	parameters := api.RepositoryValidationRequest{
		Name: pointy.String(""),
		URL:  pointy.String(""),
	}
	response, err := dao.ValidateParameters(repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)

	assert.False(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.Contains(t, response.Name.Error, "blank")
	assert.False(t, response.URL.Valid)
	assert.False(t, response.URL.Skipped)
	assert.Contains(t, response.URL.Error, "blank")
}

func (suite *RepositoryConfigSuite) TestValidateParametersValidUrlName() {
	t := suite.T()
	mockYumRepo, dao, repoConfig := suite.setupValidationTest()
	// Providing a valid url & name
	parameters := api.RepositoryValidationRequest{
		Name: pointy.String("Some Other Name"),
		URL:  pointy.String("http://example.com/"),
	}
	mockYumRepo.Mock.On("Repomd").Return(&yum.Repomd{}, 200, nil)
	mockYumRepo.Mock.On("Signature").Return(test.RepomdSignature(), 200, nil)

	response, err := dao.ValidateParameters(repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)

	assert.True(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.True(t, response.URL.Valid)
	assert.True(t, response.URL.MetadataPresent)
	assert.False(t, response.URL.Skipped)
}

func (suite *RepositoryConfigSuite) TestValidateParametersBadUrl() {
	t := suite.T()
	mockYumRepo, dao, repoConfig := suite.setupValidationTest()
	// Providing a bad url that doesn't have a repo
	parameters := api.RepositoryValidationRequest{
		Name: pointy.String("Some bad repo!"),
		URL:  pointy.String("http://badrepo.example.com/"),
	}
	mockYumRepo.Mock.On("Repomd").Return(nil, 404, nil)

	response, err := dao.ValidateParameters(repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)

	assert.True(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.True(t, response.URL.Valid) // Even if the metadata isn't present, the URL itself is valid
	assert.Equal(t, response.URL.HTTPCode, 404)
	assert.False(t, response.URL.MetadataPresent)
	assert.False(t, response.URL.Skipped)
}

func (suite *RepositoryConfigSuite) TestValidateParametersTimeOutUrl() {
	t := suite.T()
	mockYumRepo, dao, repoConfig := suite.setupValidationTest()
	// Providing a timed out url
	parameters := api.RepositoryValidationRequest{
		Name: pointy.String("Some Timeout repo!"),
		URL:  pointy.String("http://timeout.example.com"),
	}

	timeoutErr := MockTimeoutError{
		Message: " (Client.Timeout exceeded while awaiting headers)",
		Timeout: true,
	}

	mockYumRepo.Mock.On("Repomd").Return(nil, 0, timeoutErr)

	response, err := dao.ValidateParameters(repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)

	assert.True(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.True(t, response.URL.Valid)
	assert.Equal(t, response.URL.HTTPCode, 0)
	assert.False(t, response.URL.MetadataPresent)
	assert.Contains(t, response.URL.Error, "Timeout")
	assert.False(t, response.URL.Skipped)
}
func (suite *RepositoryConfigSuite) TestValidateParametersGpgKey() {
	t := suite.T()
	mockYumRepo, dao, repoConfig := suite.setupValidationTest()
	// Providing a timed out url
	parameters := api.RepositoryValidationRequest{
		Name:                 pointy.String("Good Gpg"),
		URL:                  pointy.String("http://goodgpg.example.com/"),
		GPGKey:               test.GpgKey(),
		MetadataVerification: true,
	}

	mockYumRepo.Mock.On("Repomd").Return(test.Repomd, 200, nil)
	mockYumRepo.Mock.On("Signature").Return(test.RepomdSignature(), 200, nil)

	response, err := dao.ValidateParameters(repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)
	assert.True(t, response.GPGKey.Valid)
	assert.Equal(t, "", response.GPGKey.Error)
	assert.True(t, response.URL.MetadataSignaturePresent)
	assert.True(t, response.URL.Valid)
}

func (suite *RepositoryConfigSuite) TestValidateParametersBadSig() {
	t := suite.T()
	mockYumRepo, dao, repoConfig := suite.setupValidationTest()
	parameters := api.RepositoryValidationRequest{
		Name:                 pointy.String("Good Gpg"),
		URL:                  pointy.String("http://badsig.example.com/"),
		GPGKey:               test.GpgKey(),
		MetadataVerification: true,
	}

	badRepomdXml := *test.Repomd.RepomdString + "<BadXML>"
	badRepomd := yum.Repomd{RepomdString: &badRepomdXml}
	mockYumRepo.Mock.On("Repomd").Return(&badRepomd, 200, nil)
	mockYumRepo.Mock.On("Signature").Return(test.RepomdSignature(), 200, nil)

	response, err := dao.ValidateParameters(repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)
	assert.False(t, response.GPGKey.Valid)
	assert.True(t, response.URL.MetadataSignaturePresent)
	assert.True(t, response.URL.Valid)

	// retest disabling metadata verification
	parameters.MetadataVerification = false
	response, err = dao.ValidateParameters(repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)
	assert.True(t, response.GPGKey.Valid)
	assert.True(t, response.URL.MetadataSignaturePresent)
	assert.True(t, response.URL.Valid)
}

func (suite *RepositoryConfigSuite) TestValidateParametersBadGpgKey() {
	t := suite.T()
	mockYumRepo, dao, repoConfig := suite.setupValidationTest()
	// Providing a timed out url
	parameters := api.RepositoryValidationRequest{
		Name:                 pointy.String("Good Gpg"),
		URL:                  pointy.String("http://badsig.example.com/"),
		GPGKey:               pointy.String("Not a real key"),
		MetadataVerification: true,
	}

	mockYumRepo.Mock.On("Repomd").Return(test.Repomd, 200, nil)
	mockYumRepo.Mock.On("Signature").Return(test.RepomdSignature(), 200, nil)

	response, err := dao.ValidateParameters(repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)
	assert.False(t, response.GPGKey.Valid)
	assert.True(t, response.URL.MetadataSignaturePresent)
	assert.True(t, response.URL.Valid)
}

func TestDBErrorToApi(t *testing.T) {
	var result *ce.DaoError

	type TestCase struct {
		Name     string
		Given    error
		Expected *ce.DaoError
	}

	testCases := []TestCase{
		{
			Name:     "nil return nil",
			Given:    nil,
			Expected: nil,
		},
		{
			Name:     "A models.Error",
			Given:    models.Error{Message: "error model", Validation: false},
			Expected: &ce.DaoError{BadValidation: false, Message: "error model"},
		},
		{
			Name:     "pgconn.PgError Code = 23505, ConstraintName = repo_and_org_id_unique",
			Given:    &pgconn.PgError{Code: "23505", ConstraintName: "repo_and_org_id_unique"},
			Expected: &ce.DaoError{BadValidation: true, Message: "Repository with this URL already belongs to organization"},
		},
		{
			Name:     "pgconn.PgError Code = 23505, ConstraintName = repositories_unique_url",
			Given:    &pgconn.PgError{Code: "23505", ConstraintName: "repositories_unique_url"},
			Expected: &ce.DaoError{BadValidation: true, Message: "Repository with this URL already belongs to organization"},
		},
		{
			Name:     "pgconn.PgError Code = 23505, ConstraintName = name_and_org_id_unique",
			Given:    &pgconn.PgError{Code: "23505", ConstraintName: "name_and_org_id_unique"},
			Expected: &ce.DaoError{BadValidation: true, Message: "Repository with this name already belongs to organization"},
		},
		{
			Name:     "Undefined error",
			Given:    fmt.Errorf("undefined error"),
			Expected: &ce.DaoError{BadValidation: false, Message: "undefined error"},
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		result = DBErrorToApi(testCase.Given)
		assert.Equal(t, testCase.Expected, result)
	}
}

func (suite *RepositoryConfigSuite) setupValidationTest() (*mockExt.YumRepositoryMock, repositoryConfigDaoImpl, models.RepositoryConfiguration) {
	t := suite.T()
	orgId := seeds.RandomOrgId()
	err := seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgId})
	assert.NoError(t, err)

	mockYumRepo := mockExt.YumRepositoryMock{}
	dao := repositoryConfigDaoImpl{
		db:      suite.tx,
		yumRepo: &mockYumRepo,
	}

	repoConfig := models.RepositoryConfiguration{}
	err = suite.tx.
		Preload("Repository").
		First(&repoConfig, "org_id = ?", orgId).
		Error
	require.NoError(t, err)
	return &mockYumRepo, dao, repoConfig
}
