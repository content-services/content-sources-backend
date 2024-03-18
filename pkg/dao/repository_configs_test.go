package dao

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/test"
	mockExt "github.com/content-services/content-sources-backend/pkg/test/mocks/mock_external"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type RepositoryConfigSuite struct {
	*DaoSuite
	mockPulpClient *pulp_client.MockPulpClient
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
	r := RepositoryConfigSuite{DaoSuite: &m, mockPulpClient: pulp_client.NewMockPulpClient(t)}
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
	moduleHotfixes := true
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
		ModuleHotfixes:       &moduleHotfixes,
	}

	dao := GetRepositoryConfigDao(tx, suite.mockPulpClient)
	created, err := dao.Create(context.Background(), toCreate)
	assert.Nil(t, err)

	foundRepo, err := dao.Fetch(context.Background(), orgID, created.UUID)
	assert.Nil(t, err)
	assert.Equal(t, url, foundRepo.URL)
	assert.Equal(t, true, foundRepo.ModuleHotfixes)
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
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	_, err := dao.Create(context.Background(), toCreate)
	assert.ErrorContains(suite.T(), err, "Invalid URL for request.")

	dao = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	_, err = dao.Create(context.Background(), toCreate)
	assert.ErrorContains(suite.T(), err, "Invalid URL for request.")
}

func (suite *RepositoryConfigSuite) TestCreateRedHatRepository() {
	toCreate := api.RepositoryRequest{
		Name:             pointy.String(""),
		URL:              pointy.String("something-no-slash"),
		OrgID:            pointy.String(config.RedHatOrg),
		AccountID:        pointy.String("123"),
		DistributionArch: pointy.String(""),
		DistributionVersions: &[]string{
			config.El9,
		},
	}
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	_, err := dao.Create(context.Background(), toCreate)
	assert.ErrorContains(suite.T(), err, "Creating of Red Hat repositories is not permitted")
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
	tx.SavePoint("before")
	_, err = GetRepositoryConfigDao(tx, suite.mockPulpClient).Create(context.Background(), api.RepositoryRequest{
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
			assert.Contains(t, err.Error(), "name")
		}
	}
	tx.RollbackTo("before")

	// Force failure on creating duplicate url
	_, err = GetRepositoryConfigDao(tx, suite.mockPulpClient).Create(context.Background(), api.RepositoryRequest{
		Name:      pointy.Pointer("new name"),
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
			assert.Contains(t, err.Error(), "URL")
		}
	}
}

func (suite *RepositoryConfigSuite) TestRepositoryUrlInvalid() {
	t := suite.T()
	tx := suite.tx

	invalidURL := "hey/there!"
	invalidURL2 := "golang.org"
	name := "name"
	OrgID := seeds.RandomOrgId()

	type testCases struct {
		given    api.RepositoryRequest
		expected string
	}
	invalidItems := []testCases{
		{
			given: api.RepositoryRequest{
				Name:  &name,
				URL:   &invalidURL,
				OrgID: &OrgID,
			},
			expected: "Invalid URL for request.",
		},
		{
			given: api.RepositoryRequest{
				Name:  &name,
				URL:   &invalidURL2,
				OrgID: &OrgID,
			},
			expected: "Invalid URL for request",
		},
	}
	tx.SavePoint("testrepositorycreateinvalidtest")
	for i := 0; i < len(invalidItems); i++ {
		_, err := GetRepositoryConfigDao(tx, suite.mockPulpClient).Create(context.Background(), invalidItems[i].given)
		assert.NotNil(t, err)
		if invalidItems[i].expected == "" {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
			if err != nil {
				daoError, ok := err.(*ce.DaoError)
				assert.True(t, ok)
				assert.True(t, daoError.BadValidation)
				assert.Contains(t, daoError.Message, invalidItems[i].expected)
			}
		}
		tx.RollbackTo("testrepositorycreateinvalidtest")
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
		_, err := GetRepositoryConfigDao(tx, suite.mockPulpClient).Create(context.Background(), blankItems[i].given)
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

	rr, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient).BulkCreate(context.Background(), request)
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
			Name:           &name,
			URL:            &url,
			OrgID:          &orgID,
			ModuleHotfixes: pointy.Pointer(i%3 == 0),
		}
	}

	rr, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient).BulkCreate(context.Background(), requests)
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
		assert.Equal(t, i%3 == 0, foundRepoConfig.ModuleHotfixes)
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
			URL:       pointy.String("https://repo_2_url.org"),
			OrgID:     &orgID,
			AccountID: &accountID,
		},
		{
			Name:      pointy.String("repo_1"),
			URL:       pointy.String("https://repo_1_url.org"),
			OrgID:     &orgID,
			AccountID: &accountID,
		},
	}

	rr, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient).BulkCreate(context.Background(), requests)

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

	createResp, err := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).Create(context.Background(), api.RepositoryRequest{
		Name:  pointy.String("NotUpdated"),
		URL:   &url,
		OrgID: pointy.String("MyGreatOrg"),
	})
	assert.Nil(t, err)

	_, err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).Update(context.Background(), createResp.OrgID, createResp.UUID,
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

func (suite *RepositoryConfigSuite) TestUpdateAttributes() {
	t := suite.T()
	var err error

	createResp, err := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).Create(context.Background(), api.RepositoryRequest{
		Name:                 pointy.Pointer("NotUpdated"),
		URL:                  pointy.Pointer("http://example.com/testupdateattributes"),
		OrgID:                pointy.Pointer("MyGreatOrg"),
		ModuleHotfixes:       pointy.Pointer(false),
		MetadataVerification: pointy.Pointer(false),
	})
	assert.Nil(t, err)

	_, err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).Update(context.Background(), createResp.OrgID, createResp.UUID,
		api.RepositoryRequest{
			ModuleHotfixes:       pointy.Pointer(true),
			MetadataVerification: pointy.Pointer(true),
		})
	assert.NoError(t, err)

	found := models.RepositoryConfiguration{}
	err = suite.tx.
		First(&found, "org_id = ?", createResp.OrgID).
		Error
	assert.NoError(t, err)
	assert.True(t, found.ModuleHotfixes)
	assert.True(t, found.MetadataVerification)
}

