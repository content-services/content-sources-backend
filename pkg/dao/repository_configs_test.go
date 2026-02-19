package dao

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

var originCustom = config.OriginExternal + "," + config.OriginUpload

type RepositoryConfigSuite struct {
	*DaoSuite
	mockPulpClient *pulp_client.MockPulpClient
	mockFsClient   *feature_service_client.MockFeatureServiceClient
}

func TestRepositoryConfigSuite(t *testing.T) {
	m := DaoSuite{}
	r := RepositoryConfigSuite{
		DaoSuite:       &m,
		mockPulpClient: pulp_client.NewMockPulpClient(t),
		mockFsClient:   feature_service_client.NewMockFeatureServiceClient(t),
	}
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

	dao := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient)
	created, err := dao.Create(context.Background(), toCreate)
	require.Nil(t, err)

	foundRepo, err := dao.Fetch(context.Background(), orgID, created.UUID)
	require.Nil(t, err)
	assert.Equal(t, url, foundRepo.URL)
	assert.Equal(t, true, foundRepo.ModuleHotfixes)
}

func (suite *RepositoryConfigSuite) TestCreateRedHat() {
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
		Origin:           utils.Ptr(config.OriginRedHat),
		AccountID:        &accountId,
		DistributionArch: &distributionArch,
		DistributionVersions: &[]string{
			config.El9,
		},
		GpgKey:               &gpgKey,
		MetadataVerification: &metadataVerification,
		ModuleHotfixes:       &moduleHotfixes,
	}

	dao := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient)
	_, err = dao.Create(context.Background(), toCreate)
	require.Error(t, err)
	assert.ErrorContains(t, err, "red_hat")
	assert.ErrorContains(t, err, "is not permitted")
}

func (suite *RepositoryConfigSuite) TestCreateUpload() {
	rcDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)

	toCreate := api.RepositoryRequest{
		Name:      utils.Ptr("myRepo"),
		URL:       utils.Ptr("http://example.com/"),
		OrgID:     utils.Ptr("123"),
		AccountID: utils.Ptr("123"),
		Origin:    utils.Ptr(config.OriginUpload),
		Snapshot:  utils.Ptr(true),
	}
	_, err := rcDao.Create(context.Background(), toCreate)
	assert.ErrorContains(suite.T(), err, "URL cannot be specified for upload repositories.")

	toCreate.URL = nil
	repo, err := rcDao.Create(context.Background(), toCreate)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), repo.UUID != "")

	// create a second repo
	toCreate2 := api.RepositoryRequest{
		Name:      utils.Ptr("myRepo2"),
		OrgID:     utils.Ptr("123"),
		AccountID: utils.Ptr("123"),
		Origin:    utils.Ptr(config.OriginUpload),
		Snapshot:  utils.Ptr(true),
	}

	repo2, err := rcDao.Create(context.Background(), toCreate2)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), repo.UUID != "")
	assert.NotEqual(suite.T(), repo.UUID, repo2.UUID)
	assert.NotEmpty(suite.T(), repo.LastIntrospectionTime)
	assert.NotEmpty(suite.T(), repo.LastIntrospectionStatus)
}

func (suite *RepositoryConfigSuite) TestCreateUploadNoSnap() {
	rcDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)

	toCreate := api.RepositoryRequest{
		Name:      utils.Ptr("myRepo"),
		OrgID:     utils.Ptr("123"),
		AccountID: utils.Ptr("123"),
		Origin:    utils.Ptr(config.OriginUpload),
		Snapshot:  utils.Ptr(false),
	}
	_, err := rcDao.Create(context.Background(), toCreate)
	assert.ErrorContains(suite.T(), err, "Snapshot must be true for upload repositories")
}

func (suite *RepositoryConfigSuite) TestCreateUploadURL() {
	rcDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)

	toCreate := api.RepositoryRequest{
		Name:      utils.Ptr("myRepo"),
		URL:       utils.Ptr("http://example.com/"),
		OrgID:     utils.Ptr("123"),
		AccountID: utils.Ptr("123"),
		Origin:    utils.Ptr(config.OriginUpload),
		Snapshot:  utils.Ptr(true),
	}
	_, err := rcDao.Create(context.Background(), toCreate)
	assert.ErrorContains(suite.T(), err, "URL cannot be specified for upload repositories.")
}

func (suite *RepositoryConfigSuite) TestCreateUpdateUploadWithExistingURL() {
	rcDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	url := "http://example.com/testcreateuploadexistingurl/"
	err := suite.tx.Create(&models.Repository{URL: url}).Error
	require.NoError(suite.T(), err)

	repo, err := rcDao.Create(context.Background(), api.RepositoryRequest{
		OrgID:    utils.Ptr("123"),
		Origin:   utils.Ptr("upload"),
		Name:     utils.Ptr(url),
		URL:      utils.Ptr(url),
		Snapshot: utils.Ptr(true),
	})
	assert.NotNil(suite.T(), err)
	assert.Empty(suite.T(), repo.UUID)

	repo, err = rcDao.Create(context.Background(), api.RepositoryRequest{
		OrgID:    utils.Ptr("123"),
		Origin:   utils.Ptr("upload"),
		Name:     utils.Ptr(url),
		Snapshot: utils.Ptr(true),
	})
	assert.Nil(suite.T(), err)
	assert.NotEmpty(suite.T(), repo.UUID)

	_, err = rcDao.Update(context.Background(), repo.OrgID, repo.UUID, api.RepositoryUpdateRequest{URL: &url})
	assert.NotNil(suite.T(), err)
}

func (suite *RepositoryConfigSuite) TestCreateTwiceWithNoSlash() {
	toCreate := api.RepositoryRequest{
		Name:             utils.Ptr(""),
		URL:              utils.Ptr("something-no-slash"),
		OrgID:            utils.Ptr("123"),
		AccountID:        utils.Ptr("123"),
		DistributionArch: utils.Ptr(""),
		DistributionVersions: &[]string{
			config.El9,
		},
	}
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	_, err := dao.Create(context.Background(), toCreate)
	assert.ErrorContains(suite.T(), err, "Invalid URL for request.")

	dao = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	_, err = dao.Create(context.Background(), toCreate)
	assert.ErrorContains(suite.T(), err, "Invalid URL for request.")
}

func (suite *RepositoryConfigSuite) TestCreateRedHatRepository() {
	toCreate := api.RepositoryRequest{
		Name:             utils.Ptr(""),
		URL:              utils.Ptr("something-no-slash"),
		OrgID:            utils.Ptr(config.RedHatOrg),
		AccountID:        utils.Ptr("123"),
		DistributionArch: utils.Ptr(""),
		DistributionVersions: &[]string{
			config.El9,
		},
	}
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	_, err := dao.Create(context.Background(), toCreate)
	assert.ErrorContains(suite.T(), err, "creating of Red Hat repositories is not permitted")
}

func (suite *RepositoryConfigSuite) TestCreateAlreadyExists() {
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepository(tx, 1, seeds.SeedOptions{})
	assert.NoError(t, err)
	var repo []models.Repository
	err = tx.Limit(1).Find(&repo).Error
	assert.NoError(t, err)

	_, err = seeds.SeedRepositoryConfigurations(tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.NoError(t, err)

	found := models.RepositoryConfiguration{}
	err = tx.
		Preload("Repository").
		First(&found, "org_id = ?", orgID).
		Error
	require.NoError(t, err)

	// Force failure on creating duplicate
	tx.SavePoint("before")
	_, err = GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).Create(context.Background(), api.RepositoryRequest{
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
			assert.True(t, daoError.AlreadyExists)
			assert.Contains(t, err.Error(), "name")
		}
	}
	tx.RollbackTo("before")

	// Force failure on creating duplicate url
	_, err = GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).Create(context.Background(), api.RepositoryRequest{
		Name:      utils.Ptr("new name"),
		URL:       &found.Repository.URL,
		OrgID:     &found.OrgID,
		AccountID: &found.AccountID,
	})
	assert.Error(t, err)
	if err != nil {
		daoError, ok := err.(*ce.DaoError)
		assert.True(t, ok)
		if ok {
			assert.True(t, daoError.AlreadyExists)
			assert.Contains(t, err.Error(), "URL")
		}
	}
	tx.RollbackTo("before")
}

func (suite *RepositoryConfigSuite) TestCreateDuplicateLabel() {
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()
	var err error

	err = seeds.SeedRepository(tx, 1, seeds.SeedOptions{})
	assert.NoError(t, err)
	var repo []models.Repository
	err = tx.Limit(1).Find(&repo).Error
	assert.NoError(t, err)

	_, err = seeds.SeedRepositoryConfigurations(tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.NoError(t, err)

	found := models.RepositoryConfiguration{}
	err = tx.
		Preload("Repository").
		First(&found, "org_id = ?", orgID).
		Error
	require.NoError(t, err)
	nameForDupeLabel := strings.ReplaceAll(found.Name, "-", "_")
	nameForDupeLabel = strings.ReplaceAll(nameForDupeLabel, " ", "_")
	resp, err := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).Create(context.Background(), api.RepositoryRequest{
		Name:      &nameForDupeLabel,
		URL:       utils.Ptr("http://example.com"),
		OrgID:     &found.OrgID,
		AccountID: &found.AccountID,
	})
	assert.NoError(t, err)
	assert.Contains(t, resp.Label, found.Label)
}