func (suite *RepositoryConfigSuite) TestUpdateDuplicateVersions() {
	t := suite.T()

	err := seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{})
	duplicateVersions := []string{config.El7, config.El7}

	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.First(&found)
	_, err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).Update(context.Background(), found.OrgID, found.UUID,
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
	_, err = GetRepositoryConfigDao(tx, suite.mockPulpClient).Update(context.Background(), found.OrgID, found.UUID,
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

	created1, err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).
		Create(context.Background(), api.RepositoryRequest{
			OrgID:     &repoConfig.OrgID,
			AccountID: &repoConfig.AccountID,
			Name:      &repoConfig.Name,
			URL:       &repo.URL,
		})
	assert.NoError(t, err)

	created2, err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).
		Create(context.Background(), api.RepositoryRequest{
			OrgID:     &created1.OrgID,
			AccountID: &created1.AccountID,
			Name:      &name,
			URL:       &url})
	assert.NoError(t, err)

	_, err = GetRepositoryConfigDao(tx, suite.mockPulpClient).Update(
		context.Background(),
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

	_, err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).Update(context.Background(), "Wrong OrgID!! zomg hacker", found.UUID,
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
		_, err := GetRepositoryConfigDao(tx, suite.mockPulpClient).Update(context.Background(), orgID, found.UUID, blankItems[i].given)
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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)

	snap := models.Snapshot{
		Base:                        models.Base{UUID: uuid.NewString()},
		VersionHref:                 "/pulp/version",
		PublicationHref:             "/pulp/publication",
		DistributionPath:            fmt.Sprintf("/path/to/%v", uuid.NewString()),
		RepositoryConfigurationUUID: found.UUID,
		ContentCounts:               models.ContentCountsType{"rpm.package": int64(3), "rpm.advisory": int64(1)},
		AddedCounts:                 models.ContentCountsType{"rpm.package": int64(1), "rpm.advisory": int64(3)},
		RemovedCounts:               models.ContentCountsType{"rpm.package": int64(2), "rpm.advisory": int64(2)},
	}
	sDao := snapshotDaoImpl{db: tx}
	err = sDao.Create(context.Background(), &snap)
	assert.NoError(t, err)

	err = tx.
		Preload("Repository").Preload("LastSnapshot").
		First(&found, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)

	if config.Get().Features.Snapshots.Enabled {
		suite.mockPulpForListOrFetch(1)
	}

	fetched, err := repoConfigDao.Fetch(context.Background(), found.OrgID, found.UUID)
	assert.Nil(t, err)
	assert.Equal(t, found.UUID, fetched.UUID)
	assert.Equal(t, found.Name, fetched.Name)
	assert.Equal(t, found.Repository.URL, fetched.URL)
	assert.Equal(t, found.LastSnapshot.UUID, fetched.LastSnapshot.UUID)

	if config.Get().Features.Snapshots.Enabled {
		assert.Equal(t, testContentPath+"/", fetched.LastSnapshot.URL)
	}
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

	fetched, err := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).FetchByRepoUuid(context.Background(), found.OrgID, found.RepositoryUUID)
	assert.Nil(t, err)
	assert.Equal(t, found.UUID, fetched.UUID)
	assert.Equal(t, found.Name, fetched.Name)
	assert.Equal(t, found.Repository.URL, fetched.URL)
}