func (suite *RepositoryConfigSuite) TestCreateEPELRepositoryShouldFail() {
	defer func() {
		config.Get().Features.CommunityRepos.Enabled = false
		config.Get().Features.AllowCustomEPELCreation.Enabled = true
	}()
	config.Get().Features.CommunityRepos.Enabled = true
	config.Get().Features.AllowCustomEPELCreation.Enabled = false

	toCreate := api.RepositoryRequest{
		Name:             utils.Ptr("epel-repo-1"),
		URL:              utils.Ptr("https://epel-repo.org"),
		OrgID:            utils.Ptr(config.CommunityOrg),
		DistributionArch: utils.Ptr("x86_64"),
		DistributionVersions: &[]string{
			config.El9,
		},
	}
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	_, err := dao.Create(context.Background(), toCreate)
	assert.ErrorContains(suite.T(), err, "creating of EPEL repositories is not permitted, please use the community repositories")

	toCreate = api.RepositoryRequest{
		Name:             utils.Ptr("epel-repo-2"),
		URL:              utils.Ptr("https://epel-repo.org"),
		Origin:           utils.Ptr("community"),
		OrgID:            utils.Ptr(orgIDTest),
		DistributionArch: utils.Ptr("x86_64"),
		DistributionVersions: &[]string{
			config.El9,
		},
	}
	_, err = dao.Create(context.Background(), toCreate)
	assert.ErrorContains(suite.T(), err, "creating of EPEL repositories is not permitted, please use the community repositories")

	toCreate = api.RepositoryRequest{
		Name:             utils.Ptr("epel-repo-3"),
		URL:              utils.Ptr(config.EPEL10Url),
		OrgID:            utils.Ptr(orgIDTest),
		DistributionArch: utils.Ptr("x86_64"),
		DistributionVersions: &[]string{
			config.El10,
		},
	}
	_, err = dao.Create(context.Background(), toCreate)
	assert.ErrorContains(suite.T(), err, "creating of EPEL repositories is not permitted, please use the community repositories")
}

func (suite *RepositoryConfigSuite) TestRepositoryUrlInvalid() {
	t := suite.T()
	tx := suite.tx

	invalidURL := "hey/there!"
	invalidURL2 := "golang.org"
	invalidURL3 := "go lang.org"
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
		{
			given: api.RepositoryRequest{
				Name:  &name,
				URL:   &invalidURL3,
				OrgID: &OrgID,
			},
			expected: "URL cannot contain whitespace.",
		},
	}
	tx.SavePoint("testrepositorycreateinvalidtest")
	for i := 0; i < len(invalidItems); i++ {
		_, err := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).Create(context.Background(), invalidItems[i].given)
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
			expected: "URL cannot be blank for custom and Red Hat repositories",
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
		_, err := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).Create(context.Background(), blankItems[i].given)
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

	err = tx.Where("origin != ?", config.OriginUpload).Where("url IS NOT NULL").First(&repository).Error
	require.NoError(t, err)
	assert.NotEmpty(t, repository)
	urlNoSlash := repository.URL[0 : len(repository.URL)-1]

	// create repository without trailing slash to see that URL is cleaned up before query for repository
	request := []api.RepositoryRequest{
		{
			Name:  utils.Ptr("repo"),
			URL:   utils.Ptr(urlNoSlash),
			OrgID: utils.Ptr(orgID),
		},
	}

	rr, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).BulkCreate(context.Background(), request)
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
			ModuleHotfixes: utils.Ptr(i%3 == 0),
		}
	}

	rr, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).BulkCreate(context.Background(), requests)
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

func (suite *RepositoryConfigSuite) TestBulkCreateRedHat() {
	t := suite.T()
	tx := suite.tx

	orgID := seeds.RandomOrgId()
	requests := make([]api.RepositoryRequest, 1)
	name := "repo_1"
	url := "https://repo.example.com"
	requests[0] = api.RepositoryRequest{
		Name:   &name,
		URL:    &url,
		OrgID:  &orgID,
		Origin: utils.Ptr(config.OriginRedHat),
	}

	_, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).BulkCreate(context.Background(), requests)
	assert.Error(t, errs[0])
	assert.ErrorContains(t, errs[0], "origin")
	assert.ErrorContains(t, errs[0], "is not permitted")
}

func (suite *RepositoryConfigSuite) TestBulkCreateUpload() {
	t := suite.T()
	tx := suite.tx

	orgID := seeds.RandomOrgId()
	requests := make([]api.RepositoryRequest, 1)
	requests[0] = api.RepositoryRequest{
		Name:     utils.Ptr("uploadbulktest"),
		Origin:   utils.Ptr(config.OriginUpload),
		OrgID:    &orgID,
		Snapshot: utils.Ptr(true),
	}

	rr, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).BulkCreate(context.Background(), requests)
	assert.Empty(t, errs)
	assert.NotEmpty(t, rr[0].LastIntrospectionStatus)

	var foundRepoConfig models.RepositoryConfiguration
	err := tx.
		Where("name = ? AND org_id = ?", requests[0].Name, orgID).
		Find(&foundRepoConfig).
		Error
	assert.NoError(t, err)
	assert.NotEmpty(t, foundRepoConfig.UUID)
}

func (suite *RepositoryConfigSuite) TestBulkCreateUploadSnapshotFalse() {
	t := suite.T()
	tx := suite.tx

	orgID := seeds.RandomOrgId()
	requests := make([]api.RepositoryRequest, 1)
	requests[0] = api.RepositoryRequest{
		Name:     utils.Ptr("uploadbulktest"),
		Origin:   utils.Ptr(config.OriginUpload),
		OrgID:    &orgID,
		Snapshot: utils.Ptr(false),
	}

	_, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).BulkCreate(context.Background(), requests)
	assert.NotEmpty(t, errs)
	assert.ErrorContains(t, errs[0], "Snapshot must be true for upload repositories")
}

func (suite *RepositoryConfigSuite) TestBulkCreateOneFails() {
	t := suite.T()
	tx := suite.tx

	orgID := orgIDTest
	accountID := accountIdTest

	requests := []api.RepositoryRequest{
		{
			Name:      utils.Ptr(""),
			URL:       utils.Ptr("https://repo_2_url.org"),
			OrgID:     &orgID,
			AccountID: &accountID,
		},
		{
			Name:      utils.Ptr("repo_1"),
			URL:       utils.Ptr("https://repo_1_url.org"),
			OrgID:     &orgID,
			AccountID: &accountID,
		},
	}

	rr, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).BulkCreate(context.Background(), requests)

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

func (suite *RepositoryConfigSuite) TestBulkCreateEPELReposShouldFail() {
	t := suite.T()
	tx := suite.tx
	defer func() {
		config.Get().Features.CommunityRepos.Enabled = false
		config.Get().Features.AllowCustomEPELCreation.Enabled = true
	}()
	config.Get().Features.CommunityRepos.Enabled = true
	config.Get().Features.AllowCustomEPELCreation.Enabled = false

	requests := []api.RepositoryRequest{
		{
			Name:  utils.Ptr("epel-repo-1"),
			URL:   utils.Ptr("https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/"),
			OrgID: utils.Ptr(test_handler.MockOrgId),
		},
		{
			Name:  utils.Ptr("epel-repo-2"),
			URL:   utils.Ptr("https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/"),
			OrgID: utils.Ptr(test_handler.MockOrgId),
		},
		{
			Name:  utils.Ptr("epel-repo-3"),
			URL:   utils.Ptr("https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/"),
			OrgID: utils.Ptr(test_handler.MockOrgId),
		},
		{
			Name:  utils.Ptr("epel-repo-4"),
			URL:   utils.Ptr("https://epel-repo.org"),
			OrgID: utils.Ptr(config.CommunityOrg),
		},
		{
			Name:   utils.Ptr("epel-repo-5"),
			URL:    utils.Ptr("https://epel-repo.org"),
			OrgID:  utils.Ptr(test_handler.MockOrgId),
			Origin: utils.Ptr(config.OriginCommunity),
		},
	}

	_, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).BulkCreate(context.Background(), requests)
	assert.NotEmpty(t, errs)
	for i := 0; i < len(errs); i++ {
		assert.ErrorContains(t, errs[i], "creating of EPEL repositories is not permitted, please use the community repositories")
	}
}

func (suite *RepositoryConfigSuite) TestBulkImportNoneExist() {
	t := suite.T()
	tx := suite.tx

	orgID := orgIDTest
	accountID := accountIdTest

	amountToImport := 20

	requests := make([]api.RepositoryRequest, amountToImport)
	for i := range amountToImport {
		name := "repo_" + strconv.Itoa(i)
		url := "https://repo_" + strconv.Itoa(i)
		request := api.RepositoryRequest{
			Name:                 &name,
			OrgID:                &orgID,
			AccountID:            &accountID,
			DistributionVersions: &[]string{"any"},
			DistributionArch:     utils.Ptr("any"),
			GpgKey:               utils.Ptr(""),
			MetadataVerification: utils.Ptr(false),
			ModuleHotfixes:       utils.Ptr(false),
		}
		if i < amountToImport/2 {
			request.URL = utils.Ptr(url)
			request.Snapshot = utils.Ptr(false)
		} else {
			request.Origin = utils.Ptr(config.OriginUpload)
		}
		requests[i] = request
	}
	tx.SavePoint("testbulkimportnoneexist")
	rr, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).BulkImport(context.Background(), requests)
	assert.Empty(t, errs)
	assert.Equal(t, amountToImport, len(rr))
	for i := range rr {
		if rr[i].Origin == config.OriginUpload {
			assert.NotEmpty(t, rr[i].Warnings)
			assert.Equal(t, UploadRepositoryWarning, rr[i].Warnings[0]["description"])
			assert.Equal(t, true, rr[i].Snapshot)
		} else {
			assert.Empty(t, rr[i].Warnings)
		}
	}

	for i := range amountToImport {
		var foundRepoConfig models.RepositoryConfiguration
		err := tx.
			Where("name = ?", requests[i].Name).
			Find(&foundRepoConfig).
			Error
		assert.NoError(t, err)
		assert.NotEmpty(t, foundRepoConfig.UUID)
		if foundRepoConfig.Repository.Origin == config.OriginUpload {
			assert.Equal(t, true, foundRepoConfig.Snapshot)
		}
	}
	tx.RollbackTo("testbulkimportnoneexist")
}

func (suite *RepositoryConfigSuite) TestBulkImportOneExists() {
	t := suite.T()
	tx := suite.tx

	orgID := orgIDTest
	accountID := accountIdTest

	requests := []api.RepositoryRequest{
		{
			Name:                 utils.Ptr("existing_repo"),
			URL:                  utils.Ptr("https://existing_repo_url.org"),
			OrgID:                &orgID,
			AccountID:            &accountID,
			DistributionVersions: &[]string{"any"},
			DistributionArch:     utils.Ptr("any"),
			GpgKey:               utils.Ptr(""),
			MetadataVerification: utils.Ptr(false),
			ModuleHotfixes:       utils.Ptr(false),
			Snapshot:             utils.Ptr(false),
		},
		{
			Name:                 utils.Ptr("new_repo"),
			URL:                  utils.Ptr("https://new_repo_url.org"),
			OrgID:                &orgID,
			AccountID:            &accountID,
			DistributionVersions: &[]string{"any"},
			DistributionArch:     utils.Ptr("any"),
			GpgKey:               utils.Ptr(""),
			MetadataVerification: utils.Ptr(false),
			ModuleHotfixes:       utils.Ptr(false),
			Snapshot:             utils.Ptr(false),
		},
		{
			Name:                 utils.Ptr("existing_upload_repo"),
			Origin:               utils.Ptr(config.OriginUpload),
			OrgID:                &orgID,
			AccountID:            &accountID,
			DistributionVersions: &[]string{"any"},
			DistributionArch:     utils.Ptr("any"),
			GpgKey:               utils.Ptr(""),
			MetadataVerification: utils.Ptr(false),
			ModuleHotfixes:       utils.Ptr(false),
			Snapshot:             utils.Ptr(true),
		},
		{
			Name:                 utils.Ptr("new_upload_repo"),
			Origin:               utils.Ptr(config.OriginUpload),
			OrgID:                &orgID,
			AccountID:            &accountID,
			DistributionVersions: &[]string{"any"},
			DistributionArch:     utils.Ptr("any"),
			GpgKey:               utils.Ptr(""),
			MetadataVerification: utils.Ptr(false),
			ModuleHotfixes:       utils.Ptr(false),
		},
	}

	tx.SavePoint("testbulkimportoneexists")
	_, err := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).Create(context.Background(), requests[0])
	assert.Empty(t, err)
	_, err = GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).Create(context.Background(), requests[2])
	assert.Empty(t, err)

	rr, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).BulkImport(context.Background(), requests)
	assert.Empty(t, errs)
	assert.Equal(t, 4, len(rr))
	assert.NotEmpty(t, rr[0].Warnings)
	assert.Empty(t, rr[1].Warnings)
	assert.NotEmpty(t, rr[2].Warnings)
	assert.NotEmpty(t, rr[3].Warnings)
	assert.Equal(t, UploadRepositoryWarning, rr[3].Warnings[0]["description"])
	assert.Equal(t, true, rr[3].Snapshot)

	for i := 0; i < 2; i++ {
		var foundRepoConfig models.RepositoryConfiguration
		err := tx.
			Where("name = ?", requests[i].Name).
			Find(&foundRepoConfig).
			Error
		assert.NoError(t, err)
		assert.NotEmpty(t, foundRepoConfig.UUID)
	}
	tx.RollbackTo("testbulkimportoneexists")
}

func (suite *RepositoryConfigSuite) TestBulkImportOneFails() {
	t := suite.T()
	tx := suite.tx

	orgID := orgIDTest
	accountID := accountIdTest

	requests := []api.RepositoryRequest{
		{
			Name:                 utils.Ptr(""),
			URL:                  utils.Ptr("https://existing_repo_url.org"),
			OrgID:                &orgID,
			AccountID:            &accountID,
			DistributionVersions: &[]string{"any"},
			DistributionArch:     utils.Ptr("any"),
			GpgKey:               utils.Ptr(""),
			MetadataVerification: utils.Ptr(false),
			ModuleHotfixes:       utils.Ptr(false),
			Snapshot:             utils.Ptr(false),
		},
		{
			Name:                 utils.Ptr("new_repo"),
			URL:                  utils.Ptr("https://new_repo_url.org"),
			OrgID:                &orgID,
			AccountID:            &accountID,
			DistributionVersions: &[]string{"any"},
			DistributionArch:     utils.Ptr("any"),
			GpgKey:               utils.Ptr(""),
			MetadataVerification: utils.Ptr(false),
			ModuleHotfixes:       utils.Ptr(false),
			Snapshot:             utils.Ptr(false),
		},
	}

	rr, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).BulkImport(context.Background(), requests)

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

func (suite *RepositoryConfigSuite) TestBulkExport() {
	t := suite.T()
	tx := suite.tx

	orgID := orgIDTest
	accountID := accountIdTest
	seedSize := 5
	var repoConfigs []models.RepositoryConfiguration
	var total int64
	var err error

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, seedSize, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	result := suite.tx.
		Preload("Repository").
		Where("org_id = ?", orgID).
		Order("repository_configurations.name asc").
		Find(&repoConfigs).
		Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(seedSize), total)

	var repoUuids []string
	for i := 0; i < seedSize; i++ {
		repoUuids = append(repoUuids, repoConfigs[i].UUID)
	}
	request := api.RepositoryExportRequest{
		RepositoryUuids: repoUuids,
	}
	uploadRepoName := "yyy_upload_repo_for_export"
	importRequest := api.RepositoryRequest{
		Name:                 utils.Ptr(uploadRepoName),
		OrgID:                &orgID,
		Origin:               utils.Ptr(config.OriginUpload),
		AccountID:            &accountID,
		DistributionVersions: &[]string{"any"},
		DistributionArch:     utils.Ptr("any"),
		GpgKey:               utils.Ptr(""),
		MetadataVerification: utils.Ptr(false),
		ModuleHotfixes:       utils.Ptr(false),
		Snapshot:             utils.Ptr(true),
	}
	rr, errs := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).BulkImport(context.Background(), []api.RepositoryRequest{importRequest})
	assert.Empty(t, errs)
	request.RepositoryUuids = append(request.RepositoryUuids, rr[0].UUID)

	response, err := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).BulkExport(context.Background(), orgID, request)
	assert.Empty(t, err)

	assert.Equal(t, seedSize+1, len(response))

	// Verify upload repo is found or other repos are found
	for _, resp := range response {
		if resp.Origin == config.OriginUpload {
			assert.Equal(t, uploadRepoName, resp.Name)
		} else {
			found := false
			for _, rc := range repoConfigs {
				if rc.Repository.URL == resp.URL {
					assert.Equal(t, rc.Name, resp.Name)
					found = true
					break
				}
			}
			assert.True(t, found)
		}
	}
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

	createResp, err := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).Create(context.Background(), api.RepositoryRequest{
		Name:  utils.Ptr("NotUpdated"),
		URL:   &url,
		OrgID: utils.Ptr("MyGreatOrg"),
	})
	assert.Nil(t, err)

	_, err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).Update(context.Background(), createResp.OrgID, createResp.UUID,
		api.RepositoryUpdateRequest{
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

	createResp, err := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).Create(context.Background(), api.RepositoryRequest{
		Name:                 utils.Ptr("NotUpdated"),
		URL:                  utils.Ptr("http://example.com/testupdateattributes"),
		OrgID:                utils.Ptr("MyGreatOrg"),
		ModuleHotfixes:       utils.Ptr(false),
		MetadataVerification: utils.Ptr(false),
	})
	assert.Nil(t, err)

	_, err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).Update(context.Background(), createResp.OrgID, createResp.UUID,
		api.RepositoryUpdateRequest{
			ModuleHotfixes:       utils.Ptr(true),
			MetadataVerification: utils.Ptr(true),
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

	_, err := seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{})
	duplicateVersions := []string{config.El7, config.El7}

	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	suite.tx.First(&found)
	_, err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).Update(context.Background(), found.OrgID, found.UUID,
		api.RepositoryUpdateRequest{
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
	_, err = GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).Update(context.Background(), found.OrgID, found.UUID,
		api.RepositoryUpdateRequest{
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
	tx := suite.tx

	var err error
	name := "testduplicateupdate - repository"
	url := "https://testduplicate.com"

	repo := repoPublicTest.DeepCopy()
	repoConfig := repoConfigTest1.DeepCopy()
	var created1 api.RepositoryResponse
	var created2 api.RepositoryResponse

	created1, err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).
		Create(context.Background(), api.RepositoryRequest{
			OrgID:     &repoConfig.OrgID,
			AccountID: &repoConfig.AccountID,
			Name:      &repoConfig.Name,
			URL:       &repo.URL,
		})
	assert.NoError(t, err)

	created2, err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).
		Create(context.Background(), api.RepositoryRequest{
			OrgID:     &created1.OrgID,
			AccountID: &created1.AccountID,
			Name:      &name,
			URL:       &url,
		})
	assert.NoError(t, err)

	_, err = GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).Update(
		context.Background(),
		created2.OrgID,
		created2.UUID,
		api.RepositoryUpdateRequest{
			Name: &created1.Name,
			URL:  utils.Ptr("https://testduplicate2.com"),
		})
	assert.Error(t, err)

	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.AlreadyExists)
}

func (suite *RepositoryConfigSuite) TestUpdateNotFound() {
	name := "unique"
	t := suite.T()
	orgID := seeds.RandomOrgId()
	var err error

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	err = suite.tx.
		First(&found, "org_id = ?", orgID).
		Error
	require.NoError(t, err)

	_, err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).Update(context.Background(), "Wrong OrgID!! zomg hacker", found.UUID,
		api.RepositoryUpdateRequest{
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
			expected: "URL cannot be blank for custom and Red Hat repositories.",
		},
	}
	tx.SavePoint("updateblanktest")
	for i := 0; i < len(blankItems); i++ {
		_, err := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).Update(context.Background(), orgID, found.UUID, blankItems[i].given.ToRepositoryUpdateRequest())
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

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	err = tx.
		Preload("Repository").
		First(&found, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)

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
	assert.Equal(t, found.UUID, fetched.LastSnapshot.RepositoryUUID)
	assert.Equal(t, found.Name, fetched.LastSnapshot.RepositoryName)

	if config.Get().Features.Snapshots.Enabled {
		assert.Equal(t, testContentPath+"/", fetched.LastSnapshot.URL)
		assert.Equal(t, testContentPath+"/"+fetched.UUID+"/latest/", fetched.LatestSnapshotURL)
	}
}

func (suite *RepositoryConfigSuite) TestFetchByRepo() {
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()
	var err error

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	err = tx.
		Preload("Repository").
		First(&found, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)

	fetched, err := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).FetchByRepoUuid(context.Background(), found.OrgID, found.RepositoryUUID)
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

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	err = tx.
		First(&found, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)

	fetched, err := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).FetchWithoutOrgID(context.Background(), found.UUID, false)
	assert.Nil(t, err)
	assert.Equal(t, found.UUID, fetched.UUID)
	assert.Equal(t, found.Name, fetched.Name)
}