func (suite *RepositoryConfigSuite) TestFetchWithoutOrgID() {
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	err = tx.
		First(&found, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)

	fetched, err := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).FetchWithoutOrgID(context.Background(), found.UUID)
	assert.Nil(t, err)
	assert.Equal(t, found.UUID, fetched.UUID)
	assert.Equal(t, found.Name, fetched.Name)
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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)

	_, err = repoConfigDao.Fetch(context.Background(), "bad org id", found.UUID)
	assert.NotNil(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	_, err = repoConfigDao.Fetch(context.Background(), orgID, "bad uuid")
	assert.NotNil(t, err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

func (suite *RepositoryConfigSuite) TestInternalOnly_FetchRepoConfigsForRepoUUID() {
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

	results := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).InternalOnly_FetchRepoConfigsForRepoUUID(context.Background(), repoConfig.RepositoryUUID)

	// Confirm all 10 repoConfigs are returned
	assert.Equal(t, numberOfRepos, len(results))
	// Ensure that the url and Name are successfully returned (both required) for notifications
	assert.NotEmpty(t, results[0].URL)
	assert.NotEmpty(t, results[0].Name)
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

	snap := models.Snapshot{
		Base:                        models.Base{UUID: uuid.NewString()},
		VersionHref:                 "/pulp/version",
		PublicationHref:             "/pulp/publication",
		DistributionPath:            fmt.Sprintf("/path/to/%v", uuid.NewString()),
		RepositoryConfigurationUUID: repoConfig.UUID,
		ContentCounts:               models.ContentCountsType{"rpm.package": int64(3), "rpm.advisory": int64(1)},
		AddedCounts:                 models.ContentCountsType{"rpm.package": int64(1), "rpm.advisory": int64(3)},
		RemovedCounts:               models.ContentCountsType{"rpm.package": int64(2), "rpm.advisory": int64(2)},
	}
	sDao := snapshotDaoImpl{db: suite.tx}
	err = sDao.Create(context.Background(), &snap)
	assert.NoError(t, err)

	err = suite.tx.
		Preload("Repository").Preload("LastSnapshot").
		First(&repoConfig, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)

	rDao := repositoryConfigDaoImpl{db: suite.tx, pulpClient: suite.mockPulpClient}
	if config.Get().Features.Snapshots.Enabled {
		suite.mockPulpForListOrFetch(1)
	}

	response, total, err := rDao.List(context.Background(), orgID, pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 1, len(response.Data))
	if len(response.Data) > 0 {
		assert.Equal(t, repoConfig.Name, response.Data[0].Name)
		assert.Equal(t, repoConfig.Repository.URL, response.Data[0].URL)
		assert.Equal(t, repoConfig.LastSnapshot.UUID, response.Data[0].LastSnapshot.UUID)
		assert.Equal(t, repoConfig.LastSnapshot.RepositoryPath, response.Data[0].LastSnapshot.RepositoryPath)
		if config.Get().Features.Snapshots.Enabled {
			assert.Equal(t, testContentPath+"/", response.Data[0].LastSnapshot.URL)
		}
	}
}

func (suite *RepositoryConfigSuite) TestListPageDataLimit0() {
	t := suite.T()
	repoConfig := models.RepositoryConfiguration{}
	orgID := seeds.RandomOrgId()
	var total int64
	pageData := api.PaginationData{
		// Limit:  0, << defaults to 0
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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	suite.mockPulpForListOrFetch(1)
	response, total, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 0, len(response.Data)) // We have limited the data to 0, so response.data will return 0
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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	suite.mockPulpForListOrFetch(1)

	response, total, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)
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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	suite.mockPulpForListOrFetch(1)

	response, total, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)

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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	suite.mockPulpForListOrFetch(1)
	suite.mockPulpForListOrFetch(1)

	allRepoResp, _, err := repoConfigDao.List(context.Background(), orgID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(t, err)
	filterData.Name = allRepoResp.Data[0].Name

	response, total, err := repoConfigDao.List(context.Background(), orgID, api.PaginationData{Limit: -1}, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, 1, int(total))

	assert.Equal(t, filterData.Name, response.Data[0].Name)
}