func (suite *RepositoryConfigSuite) TestFetchNotFound() {
	t := suite.T()
	orgID := seeds.RandomOrgId()
	var err error

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)
	found := models.RepositoryConfiguration{}
	err = suite.tx.
		First(&found, "org_id = ?", orgID).
		Error
	assert.NoError(t, err)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)

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

	results := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).InternalOnly_FetchRepoConfigsForRepoUUID(context.Background(), repoConfig.RepositoryUUID)

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
		Origin:  originCustom,
	}
	var err error

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
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

	rDao := repositoryConfigDaoImpl{db: suite.tx, pulpClient: suite.mockPulpClient, fsClient: suite.mockFsClient}
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)
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
		assert.Equal(t, repoConfig.UUID, response.Data[0].LastSnapshot.RepositoryUUID)
		assert.Equal(t, repoConfig.Name, response.Data[0].LastSnapshot.RepositoryName)
		if config.Get().Features.Snapshots.Enabled {
			assert.Equal(t, testContentPath+"/", response.Data[0].LastSnapshot.URL)
			assert.Equal(t, testContentPath+"/"+response.Data[0].UUID+"/latest/", response.Data[0].LatestSnapshotURL)
		}
	}
}

func (suite *RepositoryConfigSuite) TestListErrorGettingEntitledFeatures() {
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
		Origin:  originCustom,
	}
	var err error

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	result := suite.tx.
		Preload("Repository").
		Where("org_id = ?", orgID).
		Find(&repoConfig).
		Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(1), total)

	rDao := repositoryConfigDaoImpl{db: suite.tx, pulpClient: suite.mockPulpClient, fsClient: suite.mockFsClient}
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, errors.New("error getting entitled features"))
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
		Origin:  originCustom,
	}
	var err error

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	result := suite.tx.
		Preload("Repository").
		Where("org_id = ?", orgID).
		Find(&repoConfig).
		Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(1), total)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)
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
		Origin:  originCustom,
	}

	result := suite.tx.Where("org_id = ?", orgID).Find(&repoConfigs).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(0), total)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)

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
		Origin:  originCustom,
	}

	var total int64
	var err error

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, 20, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	result := suite.tx.Where("org_id = ?", orgID).Find(&repoConfigs).Count(&total)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(20), total)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)

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
	filterData := api.FilterData{Origin: originCustom}

	_, err := seeds.SeedRepositoryConfigurations(suite.tx, 2, seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El9}})
	assert.Nil(t, err)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	suite.mockPulpForListOrFetch(1)
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)

	allRepoResp, _, err := repoConfigDao.List(context.Background(), orgID, api.PaginationData{Limit: -1}, filterData)
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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	suite.mockPulpForListOrFetch(4)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)

	filterData := api.FilterData{Origin: originCustom}

	_, err := seeds.SeedRepositoryConfigurations(suite.tx, 3, seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El9}})
	assert.Nil(t, err)
	allRepoResp, _, err := repoConfigDao.List(context.Background(), orgID, api.PaginationData{Limit: -1}, filterData)
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

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	suite.mockPulpForListOrFetch(3)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)

	filterData := api.FilterData{Origin: originCustom}

	_, err := seeds.SeedRepositoryConfigurations(suite.tx, 3, seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El9}})
	assert.Nil(t, err)
	allRepoResp, _, err := repoConfigDao.List(context.Background(), orgID, api.PaginationData{Limit: -1}, filterData)
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
		Origin:  originCustom,
	}

	var total int64
	quantity := 20

	_, err := seeds.SeedRepositoryConfigurations(suite.tx, quantity, seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El9}})
	assert.Nil(t, err)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)

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
		Origin:  originCustom,
	}

	var total int64

	quantity := 20
	_, err := seeds.SeedRepositoryConfigurations(tx, quantity, seeds.SeedOptions{OrgID: orgID, Arch: &filterData.Arch})
	assert.Nil(t, err)

	result := tx.
		Where("org_id = ? AND arch = ?", orgID, filterData.Arch).
		Find(&repoConfigs).
		Count(&total)

	assert.Nil(t, result.Error)
	assert.Equal(t, int64(quantity), total)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)

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
	_, err := seeds.SeedRepositoryConfigurations(tx, quantity, seeds.SeedOptions{OrgID: orgID, Origin: &filterData.Origin})
	assert.Nil(t, err)
	_, err = seeds.SeedRepositoryConfigurations(tx, quantity, seeds.SeedOptions{OrgID: orgID, Origin: utils.Ptr("SomeOther")})
	assert.Nil(t, err)

	result := tx.Joins("inner join repositories on repositories.uuid = repository_configurations.repository_uuid").
		Where("org_id = ? AND repositories.origin = ?", orgID, filterData.Origin).
		Find(&repoConfigs).
		Count(&total)

	assert.Nil(t, result.Error)
	assert.Equal(t, int64(quantity), total)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	suite.mockPulpForListOrFetch(2)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)

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
		Origin:      originCustom,
	}

	var total int64

	quantity := 20
	_, err := seeds.SeedRepositoryConfigurations(tx, quantity, seeds.SeedOptions{OrgID: orgID, ContentType: &filterData.ContentType})
	assert.Nil(t, err)
	_, err = seeds.SeedRepositoryConfigurations(tx, quantity, seeds.SeedOptions{OrgID: orgID, ContentType: utils.Ptr("SomeOther")})
	assert.Nil(t, err)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)

	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)
	response, total, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(quantity), total)
}

func (suite *RepositoryConfigSuite) TestListFilterExtendedRelease() {
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()

	repoEUS := models.Repository{URL: "https://eus.example.com"}
	err := tx.Create(&repoEUS).Error
	require.NoError(t, err)

	repoE4S := models.Repository{URL: "https://e4s.example.com"}
	err = tx.Create(&repoE4S).Error
	require.NoError(t, err)

	repoRegular := models.Repository{URL: "https://regular.example.com"}
	err = tx.Create(&repoRegular).Error
	require.NoError(t, err)

	repoEUS2 := models.Repository{URL: "https://eus2.example.com"}
	err = tx.Create(&repoEUS2).Error
	require.NoError(t, err)

	err = tx.Create(&models.RepositoryConfiguration{
		Name:                   "EUS Repo 9.4 x86_64",
		OrgID:                  orgID,
		RepositoryUUID:         repoEUS.UUID,
		Arch:                   config.X8664,
		Versions:               pq.StringArray{config.El9},
		ExtendedRelease:        "eus",
		ExtendedReleaseVersion: "9.4",
		FeatureName:            "RHEL-EUS-x86_64",
	}).Error
	require.NoError(t, err)

	err = tx.Create(&models.RepositoryConfiguration{
		Name:                   "E4S Repo 9.4",
		OrgID:                  orgID,
		RepositoryUUID:         repoE4S.UUID,
		Arch:                   config.X8664,
		Versions:               pq.StringArray{config.El9},
		ExtendedRelease:        "e4s",
		ExtendedReleaseVersion: "9.4",
		FeatureName:            "RHEL-E4S-x86_64",
	}).Error
	require.NoError(t, err)

	err = tx.Create(&models.RepositoryConfiguration{
		Name:           "Regular Repo",
		OrgID:          orgID,
		RepositoryUUID: repoRegular.UUID,
		Arch:           config.X8664,
		Versions:       pq.StringArray{config.El9},
		FeatureName:    "RHEL-OS-x86_64",
	}).Error
	require.NoError(t, err)

	err = tx.Create(&models.RepositoryConfiguration{
		Name:                   "EUS Repo 9.6 x86_64",
		OrgID:                  orgID,
		RepositoryUUID:         repoEUS2.UUID,
		Arch:                   config.X8664,
		Versions:               pq.StringArray{config.El9},
		ExtendedRelease:        "eus",
		ExtendedReleaseVersion: "9.6",
		FeatureName:            "RHEL-EUS-x86_64",
	}).Error
	require.NoError(t, err)

	repoConfigDao := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient)
	pageData := api.PaginationData{Limit: 20, Offset: 0}

	// Test 1: Filter by extended_release=eus
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64", "RHEL-EUS-x86_64", "RHEL-E4S-x86_64"}, nil).Once()
	filterData := api.FilterData{ExtendedRelease: "eus", Origin: config.OriginExternal}
	response, total, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Equal(t, 2, len(response.Data))
	for _, repo := range response.Data {
		assert.Equal(t, "eus", repo.ExtendedRelease)
		assert.Equal(t, "RHEL-EUS-x86_64", repo.FeatureName)
	}

	// Test 2: Filter by extended_release_version=9.4
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64", "RHEL-EUS-x86_64", "RHEL-E4S-x86_64"}, nil).Once()
	filterData = api.FilterData{ExtendedReleaseVersion: "9.4", Origin: config.OriginExternal}
	response, total, err = repoConfigDao.List(context.Background(), orgID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Equal(t, 2, len(response.Data))
	for _, repo := range response.Data {
		assert.Equal(t, "9.4", repo.ExtendedReleaseVersion)
	}

	// Test 3: Filter by both extended_release=eus AND extended_release_version=9.4
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64", "RHEL-EUS-x86_64", "RHEL-E4S-x86_64"}, nil).Once()
	filterData = api.FilterData{ExtendedRelease: "eus", ExtendedReleaseVersion: "9.4", Origin: config.OriginExternal}
	response, total, err = repoConfigDao.List(context.Background(), orgID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, "EUS Repo 9.4 x86_64", response.Data[0].Name)

	// Test 4: Filter by extended_release=eus with NO entitlements
	// Should return no feature-gated repos
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{}, nil).Once()
	filterData = api.FilterData{ExtendedRelease: "eus", Origin: config.OriginExternal}
	response, total, err = repoConfigDao.List(context.Background(), orgID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, 0, len(response.Data))

	// Test 5: Filter by comma-separated extended_release_version=9.4,9.6
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64", "RHEL-EUS-x86_64", "RHEL-E4S-x86_64"}, nil).Once()
	filterData = api.FilterData{ExtendedReleaseVersion: "9.4,9.6", Origin: config.OriginExternal}
	response, total, err = repoConfigDao.List(context.Background(), orgID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Equal(t, 3, len(response.Data))

	// Test 6: Filter by comma-separated extended_release=eus,e4s
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64", "RHEL-EUS-x86_64", "RHEL-E4S-x86_64"}, nil).Once()
	filterData = api.FilterData{ExtendedRelease: "eus,e4s", Origin: config.OriginExternal}
	response, total, err = repoConfigDao.List(context.Background(), orgID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Equal(t, 3, len(response.Data))

	// Test 7: Filter by extended_release=none
	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64", "RHEL-EUS-x86_64", "RHEL-E4S-x86_64"}, nil).Once()
	filterData = api.FilterData{ExtendedRelease: "none", Origin: config.OriginExternal}
	response, total, err = repoConfigDao.List(context.Background(), orgID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, "Regular Repo", response.Data[0].Name)
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
		Origin: originCustom,
	}

	statuses := [4]string{
		config.StatusValid,
		config.StatusPending,
		config.StatusUnavailable,
		config.StatusInvalid,
	}

	quantity := 40

	_, err := seeds.SeedTasks(suite.tx, 20, seeds.TaskSeedOptions{
		OrgID: orgID, Typename: "snapshot", Status: config.TaskStatusCompleted,
	})
	assert.Nil(t, err)
	_, err = seeds.SeedTasks(suite.tx, 20, seeds.TaskSeedOptions{
		OrgID: orgID, Typename: "add-uploads-repository", Status: config.TaskStatusCompleted,
	})
	assert.Nil(t, err)

	tasks := []models.TaskInfo{}
	result := suite.tx.
		Where("org_id = ?", orgID).
		Order("started_at DESC").
		Find(&tasks)
	assert.Nil(t, result.Error)

	for i := 0; i < 4; i++ {
		_, err := seeds.SeedRepositoryConfigurations(suite.tx, quantity/4,
			seeds.SeedOptions{OrgID: orgID, Status: &statuses[i], TaskID: tasks[i].Id.String()})
		assert.Nil(t, err)
	}

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)

	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)
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
		Origin:  originCustom,
	}

	quantity := 20

	x86ref := "x86_64"
	s390xref := "s390x"

	_, err := seeds.SeedRepositoryConfigurations(suite.tx, 10, seeds.SeedOptions{OrgID: orgID, Arch: &s390xref})
	assert.Nil(t, err)
	_, err = seeds.SeedRepositoryConfigurations(suite.tx, 30, seeds.SeedOptions{OrgID: orgID, Arch: &x86ref})
	assert.Nil(t, err)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)

	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)
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
		Origin:  originCustom,
	}

	quantity := 20

	_, err := seeds.SeedRepositoryConfigurations(suite.tx, quantity/4,
		seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El7, config.El8, config.El9, config.El10}})
	assert.Nil(t, err)

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, quantity/4,
		seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El7}})
	assert.Nil(t, err)

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, quantity/4,
		seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El8, config.El9, config.El10}})
	assert.Nil(t, err)

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, quantity/4,
		seeds.SeedOptions{OrgID: orgID, Versions: &[]string{config.El9, config.El10}})
	assert.Nil(t, err)

	// Seed data to a 2nd org to verify no crossover
	_, err = seeds.SeedRepositoryConfigurations(suite.tx, quantity,
		seeds.SeedOptions{OrgID: "kdksfkdf", Versions: &[]string{config.El7, config.El8, config.El9, config.El10}})
	assert.Nil(t, err)

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)

	suite.mockPulpForListOrFetch(1)
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)
	response, count, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, quantity, len(response.Data))
	assert.Equal(t, int64(quantity), count)

	// By setting the above seed values and SortBy to "version asc", we expect the first page to
	// contain 5 versions of length 1, 5 versions of length 4, 5 versions of length 3, and 5 versions of length 2:
	// [7], [7, 8, 9, 10], [8, 9, 10], [9, 10]
	firstItem := len(response.Data[0].DistributionVersions)
	secondItem := len(response.Data[5].DistributionVersions)
	thirdItem := len(response.Data[10].DistributionVersions)
	lastItem := len(response.Data[len(response.Data)-1].DistributionVersions)

	assert.Equal(t, firstItem, 1)
	assert.Equal(t, secondItem, 4)
	assert.Equal(t, thirdItem, 3)
	assert.Equal(t, lastItem, 2)
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
		Origin:  originCustom,
	}

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)

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
	suite.mockFsClient.Mock.On("GetEntitledFeatures", context.Background(), orgID).Return([]string{"RHEL-OS-x86_64"}, nil)
	response, total, err := repoConfigDao.List(context.Background(), orgID, pageData, filterData)

	assert.Nil(t, err)
	assert.Equal(t, int(quantity), len(response.Data))
	assert.Equal(t, quantity, total)
}

func (suite *RepositoryConfigSuite) TestListReposWithOutdatedSnaps() {
	t := suite.T()
	tx := suite.tx

	repoConfigDao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	initResponse, err := repoConfigDao.ListReposWithOutdatedSnapshots(context.Background(), 90)
	assert.Nil(t, err)

	repos, err := seeds.SeedRepositoryConfigurations(tx, 3, seeds.SeedOptions{
		OrgID: orgIDTest,
	})
	assert.Nil(t, err)

	r1, r2, r3 := repos[0], repos[1], repos[2]
	_ = suite.createSnapshotAtSpecifiedTime(r1, time.Now().Add(-2*time.Hour))
	_ = suite.createSnapshotAtSpecifiedTime(r1, time.Now().Add(-1*time.Hour))

	_ = suite.createSnapshotAtSpecifiedTime(r2, time.Now().Add(-100*24*time.Hour))
	_ = suite.createSnapshotAtSpecifiedTime(r2, time.Now().Add(-2*time.Hour))

	_ = suite.createSnapshotAtSpecifiedTime(r3, time.Now().Add(-101*24*time.Hour))
	_ = suite.createSnapshotAtSpecifiedTime(r3, time.Now().Add(-100*24*time.Hour))

	response, err := repoConfigDao.ListReposWithOutdatedSnapshots(context.Background(), 90)
	assert.Nil(t, err)
	assert.Len(t, response, len(initResponse)+2)
	assert.NotEqual(t, -1, slices.IndexFunc(response, func(rc models.RepositoryConfiguration) bool {
		return rc.UUID == r2.UUID
	}))
	assert.NotEqual(t, -1, slices.IndexFunc(response, func(rc models.RepositoryConfiguration) bool {
		return rc.UUID == r3.UUID
	}))
}

func (suite *RepositoryConfigSuite) TestSavePublicUrls() {
	t := suite.T()
	tx := suite.tx
	var count int64
	repoUrls := []string{
		"https://somepublicRepo.example.com/",
		"https://anotherpublicRepo.example.com",
	}

	repoUrlsCleaned := []string{
		"https://somepublicRepo.example.com/",
		"https://anotherpublicRepo.example.com/",
	}

	// Create the two Repository records
	err := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).SavePublicRepos(context.Background(), repoUrls)
	require.NoError(t, err)
	repos := []models.Repository{}
	err = tx.
		Model(&models.Repository{}).
		Where("url in (?)", repoUrlsCleaned).
		Count(&count).
		Find(&repos).
		Error
	require.NoError(t, err)
	assert.Equal(t, int64(len(repos)), count)

	// Repeat to check clause on conflict
	err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).SavePublicRepos(context.Background(), repoUrls)
	assert.NoError(t, err)
	err = tx.
		Model(&models.Repository{}).
		Where("url in (?)", repoUrlsCleaned).
		Count(&count).
		Find(&repos).
		Error
	require.NoError(t, err)
	assert.Equal(t, int64(len(repos)), count)

	// Remove one url and check that it was changed back to false
	noLongerPublic := repoUrlsCleaned[0]
	repoUrls = repoUrls[1:2] // remove the first item
	err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).SavePublicRepos(context.Background(), repoUrls)
	assert.NoError(t, err)

	repo := models.Repository{}
	err = tx.Model(&models.Repository{}).Where("url = ?", noLongerPublic).Find(&repo).Error
	require.NoError(t, err)
	assert.False(t, repo.Public)

	repo = models.Repository{}
	err = tx.Model(&models.Repository{}).Where("url = ?", repoUrlsCleaned[1]).Find(&repo).Error
	require.NoError(t, err)
	assert.True(t, repo.Public)
}

func (suite *RepositoryConfigSuite) TestDelete() {
	t := suite.T()
	tx := suite.tx
	orgID := seeds.RandomOrgId()
	var err error

	_, err = seeds.SeedRepositoryConfigurations(tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	repoConfig := models.RepositoryConfiguration{}
	err = tx.
		First(&repoConfig, "org_id = ?", orgID).
		Error
	require.NoError(t, err)

	err = GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient).SoftDelete(context.Background(), repoConfig.OrgID, repoConfig.UUID)
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

	_, err = seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	found := models.RepositoryConfiguration{}
	err = suite.tx.
		First(&found, "org_id = ?", orgID).
		Error
	require.NoError(t, err)

	err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).SoftDelete(context.Background(), "bad org id", found.UUID)
	assert.Error(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	err = GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient).Delete(context.Background(), "bad org id", found.UUID)
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
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	orgID := seeds.RandomOrgId()
	repoConfigCount := 5

	_, err := seeds.SeedRepositoryConfigurations(suite.tx, repoConfigCount, seeds.SeedOptions{OrgID: orgID})
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
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	orgID := seeds.RandomOrgId()
	repoConfigCount := 1

	_, err := seeds.SeedRepositoryConfigurations(suite.tx, repoConfigCount, seeds.SeedOptions{OrgID: orgID})
	assert.Nil(t, err)

	var uuids []string
	err = suite.tx.Model(models.RepositoryConfiguration{}).Where("org_id = ?", orgID).Select("repository_uuid").Find(&uuids).Error
	assert.NoError(t, err)
	assert.Len(t, uuids, repoConfigCount)

	tasks, err := seeds.SeedTasks(suite.tx, 1, seeds.TaskSeedOptions{Status: "finished"})
	require.NoError(t, err)

	taskUUID := tasks[0].Id.String()

	err = dao.UpdateLastSnapshotTask(context.Background(), taskUUID, orgID, uuids[0])
	assert.Nil(t, err)

	var found []models.RepositoryConfiguration
	err = suite.tx.Where("org_id = ?", orgID).Find(&found).Error
	assert.NoError(t, err)
	assert.Equal(t, taskUUID, found[0].LastSnapshotTaskUUID)
}