func (suite *RepositoryConfigSuite) TestListFilterUrl() {
	t := suite.T()
	orgID := seeds.RandomOrgId()

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	suite.mockPulpForListOrFetch(4)

	filterData := api.FilterData{}

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, 3, seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El9}}))
	allRepoResp, _, err := repoConfigDao.List(context.Background(), orgID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(t, err)
	filterData.URL = allRepoResp.Data[0].URL

	response, total, err := repoConfigDao.List(context.Background(), orgID, api.PaginationData{Limit: -1}, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, 1, int(total))

	assert.Equal(t, filterData.URL, response.Data[0].URL)

	filterData.URL = allRepoResp.Data[0].URL + "," + allRepoResp.Data[1].URL

	response, total, err = repoConfigDao.List(context.Background(), orgID, api.PaginationData{Limit: -1}, filterData)

	assert.Nil(t, err)
	assert.Equal(t, 2, len(response.Data))
	assert.Equal(t, 2, int(total))

	assert.Equal(t, filterData.URL, response.Data[0].URL+","+response.Data[1].URL)

	// Test that it works with urls missing a trailing slash
	filterData.URL = filterData.URL[:len(filterData.URL)-1]
	response, total, err = repoConfigDao.List(context.Background(), orgID, api.PaginationData{Limit: -1}, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(response.Data))
	assert.Equal(t, 2, int(total))
}

func (suite *RepositoryConfigSuite) TestListFilterUUIDs() {
	t := suite.T()
	orgID := seeds.RandomOrgId()

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	suite.mockPulpForListOrFetch(3)

	filterData := api.FilterData{}

	assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, 3, seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El9}}))
	allRepoResp, _, err := repoConfigDao.List(context.Background(), orgID, api.PaginationData{Limit: -1}, api.FilterData{})
	assert.NoError(t, err)
	filterData.UUID = allRepoResp.Data[0].UUID

	// Test 1
	response, total, err := repoConfigDao.List(context.Background(), orgID, api.PaginationData{Limit: -1}, filterData)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, 1, int(total))
	assert.Equal(t, filterData.UUID, response.Data[0].UUID)

	filterData.UUID = allRepoResp.Data[0].UUID + "," + allRepoResp.Data[1].UUID

	response, total, err = repoConfigDao.List(context.Background(), orgID, api.PaginationData{Limit: -1}, filterData)

	assert.Nil(t, err)
	assert.Equal(t, 2, len(response.Data))
	assert.Equal(t, 2, int(total))

	assert.Equal(t, filterData.UUID, response.Data[0].UUID+","+response.Data[1].UUID)
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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	suite.mockPulpForListOrFetch(1)

	response, total, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)

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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	suite.mockPulpForListOrFetch(1)

	response, total, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(quantity), total)

	// Asserts that list is sorted by url a-z
	firstItem := strings.ToLower(response.Data[0].URL)
	lastItem := strings.ToLower(response.Data[len(response.Data)-1].URL)
	assert.True(t, firstItem < lastItem)
}

func (suite *RepositoryConfigSuite) TestListFilterOrigin() {
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
		Origin: config.OriginExternal,
	}

	var total int64

	quantity := 20
	err := seeds.SeedRepositoryConfigurations(tx, quantity, seeds.SeedOptions{OrgID: orgID, Origin: &filterData.Origin})
	assert.Nil(t, err)
	err = seeds.SeedRepositoryConfigurations(tx, quantity, seeds.SeedOptions{OrgID: orgID, Origin: pointy.Pointer("SomeOther")})
	assert.Nil(t, err)

	result := tx.Joins("inner join repositories on repositories.uuid = repository_configurations.repository_uuid").
		Where("org_id = ? AND repositories.origin = ?", orgID, filterData.Origin).
		Find(&repoConfigs).
		Count(&total)

	assert.Nil(t, result.Error)
	assert.Equal(t, int64(quantity), total)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	suite.mockPulpForListOrFetch(2)

	response, total, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(quantity), total)

	filterData.Origin = fmt.Sprintf("%v,%v", config.OriginExternal, "notarealorigin")
	response, total, err = repoConfigDao.List(context.Background(), orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(quantity), total)
}

func (suite *RepositoryConfigSuite) TestListFilterContentType() {
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()
	pageData := api.PaginationData{
		Limit:  20,
		Offset: 0,
		SortBy: "url",
	}

	filterData := api.FilterData{
		ContentType: config.ContentTypeRpm,
	}

	var total int64

	quantity := 20
	err := seeds.SeedRepositoryConfigurations(tx, quantity, seeds.SeedOptions{OrgID: orgID, ContentType: &filterData.ContentType})
	assert.Nil(t, err)
	err = seeds.SeedRepositoryConfigurations(tx, quantity, seeds.SeedOptions{OrgID: orgID, ContentType: pointy.Pointer("SomeOther")})
	assert.Nil(t, err)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)

	suite.mockPulpForListOrFetch(1)
	response, total, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(quantity), total)
}