func (suite *RepositoryConfigSuite) TestBulkDeleteOneNotFound() {
	t := suite.T()
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	orgID := seeds.RandomOrgId()
	repoConfigCount := 5

	_, err := seeds.SeedRepositoryConfigurations(suite.tx, repoConfigCount, seeds.SeedOptions{OrgID: orgID})
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
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	orgID := config.RedHatOrg
	repoConfigCount := 5
	existingRepoConfigCount := int64(0)

	suite.tx.Model(models.RepositoryConfiguration{}).Where("org_id = ?", config.RedHatOrg).Count(&existingRepoConfigCount)

	_, err := seeds.SeedRepositoryConfigurations(suite.tx, repoConfigCount, seeds.SeedOptions{OrgID: orgID})
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
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	orgID := seeds.RandomOrgId()
	repoConfigCount := 5

	_, err := seeds.SeedRepositoryConfigurations(suite.tx, repoConfigCount, seeds.SeedOptions{OrgID: orgID})
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

	mockYumRepo.On("Configure", mock.AnythingOfType("yum.YummySettings"))
	mockYumRepo.Mock.On("Repomd", context.Background()).Return(&yum.Repomd{}, 200, nil)
	mockYumRepo.Mock.On("Signature", context.Background()).Return(test.RepomdSignature(), 200, nil)
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
		Name: utils.Ptr(""),
		URL:  utils.Ptr(""),
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
		Name: utils.Ptr("Some Other Name"),
		URL:  utils.Ptr("http://example.com/"),
	}

	mockYumRepo.On("Configure", mock.AnythingOfType("yum.YummySettings"))
	mockYumRepo.Mock.On("Repomd", context.Background()).Return(&yum.Repomd{}, 200, nil)
	mockYumRepo.Mock.On("Signature", context.Background()).Return(test.RepomdSignature(), 200, nil)

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
		UUID: utils.Ptr("not.a.real.UUID"),
		Name: utils.Ptr("Some bad repo!"),
		URL:  utils.Ptr("http://badrepo.example.com/"),
	}

	mockYumRepo.On("Configure", mock.AnythingOfType("yum.YummySettings"))
	mockYumRepo.Mock.On("Repomd", context.Background()).Return(nil, 404, nil)

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
		Name: utils.Ptr("Somebadrepo!"),
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
		Name: utils.Ptr("Some Timeout repo!"),
		URL:  utils.Ptr("http://timeout.example.com"),
	}

	timeoutErr := MockTimeoutError{
		Message: " (Client.Timeout exceeded while awaiting headers)",
		Timeout: true,
	}

	mockYumRepo.On("Configure", mock.AnythingOfType("yum.YummySettings"))
	mockYumRepo.Mock.On("Repomd", context.Background()).Return(nil, 0, timeoutErr)

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
		Name:                 utils.Ptr("Good Gpg"),
		URL:                  utils.Ptr("http://goodgpg.example.com/"),
		GPGKey:               test.GpgKey(),
		MetadataVerification: true,
	}

	mockYumRepo.On("Configure", yum.YummySettings{Client: http.DefaultClient, URL: parameters.URL})
	mockYumRepo.Mock.On("Repomd", context.Background()).Return(test.Repomd, 200, nil)
	mockYumRepo.Mock.On("Signature", context.Background()).Return(test.RepomdSignature(), 200, nil)

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
		Name:                 utils.Ptr("Good Gpg"),
		URL:                  utils.Ptr("http://badsig.example.com/"),
		GPGKey:               test.GpgKey(),
		MetadataVerification: true,
	}

	badRepomdXml := *test.Repomd.RepomdString + "<BadXML>"
	badRepomd := yum.Repomd{RepomdString: &badRepomdXml}
	mockYumRepo.On("Configure", mock.AnythingOfType("yum.YummySettings"))
	mockYumRepo.Mock.On("Repomd", context.Background()).Return(&badRepomd, 200, nil)
	mockYumRepo.Mock.On("Signature", context.Background()).Return(test.RepomdSignature(), 200, nil)

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
		Name:                 utils.Ptr("Good Gpg"),
		URL:                  utils.Ptr("http://badsig.example.com/"),
		GPGKey:               utils.Ptr("Not a real key"),
		MetadataVerification: true,
	}

	mockYumRepo.On("Configure", mock.AnythingOfType("yum.YummySettings"))
	mockYumRepo.Mock.On("Repomd", context.Background()).Return(test.Repomd, 200, nil)
	mockYumRepo.Mock.On("Signature", context.Background()).Return(test.RepomdSignature(), 200, nil)

	response, err := dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)
	assert.False(t, response.GPGKey.Valid)
	assert.True(t, response.URL.MetadataSignaturePresent)
	assert.True(t, response.URL.Valid)
}

func (suite *RepositoryConfigSuite) TestValidateParametersInvalidCharacters() {
	t := suite.T()
	orgID := seeds.RandomOrgId()
	mockYumRepo := yum.MockYumRepository{}
	dao := repositoryConfigDaoImpl{db: suite.tx, yumRepo: &mockYumRepo}
	repoConfigs, err := seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgID})
	assert.NoError(t, err)
	repoConfig := repoConfigs[0]

	mockYumRepo.On("Configure", mock.AnythingOfType("yum.YummySettings"))
	mockYumRepo.Mock.On("Repomd", context.Background()).Return(&yum.Repomd{}, 200, nil)
	mockYumRepo.Mock.On("Signature", context.Background()).Return(test.RepomdSignature(), 200, nil)

	parameters := api.RepositoryValidationRequest{
		Name: utils.Ptr("\u0000"),
	}
	suite.tx.SavePoint("before")
	_, err = dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
	assert.Error(t, err)
	if err != nil {
		daoError, ok := err.(*ce.DaoError)
		assert.True(t, ok)
		if ok {
			assert.True(t, daoError.BadValidation)
		}
	}
	suite.tx.RollbackTo("before")

	parameters = api.RepositoryValidationRequest{
		URL: utils.Ptr("\u0000"),
	}
	suite.tx.SavePoint("before")
	_, err = dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
	assert.Error(t, err)
	if err != nil {
		daoError, ok := err.(*ce.DaoError)
		assert.True(t, ok)
		if ok {
			assert.True(t, daoError.BadValidation)
		}
	}
	suite.tx.RollbackTo("before")

	parameters = api.RepositoryValidationRequest{
		GPGKey: utils.Ptr("\u0000"),
		URL:    utils.Ptr("http://example.com/"),
	}

	_, err = dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)

	parameters = api.RepositoryValidationRequest{
		UUID: utils.Ptr("\u0000"),
	}
	_, err = dao.ValidateParameters(context.Background(), repoConfig.OrgID, parameters, []string{})
	assert.NoError(t, err)
}