func (suite *RepositoryConfigSuite) TestListFilterStatus() {
	t := suite.T()
	orgID := seeds.RandomOrgId()
	pageData := api.PaginationData{
		Limit:  40,
		Offset: 0,
		SortBy: "last_introspection_status",
	}

	filterData := api.FilterData{
		Search: "",
		Status: config.StatusValid + "," + config.StatusPending,
	}

	statuses := [4]string{
		config.StatusValid,
		config.StatusPending,
		config.StatusUnavailable,
		config.StatusInvalid,
	}

	quantity := 40

	_, err := seeds.SeedTasks(suite.tx, 40, seeds.TaskSeedOptions{
		OrgID: orgID, Typename: "snapshot", Status: config.TaskStatusCompleted,
	})
	assert.Nil(t, err)

	tasks := []models.TaskInfo{}
	result := suite.tx.
		Where("org_id = ?", orgID).
		Find(&tasks)
	assert.Nil(t, result.Error)

	for i := 0; i < 4; i++ {
		assert.Nil(t, seeds.SeedRepositoryConfigurations(suite.tx, quantity/4,
			seeds.SeedOptions{OrgID: orgID, Status: &statuses[i], TaskID: tasks[i].Id.String()}))
	}

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)

	suite.mockPulpForListOrFetch(1)
	response, count, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, 20, len(response.Data))
	assert.Equal(t, int64(20), count)

	// Asserts that list is sorted by last_introspection_status a-z
	firstItem := strings.ToLower(response.Data[0].LastIntrospectionStatus)
	lastItem := strings.ToLower(response.Data[len(response.Data)-1].LastIntrospectionStatus)
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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)

	suite.mockPulpForListOrFetch(1)
	response, count, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)

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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)

	suite.mockPulpForListOrFetch(1)
	response, count, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)

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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)

	_, err := repoConfigDao.Create(context.Background(), api.RepositoryRequest{
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

	suite.mockPulpForListOrFetch(1)
	response, total, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)

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
	err := GetRepositoryConfigDao(tx, suite.mockPulpClient).SavePublicRepos(context.Background(), repoUrls)
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
	err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).SavePublicRepos(context.Background(), repoUrls)
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

	err = GetRepositoryConfigDao(tx, suite.mockPulpClient).SoftDelete(context.Background(), repoConfig.OrgID, repoConfig.UUID)
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

	err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).SoftDelete(context.Background(), "bad org id", found.UUID)
	assert.Error(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient).Delete(context.Background(), "bad org id", found.UUID)
	assert.Error(t, err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	err = suite.tx.
		First(&found, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)
}