func (suite *RepositoryConfigSuite) setupValidationTest() (*yum.MockYumRepository, repositoryConfigDaoImpl, models.RepositoryConfiguration) {
	t := suite.T()
	orgId := seeds.RandomOrgId()
	_, err := seeds.SeedRepositoryConfigurations(suite.tx, 1, seeds.SeedOptions{OrgID: orgId})
	assert.NoError(t, err)

	mockYumRepo := yum.MockYumRepository{}
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

func (suite *RepositoryConfigSuite) TestListReposToSnapshot() {
	defer func() {
		config.Get().Options.AlwaysRunCronTasks = false
	}()

	t := suite.T()
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)

	externalRepo, err := dao.Create(context.Background(), api.RepositoryRequest{
		Name:                 utils.Ptr("external-repo"),
		URL:                  utils.Ptr("http://external.example.com/"),
		OrgID:                utils.Ptr("123"),
		AccountID:            utils.Ptr("123"),
		DistributionArch:     utils.Ptr("x86_64"),
		DistributionVersions: &[]string{config.El9},
		Snapshot:             utils.Ptr(true),
		Origin:               utils.Ptr(config.OriginExternal),
	})
	assert.NoError(t, err)

	rhRepo1 := suite.createTestRedHatRepository(api.RepositoryRequest{
		Name:                 utils.Ptr("rh-repo-1"),
		URL:                  utils.Ptr("http://redhat1.example.com/"),
		OrgID:                utils.Ptr(config.RedHatOrg),
		AccountID:            utils.Ptr("123"),
		DistributionArch:     utils.Ptr("x86_64"),
		DistributionVersions: &[]string{config.El9},
		Snapshot:             utils.Ptr(true),
		Origin:               utils.Ptr(config.OriginRedHat),
	})

	rhRepo2 := suite.createTestRedHatRepository(api.RepositoryRequest{
		Name:                 utils.Ptr("rh-repo-2"),
		URL:                  utils.Ptr("http://redhat2.example.com/"),
		OrgID:                utils.Ptr(config.RedHatOrg),
		AccountID:            utils.Ptr("123"),
		DistributionArch:     utils.Ptr("x86_64"),
		DistributionVersions: &[]string{config.El9},
		Snapshot:             utils.Ptr(true),
		Origin:               utils.Ptr(config.OriginRedHat),
	})

	communityRepo := suite.createTestCommunityRepository(api.RepositoryRequest{
		Name:                 utils.Ptr("community-repo"),
		URL:                  utils.Ptr("http://community.example.com/"),
		OrgID:                utils.Ptr(config.CommunityOrg),
		AccountID:            utils.Ptr("123"),
		DistributionArch:     utils.Ptr("x86_64"),
		DistributionVersions: &[]string{config.El9},
		Snapshot:             utils.Ptr(true),
		Origin:               utils.Ptr(config.OriginCommunity),
	})
	assert.NoError(t, err)

	yesterday := time.Now().Add(time.Hour * time.Duration(-48))
	fortyFiveMinutesAgo := time.Now().Add(time.Minute * time.Duration(-46))

	testCases := []struct {
		Name                     string
		Repo                     api.RepositoryResponse
		Opts                     *seeds.TaskSeedOptions
		FailedSnapshotCount      int64
		Included                 bool
		Filter                   *ListRepoFilter
		OptionAlwaysRunCronTasks bool
	}{
		// Basic repo tests
		{
			Name:     "External repo never synced",
			Repo:     externalRepo,
			Opts:     nil,
			Included: true,
		},
		{
			Name:     "External repo with recent successful snapshot",
			Repo:     externalRepo,
			Opts:     &seeds.TaskSeedOptions{Status: config.TaskStatusCompleted},
			Included: false,
		},
		{
			Name:     "External repo with old successful snapshot",
			Repo:     externalRepo,
			Opts:     &seeds.TaskSeedOptions{Status: config.TaskStatusCompleted, QueuedAt: &yesterday},
			Included: true,
		},
		{
			Name:     "External repo with running snapshot",
			Repo:     externalRepo,
			Opts:     &seeds.TaskSeedOptions{Status: config.TaskStatusRunning},
			Included: false,
		},
		{
			Name:     "External repo with filter",
			Repo:     externalRepo,
			Opts:     &seeds.TaskSeedOptions{Status: config.TaskStatusCompleted, QueuedAt: &yesterday},
			Included: false,
			Filter:   &ListRepoFilter{URLs: &[]string{"http://not-found.example.com"}},
		},

		// Red Hat repos stale tests (45 minute interval)
		{
			Name:     "Red Hat repo with recent successful snapshot",
			Repo:     rhRepo1,
			Opts:     &seeds.TaskSeedOptions{Status: config.TaskStatusCompleted},
			Included: false,
			Filter:   &ListRepoFilter{URLs: &[]string{rhRepo1.URL}},
		},
		{
			Name:     "Red Hat repo with old successful snapshot",
			Repo:     rhRepo1,
			Opts:     &seeds.TaskSeedOptions{Status: config.TaskStatusCompleted, QueuedAt: &fortyFiveMinutesAgo},
			Included: true,
			Filter:   &ListRepoFilter{URLs: &[]string{rhRepo1.URL, rhRepo2.URL}},
		},

		// Failed repo tests - Red Hat and Community (always retry)
		{
			Name:                "Red Hat repo with recent failed snapshot",
			Repo:                rhRepo1,
			Opts:                &seeds.TaskSeedOptions{RepoConfigUUID: rhRepo1.UUID, Status: config.TaskStatusFailed},
			FailedSnapshotCount: config.FailedSnapshotLimit + 10, // Above limit but still should retry
			Included:            true,
			Filter:              &ListRepoFilter{URLs: &[]string{rhRepo1.URL}},
		},
		{
			Name:                "Community repo with recent failed snapshot",
			Repo:                communityRepo,
			Opts:                &seeds.TaskSeedOptions{RepoConfigUUID: communityRepo.UUID, Status: config.TaskStatusFailed},
			FailedSnapshotCount: config.FailedSnapshotLimit + 10, // Above limit but still should retry
			Included:            true,
			Filter:              &ListRepoFilter{URLs: &[]string{communityRepo.URL}},
		},

		// Failed repo tests - External (conditional retry)
		{
			Name:                "External repo with recent failed snapshot, below limit (no retry - too recent)",
			Repo:                externalRepo,
			Opts:                &seeds.TaskSeedOptions{RepoConfigUUID: externalRepo.UUID, Status: config.TaskStatusFailed},
			FailedSnapshotCount: config.FailedSnapshotLimit - 1,
			Included:            false,
		},
		{
			Name:                "External repo with old failed snapshot, below limit (retry)",
			Repo:                externalRepo,
			Opts:                &seeds.TaskSeedOptions{RepoConfigUUID: externalRepo.UUID, Status: config.TaskStatusFailed, QueuedAt: &yesterday},
			FailedSnapshotCount: config.FailedSnapshotLimit - 1,
			Included:            true,
		},
		{
			Name:                "External repo with old failed snapshot, above limit (no retry)",
			Repo:                externalRepo,
			Opts:                &seeds.TaskSeedOptions{RepoConfigUUID: externalRepo.UUID, Status: config.TaskStatusFailed, QueuedAt: &yesterday},
			FailedSnapshotCount: config.FailedSnapshotLimit + 1,
			Included:            false,
		},

		// Force snapshot test
		{
			Name:                     "External repo with recent successful snapshot but always run enabled",
			Repo:                     externalRepo,
			Opts:                     &seeds.TaskSeedOptions{RepoConfigUUID: externalRepo.UUID, Status: config.TaskStatusCompleted},
			Included:                 true,
			OptionAlwaysRunCronTasks: true,
		},
	}

	for _, testCase := range testCases {
		var testsAllRHReposReturned bool

		err = suite.tx.Where("uuid = ?", testCase.Repo.UUID).Model(&models.RepositoryConfiguration{}).UpdateColumn("failed_snapshot_count", testCase.FailedSnapshotCount).Error
		assert.NoError(t, err)

		if testCase.Opts != nil {
			testsAllRHReposReturned = testCase.Repo.UUID == rhRepo1.UUID && testCase.Opts.QueuedAt != nil && testCase.Opts.Status == config.TaskStatusCompleted

			taskSeedOptions := testCase.Opts
			taskSeedOptions.RepoConfigUUID = testCase.Repo.UUID
			taskSeedOptions.OrgID = testCase.Repo.OrgID

			tasks, err := seeds.SeedTasks(suite.tx, 1, *taskSeedOptions)
			assert.NoError(t, err)

			err = dao.UpdateLastSnapshotTask(context.Background(), tasks[0].Id.String(), testCase.Repo.OrgID, testCase.Repo.RepositoryUUID)
			assert.NoError(t, err)

			// When RedHatRepo1 is expected due to a stale snapshot, RedHatRepo2 must also be included even if not otherwise due
			if testsAllRHReposReturned {
				seedOptions := seeds.TaskSeedOptions{
					RepoConfigUUID: rhRepo2.UUID,
					OrgID:          rhRepo2.OrgID,
					Status:         config.TaskStatusCompleted,
				}

				rhRepo2Tasks, err := seeds.SeedTasks(suite.tx, 1, seedOptions)
				assert.NoError(t, err)

				err = dao.UpdateLastSnapshotTask(context.Background(), rhRepo2Tasks[0].Id.String(), rhRepo2.OrgID, rhRepo2.RepositoryUUID)
				assert.NoError(t, err)
			}
		}

		config.Get().Options.AlwaysRunCronTasks = testCase.OptionAlwaysRunCronTasks

		afterRepos, err := dao.InternalOnly_ListReposToSnapshot(context.Background(), testCase.Filter)
		assert.NoError(t, err)

		var found bool
		for i := range afterRepos {
			if testCase.Repo.UUID == afterRepos[i].UUID {
				found = true
				break
			}
		}
		assert.Equal(t, testCase.Included, found, "Test case %v, expected to be found: %v, but was: %v", testCase.Name, testCase.Included, found)

		// Verify that red hat repo 2 is also found
		if testsAllRHReposReturned {
			found = false
			for _, repo := range afterRepos {
				if rhRepo2.UUID == repo.UUID {
					found = true
					break
				}
			}
			assert.True(t, found, "RedHatRepo2 should also be included when RedHatRepo1 is expected due to stale snapshot")
		}
	}
}

func (suite *RepositoryConfigSuite) TestListReposToSnapshotExtraRepos() {
	// Delete all repo configs to prevent the minimum repo count from taking effect on random repos
	suite.tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.RepositoryConfiguration{})
	t := suite.T()
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)

	// Test with no repos
	afterRepos, err := dao.InternalOnly_ListReposToSnapshot(context.Background(), &ListRepoFilter{
		MinimumInterval: utils.Ptr(24),
	})
	assert.NoError(t, err)
	assert.Empty(t, afterRepos)

	rconfigs := []models.RepositoryConfiguration{}
	// Populate repo configs with tasks
	for i := 0; i < 49; i++ {
		repo := models.Repository{
			URL: "http://example.com/" + fmt.Sprintf("%v", i) + "/",
		}

		query := suite.tx.Create(&repo)
		assert.NoError(t, query.Error)

		task := models.TaskInfo{
			Typename: config.RepositorySnapshotTask,
			Id:       uuid.New(),
			Token:    uuid.New(),
			Queued:   nil,
			Started:  nil,
			Finished: nil,
			Status:   config.TaskStatusCompleted,
		}
		if i == 0 {
			// first task was 48 hours ago
			task.Finished = utils.Ptr(time.Now().Add(-48 * time.Hour))
		} else {
			task.Finished = utils.Ptr(time.Now().Add(time.Duration(i-49) * time.Minute))
		}
		task.Started = task.Finished
		task.Queued = task.Finished

		query = suite.tx.Create(&task)
		assert.NoError(t, query.Error)

		repoConfig := models.RepositoryConfiguration{
			Name:                 "test-list-repos" + fmt.Sprintf("%v", i),
			AccountID:            "someaccount",
			OrgID:                "someorg",
			RepositoryUUID:       repo.UUID,
			Snapshot:             true,
			LastSnapshotTaskUUID: task.Id.String(),
		}
		query = suite.tx.Create(&repoConfig)
		assert.NoError(t, query.Error)
		rconfigs = append(rconfigs, repoConfig)
	}

	afterRepos, err = dao.InternalOnly_ListReposToSnapshot(context.Background(), &ListRepoFilter{
		MinimumInterval: utils.Ptr(24),
	})
	assert.NoError(t, err)
	assert.Len(t, afterRepos, 3)
	assert.Equal(t, rconfigs[0].UUID, afterRepos[0].UUID)
	assert.Equal(t, rconfigs[1].UUID, afterRepos[1].UUID)
	assert.Equal(t, rconfigs[2].UUID, afterRepos[2].UUID)

	afterRepos, err = dao.InternalOnly_ListReposToSnapshot(context.Background(), &ListRepoFilter{
		MinimumInterval: utils.Ptr(49),
	})
	assert.NoError(t, err)
	assert.Len(t, afterRepos, 2)
	assert.Equal(t, rconfigs[0].UUID, afterRepos[0].UUID)
}