func (suite *RepositoryConfigSuite) TestBulkDelete() {
	t := suite.T()
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	orgID := seeds.RandomOrgId()
	repoConfigCount := 5

	err := seeds.SeedRepositoryConfigurations(suite.tx, repoConfigCount, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	var uuids []string
	err = suite.tx.Model(models.RepositoryConfiguration{}).Where("org_id = ?", orgID).Select("uuid").Find(&uuids).Error
	assert.NoError(t, err)
	assert.Len(t, uuids, repoConfigCount)

	errs := dao.BulkDelete(context.Background(), orgID, uuids)
	assert.Len(t, errs, 0)

	var found []models.RepositoryConfiguration
	err = suite.tx.Where("org_id = ?", orgID).Find(&found).Error
	assert.NoError(t, err)
	assert.Len(t, found, 0)
}

func (suite *RepositoryConfigSuite) TestUpdateLastSnapshotTask() {
	t := suite.T()
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	orgID := seeds.RandomOrgId()
	repoConfigCount := 1

	err := seeds.SeedRepositoryConfigurations(suite.tx, repoConfigCount, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	var uuids []string
	err = suite.tx.Model(models.RepositoryConfiguration{}).Where("org_id = ?", orgID).Select("repository_uuid").Find(&uuids).Error
	assert.NoError(t, err)
	assert.Len(t, uuids, repoConfigCount)

	taskUUID := uuid.NewString()

	err = dao.UpdateLastSnapshotTask(context.Background(), taskUUID, orgID, uuids[0])
	assert.Nil(t, err)

	var found []models.RepositoryConfiguration
	err = suite.tx.Where("org_id = ?", orgID).Find(&found).Error
	assert.NoError(t, err)
	assert.Equal(t, taskUUID, found[0].LastSnapshotTaskUUID)
}

func (suite *RepositoryConfigSuite) TestBulkDeleteOneNotFound() {
	t := suite.T()
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	orgID := seeds.RandomOrgId()
	repoConfigCount := 5

	err := seeds.SeedRepositoryConfigurations(suite.tx, repoConfigCount, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	var uuids []string
	err = suite.tx.Model(models.RepositoryConfiguration{}).Where("org_id = ?", orgID).Select("uuid").Find(&uuids).Error
	assert.NoError(t, err)
	assert.Len(t, uuids, repoConfigCount)
	uuids[1] = uuid.NewString()

	errs := dao.BulkDelete(context.Background(), orgID, uuids)
	assert.Len(t, errs, repoConfigCount)
	assert.Error(t, errs[1])

	var found []models.RepositoryConfiguration
	err = suite.tx.Where("org_id = ?", orgID).Find(&found).Error
	assert.NoError(t, err)
	assert.Len(t, found, repoConfigCount)
}

func (suite *RepositoryConfigSuite) TestBulkDeleteRedhatRepository() {
	t := suite.T()
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	orgID := config.RedHatOrg
	repoConfigCount := 5
	existingRepoConfigCount := int64(0)

	suite.tx.Model(models.RepositoryConfiguration{}).Where("org_id = ?", config.RedHatOrg).Count(&existingRepoConfigCount)

	err := seeds.SeedRepositoryConfigurations(suite.tx, repoConfigCount, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	errs := dao.BulkDelete(context.Background(), orgID, []string{"doesn't matter"})
	assert.Len(t, errs, 1)
	assert.Equal(t, ce.HttpCodeForDaoError(errs[0]), 404)

	var found []models.RepositoryConfiguration
	err = suite.tx.Where("org_id = ?", orgID).Find(&found).Error
	assert.NoError(t, err)
	assert.Len(t, found, repoConfigCount+int(existingRepoConfigCount))
}

func (suite *RepositoryConfigSuite) TestBulkDeleteMultipleNotFound() {
	t := suite.T()
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	orgID := seeds.RandomOrgId()
	repoConfigCount := 5

	err := seeds.SeedRepositoryConfigurations(suite.tx, repoConfigCount, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	var uuids []string
	err = suite.tx.Model(models.RepositoryConfiguration{}).Where("org_id = ?", orgID).Select("uuid").Find(&uuids).Error
	assert.NoError(t, err)
	assert.Len(t, uuids, repoConfigCount)
	uuids[1] = uuid.NewString()
	uuids[3] = uuid.NewString()

	errs := dao.BulkDelete(context.Background(), orgID, uuids)
	assert.Len(t, errs, repoConfigCount)
	assert.Error(t, errs[1])
	assert.Error(t, errs[3])

	var found []models.RepositoryConfiguration
	err = suite.tx.Where("org_id = ?", orgID).Find(&found).Error
	assert.NoError(t, err)
	assert.Len(t, found, repoConfigCount)
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
	response, err := dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
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
	response, err = dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{*parameters.UUID})
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
	response, err := dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
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
	response, err := dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
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

	response, err := dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)

	assert.True(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.True(t, response.URL.Valid)
	assert.True(t, response.URL.MetadataPresent)
	assert.False(t, response.URL.Skipped)
}

func (suite *RepositoryConfigSuite) TestValidateParametersBadUUIDAndUrl() {
	t := suite.T()
	mockYumRepo, dao, repoConfig := suite.setupValidationTest()
	// Providing a bad url that doesn't have a repo
	parameters := api.RepositoryValidationRequest{
		UUID: pointy.Pointer("not.a.real.UUID"),
		Name: pointy.Pointer("Some bad repo!"),
		URL:  pointy.Pointer("http://badrepo.example.com/"),
	}
	mockYumRepo.Mock.On("Repomd").Return(nil, 404, nil)

	response, err := dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)

	assert.True(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
	assert.True(t, response.URL.Valid) // Even if the metadata isn't present, the URL itself is valid
	assert.Equal(t, response.URL.HTTPCode, 404)
	assert.False(t, response.URL.MetadataPresent)
	assert.False(t, response.URL.Skipped)
}

func (suite *RepositoryConfigSuite) TestValidateParametersNameBadUUID() {
	t := suite.T()
	mockYumRepo, dao, repoConfig := suite.setupValidationTest()
	// Providing a bad url that doesn't have a repo
	parameters := api.RepositoryValidationRequest{
		Name: pointy.Pointer("Somebadrepo!"),
	}
	mockYumRepo.Mock.On("Repomd").Return(nil, 404, nil)

	response, err := dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{"not.a.real.UUID"})
	assert.NoError(t, err)

	assert.True(t, response.Name.Valid)
	assert.False(t, response.Name.Skipped)
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

	response, err := dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
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

	response, err := dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
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

	response, err := dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)
	assert.False(t, response.GPGKey.Valid)
	assert.True(t, response.URL.MetadataSignaturePresent)
	assert.True(t, response.URL.Valid)

	// retest disabling metadata verification
	parameters.MetadataVerification = false
	response, err = dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
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

	response, err := dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)
	assert.False(t, response.GPGKey.Valid)
	assert.True(t, response.URL.MetadataSignaturePresent)
	assert.True(t, response.URL.Valid)
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

type RepoToSnapshotTest struct {
	Name                     string
	Opts                     *seeds.TaskSeedOptions
	Included                 bool
	OptionAlwaysRunCronTasks bool
	Filter                   *ListRepoFilter
}

func (suite *RepositoryConfigSuite) TestListReposToSnapshot() {
	defer func() {
		config.Get().Options.AlwaysRunCronTasks = false
	}()

	t := suite.T()
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)

	repo, err := dao.Create(context.Background(), api.RepositoryRequest{
		Name:             pointy.Pointer("name"),
		URL:              pointy.Pointer("http://example.com/"),
		OrgID:            pointy.Pointer("123"),
		AccountID:        pointy.Pointer("123"),
		DistributionArch: pointy.Pointer("x86_64"),
		DistributionVersions: &[]string{
			config.El9,
		},
		Snapshot: pointy.Pointer(true),
	})
	assert.NoError(t, err)
	yesterday := time.Now().Add(time.Hour * time.Duration(-48))
	cases := []RepoToSnapshotTest{
		{
			Name:     "Never been synced",
			Opts:     nil,
			Included: true,
		},
		{
			Name:     "Snapshot is running",
			Opts:     &seeds.TaskSeedOptions{RepoConfigUUID: repo.UUID, OrgID: repo.OrgID, Status: config.TaskStatusRunning},
			Included: false,
		},
		{
			Name:     "Previous Snapshot Failed",
			Opts:     &seeds.TaskSeedOptions{RepoConfigUUID: repo.UUID, OrgID: repo.OrgID, Status: config.TaskStatusFailed},
			Included: true,
		},
		{
			Name:     "Previous Snapshot Failed, and url specified",
			Opts:     &seeds.TaskSeedOptions{RepoConfigUUID: repo.UUID, OrgID: repo.OrgID, Status: config.TaskStatusFailed},
			Included: true,
			Filter:   &ListRepoFilter{URLs: &[]string{repo.URL}},
		},
		{
			Name:     "Previous Snapshot Failed, and url specified",
			Opts:     &seeds.TaskSeedOptions{RepoConfigUUID: repo.UUID, OrgID: repo.OrgID, Status: config.TaskStatusFailed},
			Included: false,
			Filter:   &ListRepoFilter{RedhatOnly: pointy.Pointer(true)},
		},
		{
			Name:     "Previous Snapshot was successful and recent",
			Opts:     &seeds.TaskSeedOptions{RepoConfigUUID: repo.UUID, OrgID: repo.OrgID, Status: config.TaskStatusCompleted},
			Included: false,
		},
		{
			Name:     "Previous Snapshot was successful and 24 hours ago",
			Opts:     &seeds.TaskSeedOptions{RepoConfigUUID: repo.UUID, OrgID: repo.OrgID, Status: config.TaskStatusCompleted, QueuedAt: &yesterday},
			Included: true,
		},
		{
			Name:                     "Previous Snapshot was successful and recent but Always run is set to true",
			Opts:                     &seeds.TaskSeedOptions{RepoConfigUUID: repo.UUID, OrgID: repo.OrgID, Status: config.TaskStatusCompleted},
			Included:                 true,
			OptionAlwaysRunCronTasks: true,
		},
	}

	for _, testCase := range cases {
		found := false
		if testCase.Opts != nil {
			tasks, err := seeds.SeedTasks(suite.tx, 1, *testCase.Opts)
			assert.NoError(t, err)
			err = dao.UpdateLastSnapshotTask(context.Background(), tasks[0].Id.String(), repo.OrgID, repo.RepositoryUUID)
			assert.NoError(t, err)
		}

		config.Get().Options.AlwaysRunCronTasks = testCase.OptionAlwaysRunCronTasks

		afterRepos, err := dao.InternalOnly_ListReposToSnapshot(context.Background(), testCase.Filter)
		assert.NoError(t, err)
		for i := range afterRepos {
			if repo.UUID == afterRepos[i].UUID {
				found = true
			}
		}
		assert.Equal(t, testCase.Included, found, "Test case %v, expected to be found: %v, but was: %v", testCase.Name, testCase.Included, found)
	}
}

func (suite *RepositoryConfigSuite) TestRefreshRedHatRepo() {
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient)
	rhRepo := api.RepositoryRequest{
		UUID:                 nil,
		Name:                 pointy.Pointer("Some redhat repo"),
		URL:                  pointy.Pointer("https://testurl"),
		DistributionVersions: pointy.Pointer([]string{"8"}),
		DistributionArch:     pointy.Pointer("x86_64"),
		GpgKey:               nil,
		MetadataVerification: pointy.Pointer(false),
		Origin:               nil,
		ContentType:          pointy.Pointer(config.ContentTypeRpm),
		Snapshot:             pointy.Pointer(true),
	}
	response, err := dao.InternalOnly_RefreshRedHatRepo(context.Background(), rhRepo, "another-label")
	assert.NoError(suite.T(), err)

	assert.NotEmpty(suite.T(), response.UUID)
	assert.Equal(suite.T(), config.OriginRedHat, response.Origin)
	assert.Equal(suite.T(), config.RedHatOrg, response.OrgID)
	assert.Equal(suite.T(), "another-label", response.Label)

	// Change the name
	rhRepo.Name = pointy.Pointer("another name")

	response, err = dao.InternalOnly_RefreshRedHatRepo(context.Background(), rhRepo, "some-label")
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), *rhRepo.Name, response.Name)
	assert.Equal(suite.T(), "some-label", response.Label)
}

func (suite *RepositoryConfigSuite) mockPulpForListOrFetch(times int) {
	if config.Get().Features.Snapshots.Enabled {
		suite.mockPulpClient.WithDomainMock().On("GetContentPath", context.Background()).Return(testContentPath, nil).Times(times)
	}
}

func (suite *RepositoryConfigSuite) TestCombineStatus() {
	t := suite.T()

	cases := []struct {
		Name       string
		RepoConfig *models.RepositoryConfiguration
		Repo       *models.Repository
		Expected   string
	}{
		{
			Name: "Both introspection and snapshot were successful",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusCompleted},
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusValid,
			},
			Expected: "Valid",
		},
		{
			Name: "Introspection and snapshot both pending / running",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusRunning},
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusPending,
			},
			Expected: "Pending",
		},
		{
			Name: "Introspection successful, snapshot is running, and repo has no previous snapshots",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusRunning},
				LastSnapshotUUID: "",
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusValid,
			},
			Expected: "Pending",
		},
		{
			Name: "Introspection successful, snapshot is pending, and repo has no previous snapshots",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusPending},
				LastSnapshotUUID: "",
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusValid,
			},
			Expected: "Pending",
		},
		{
			Name: "Introspection pending, last snapshot successful, and repo has no previous snapshots",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusCompleted},
				LastSnapshotUUID: "",
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusPending,
			},
			Expected: "Pending",
		},
		{
			Name: "Introspection unavailable, last snapshot failed, and repo has no previous snapshots",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusFailed},
				LastSnapshotUUID: "",
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusUnavailable,
			},
			Expected: "Invalid",
		},
		{
			Name: "Introspection failed and last snapshot was successful",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusCompleted},
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusInvalid,
			},
			Expected: "Invalid",
		},
		{
			Name: "Introspection successful, last snapshot failed, and repo has no previous snapshots",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusFailed},
				LastSnapshotUUID: "",
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusValid,
			},
			Expected: "Invalid",
		},
		{
			Name: "Both introspection and snapshot failed and repo has previous snapshots",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusFailed},
				LastSnapshotUUID: uuid.NewString(),
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusInvalid,
			},
			Expected: "Unavailable",
		},
		{
			Name: "Introspection unavailable, last snapshot failed, and repo has previous snapshots",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusFailed},
				LastSnapshotUUID: uuid.NewString(),
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusUnavailable,
			},
			Expected: "Unavailable",
		},
		{
			Name: "Introspection unavailable, last snapshot successful, and repo has previous snapshots",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusCompleted},
				LastSnapshotUUID: uuid.NewString(),
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusUnavailable,
			},
			Expected: "Unavailable",
		},
		{
			Name: "Introspection successful, snapshot is running, and repo has previous snapshots",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusRunning},
				LastSnapshotUUID: uuid.NewString(),
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusValid,
			},
			Expected: "Pending",
		},
		{
			Name: "Introspection successful, snapshot is pending, and repo has previous snapshots",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusPending},
				LastSnapshotUUID: uuid.NewString(),
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusValid,
			},
			Expected: "Pending",
		},
		{
			Name: "Introspection successful, last snapshot failed, and repo has previous snapshots",
			RepoConfig: &models.RepositoryConfiguration{
				Snapshot:         true,
				LastSnapshotTask: &models.TaskInfo{Status: config.TaskStatusFailed},
				LastSnapshotUUID: uuid.NewString(),
			},
			Repo: &models.Repository{
				LastIntrospectionStatus: config.StatusValid,
			},
			Expected: "Unavailable",
		},
	}

	for _, testCase := range cases {
		result := combineIntrospectionAndSnapshotStatuses(testCase.RepoConfig, testCase.Repo)
		assert.Equal(t, testCase.Expected, result, testCase.Name)
	}
}