func (suite *RepositoryConfigSuite) TestFetchRepoUUIDsByURLs() {
	t := suite.T()
	tx := suite.tx
	dao := GetRepositoryConfigDao(tx, suite.mockPulpClient, suite.mockFsClient)
	orgID := seeds.RandomOrgId()

	// Create test repositories
	repo1, err := dao.Create(context.Background(), api.RepositoryRequest{
		Name:      utils.Ptr("repo1"),
		URL:       utils.Ptr("http://example.com/repo1/"),
		OrgID:     &orgID,
		AccountID: utils.Ptr("test-account"),
	})
	assert.NoError(t, err)

	repo2, err := dao.Create(context.Background(), api.RepositoryRequest{
		Name:      utils.Ptr("repo2"),
		URL:       utils.Ptr("http://example.com/repo2/"),
		OrgID:     &orgID,
		AccountID: utils.Ptr("test-account"),
	})
	assert.NoError(t, err)

	repo3, err := dao.Create(context.Background(), api.RepositoryRequest{
		Name:      utils.Ptr("repo3"),
		URL:       utils.Ptr("http://example.com/repo3/"),
		OrgID:     &orgID,
		AccountID: utils.Ptr("test-account"),
	})
	assert.NoError(t, err)

	// Test fetching all repos
	urls := []string{repo1.URL, repo2.URL, repo3.URL}
	uuids, err := dao.FetchRepoUUIDsByURLs(context.Background(), orgID, urls)
	assert.NoError(t, err)
	assert.Len(t, uuids, 3)
	assert.Contains(t, uuids, repo1.UUID)
	assert.Contains(t, uuids, repo2.UUID)
	assert.Contains(t, uuids, repo3.UUID)

	// Test fetching subset of repos
	urls = []string{repo1.URL, repo2.URL}
	uuids, err = dao.FetchRepoUUIDsByURLs(context.Background(), orgID, urls)
	assert.NoError(t, err)
	assert.Len(t, uuids, 2)
	assert.Contains(t, uuids, repo1.UUID)
	assert.Contains(t, uuids, repo2.UUID)

	// Test with URLs without trailing slashes (should be cleaned up)
	urls = []string{"http://example.com/repo1", "http://example.com/repo2"}
	uuids, err = dao.FetchRepoUUIDsByURLs(context.Background(), orgID, urls)
	assert.NoError(t, err)
	assert.Len(t, uuids, 2)
	assert.Contains(t, uuids, repo1.UUID)
	assert.Contains(t, uuids, repo2.UUID)

	// Test with non-existent URL
	urls = []string{"http://example.com/nonexistent/"}
	_, err = dao.FetchRepoUUIDsByURLs(context.Background(), orgID, urls)
	assert.Error(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	// Test with mix of found and not found URLs
	urls = []string{repo1.URL, "http://example.com/nonexistent/"}
	_, err = dao.FetchRepoUUIDsByURLs(context.Background(), orgID, urls)
	assert.Error(t, err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	// Test with empty URL list
	urls = []string{}
	uuids, err = dao.FetchRepoUUIDsByURLs(context.Background(), orgID, urls)
	assert.NoError(t, err)
	assert.Empty(t, uuids)

	// Test with wrong org ID
	urls = []string{repo1.URL}
	_, err = dao.FetchRepoUUIDsByURLs(context.Background(), "wrong-org-id", urls)
	assert.Error(t, err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

func (suite *RepositoryConfigSuite) TestRefreshRedHatRepo() {
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	rhRepo := api.RepositoryRequest{
		UUID:                 nil,
		Name:                 utils.Ptr("Some redhat repo"),
		URL:                  utils.Ptr("https://testurl"),
		DistributionVersions: utils.Ptr([]string{"8"}),
		DistributionArch:     utils.Ptr("x86_64"),
		GpgKey:               nil,
		MetadataVerification: utils.Ptr(false),
		Origin:               utils.Ptr(config.OriginRedHat),
		ContentType:          utils.Ptr(config.ContentTypeRpm),
		Snapshot:             utils.Ptr(true),
	}
	response, err := dao.InternalOnly_RefreshPredefinedSnapshotRepo(context.Background(), rhRepo, "another-label", "test-feature")
	assert.NoError(suite.T(), err)

	assert.NotEmpty(suite.T(), response.UUID)
	assert.Equal(suite.T(), config.OriginRedHat, response.Origin)
	assert.Equal(suite.T(), config.RedHatOrg, response.OrgID)
	assert.Equal(suite.T(), "another-label", response.Label)

	// Change the name
	rhRepo.Name = utils.Ptr("another name")

	response, err = dao.InternalOnly_RefreshPredefinedSnapshotRepo(context.Background(), rhRepo, "some-label", "test-feature")
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), *rhRepo.Name, response.Name)
	assert.Equal(suite.T(), "some-label", response.Label)
}

func (suite *RepositoryConfigSuite) TestRefreshCommunityRepo() {
	dao := GetRepositoryConfigDao(suite.tx, suite.mockPulpClient, suite.mockFsClient)
	communityRepo := api.RepositoryRequest{
		UUID:                 nil,
		Name:                 utils.Ptr("Some commnuity repo"),
		URL:                  utils.Ptr("https://testurl"),
		DistributionVersions: utils.Ptr([]string{"8"}),
		DistributionArch:     utils.Ptr("x86_64"),
		GpgKey:               nil,
		MetadataVerification: utils.Ptr(false),
		Origin:               utils.Ptr(config.OriginCommunity),
		ContentType:          utils.Ptr(config.ContentTypeRpm),
		Snapshot:             utils.Ptr(true),
	}
	response, err := dao.InternalOnly_RefreshPredefinedSnapshotRepo(context.Background(), communityRepo, "", "")
	assert.NoError(suite.T(), err)

	assert.NotEmpty(suite.T(), response.UUID)
	assert.Equal(suite.T(), config.OriginCommunity, response.Origin)
	assert.Equal(suite.T(), config.CommunityOrg, response.OrgID)

	// Change the name
	communityRepo.Name = utils.Ptr("another name")

	response, err = dao.InternalOnly_RefreshPredefinedSnapshotRepo(context.Background(), communityRepo, "", "")
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), *communityRepo.Name, response.Name)
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

func (suite *RepositoryConfigSuite) TestIncrementResetFailedSnapshotCount() {
	t := suite.T()
	tx := suite.tx

	rConfigs, err := seeds.SeedRepositoryConfigurations(tx, 1, seeds.SeedOptions{})
	require.NoError(t, err)
	rConfig := rConfigs[0]
	daoReg := GetDaoRegistry(tx)
	err = daoReg.RepositoryConfig.InternalOnly_IncrementFailedSnapshotCount(context.Background(), rConfig.UUID)
	assert.NoError(t, err)
	err = daoReg.RepositoryConfig.InternalOnly_IncrementFailedSnapshotCount(context.Background(), rConfig.UUID)
	assert.NoError(t, err)

	err = tx.Where("uuid = ?", rConfig.UUID).Find(&rConfig).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(2), rConfig.FailedSnapshotCount)

	err = daoReg.RepositoryConfig.InternalOnly_ResetFailedSnapshotCount(context.Background(), rConfig.UUID)
	assert.NoError(t, err)
	err = tx.Where("uuid = ?", rConfig.UUID).Find(&rConfig).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(0), rConfig.FailedSnapshotCount)
}

func (suite *RepositoryConfigSuite) TestIncrementResetFailedSnapshotCountWithNilUUID() {
	t := suite.T()
	tx := suite.tx

	daoReg := GetDaoRegistry(tx)
	err := daoReg.RepositoryConfig.InternalOnly_IncrementFailedSnapshotCount(context.Background(), "")
	assert.NotNil(t, err)

	err = daoReg.RepositoryConfig.InternalOnly_ResetFailedSnapshotCount(context.Background(), "")
	assert.NotNil(t, err)
}

func (suite *RepositoryConfigSuite) TestFetchRepoConfigsForTemplate() {
	t := suite.T()
	tx := suite.tx
	daoReg := GetDaoRegistry(tx)
	orgID := seeds.RandomOrgId()

	repoConfigs, err := seeds.SeedRepositoryConfigurations(tx, 1, seeds.SeedOptions{OrgID: orgID})
	require.NoError(t, err)
	repoConfig := repoConfigs[0]
	err = tx.
		Preload("Repository").
		Where("uuid = ?", repoConfig.UUID).
		First(&repoConfig).
		Error
	require.NoError(t, err)

	snapshots, err := seeds.SeedSnapshots(tx, repoConfig.UUID, 1)
	require.NoError(t, err)

	templates, err := seeds.SeedTemplates(tx, 1, seeds.TemplateSeedOptions{OrgID: orgID, RepositoryConfigUUIDs: []string{repoConfig.UUID}, Snapshots: snapshots})
	require.NoError(t, err)
	template := templates[0]
	err = tx.
		Preload("TemplateRepositoryConfigurations").
		Where("uuid = ?", template.UUID).
		First(&template).
		Error
	require.NoError(t, err)

	templateRepoConfigs, err := daoReg.RepositoryConfig.InternalOnly_FetchRepoConfigsForTemplate(context.Background(), template)
	assert.NoError(t, err)
	assert.Equal(t, len(repoConfigs), len(templateRepoConfigs))
	assert.Equal(t, []models.RepositoryConfiguration{repoConfig}, templateRepoConfigs)
}

func (suite *RepositoryConfigSuite) createSnapshotAtSpecifiedTime(rConfig models.RepositoryConfiguration, CreatedAt time.Time) models.Snapshot {
	t := suite.T()
	tx := suite.tx

	snap := models.Snapshot{
		Base:                        models.Base{CreatedAt: CreatedAt},
		VersionHref:                 "/pulp/version",
		PublicationHref:             "/pulp/publication",
		DistributionPath:            fmt.Sprintf("/path/to/%v", uuid.NewString()),
		RepositoryConfigurationUUID: rConfig.UUID,
		ContentCounts:               models.ContentCountsType{"rpm.package": int64(3), "rpm.advisory": int64(1)},
		AddedCounts:                 models.ContentCountsType{"rpm.package": int64(1), "rpm.advisory": int64(3)},
		RemovedCounts:               models.ContentCountsType{"rpm.package": int64(2), "rpm.advisory": int64(2)},
	}

	sDao := snapshotDaoImpl{db: tx}
	err := sDao.Create(context.Background(), &snap)
	assert.NoError(t, err)
	return snap
}
