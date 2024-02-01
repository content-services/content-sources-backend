package dao

import (
	"context"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/google/uuid"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type EnvironmentSuite struct {
	*DaoSuite
	repoConfig  *models.RepositoryConfiguration
	repo        *models.Repository
	repoPrivate *models.Repository
}

func (s *EnvironmentSuite) SetupTest() {
	s.DaoSuite.SetupTest()

	repo := repoPublicTest.DeepCopy()
	if err := s.tx.Create(repo).Error; err != nil {
		s.FailNow("Preparing Repository record: %w", err)
	}
	s.repo = repo

	repoPrivate := repoPrivateTest.DeepCopy()
	if err := s.tx.Create(repoPrivate).Error; err != nil {
		s.FailNow("Preparing private Repository record: %w", err)
	}
	s.repoPrivate = repoPrivate

	repoConfig := repoConfigTest1.DeepCopy()
	repoConfig.RepositoryUUID = repo.Base.UUID
	if err := s.tx.Create(repoConfig).Error; err != nil {
		s.FailNow("Preparing RepositoryConfiguration record: %w", err)
	}
	s.repoConfig = repoConfig
}

func TestEnvironmentSuite(t *testing.T) {
	m := DaoSuite{}
	r := EnvironmentSuite{DaoSuite: &m}
	suite.Run(t, &r)
}

func (s *EnvironmentSuite) TestEnvironmentList() {
	var err error
	t := s.Suite.T()

	// Prepare RepositoryEnvironment records
	environment1 := repoEnvironmentTest1.DeepCopy()
	environment2 := repoEnvironmentTest2.DeepCopy()
	dao := GetEnvironmentDao(s.tx)

	err = s.tx.Create(&environment1).Error
	assert.NoError(t, err)
	err = s.tx.Create(&environment2).Error
	assert.NoError(t, err)
	err = s.tx.Create(&models.RepositoryEnvironment{
		RepositoryUUID:  s.repo.Base.UUID,
		EnvironmentUUID: environment1.Base.UUID,
	}).Error
	assert.NoError(t, err)
	err = s.tx.Create(&models.RepositoryEnvironment{
		RepositoryUUID:  s.repo.Base.UUID,
		EnvironmentUUID: environment2.Base.UUID,
	}).Error
	assert.NoError(t, err)

	var repoEnvironmentList api.RepositoryEnvironmentCollectionResponse
	var count int64
	repoEnvironmentList, count, err = dao.List(orgIDTest, s.repoConfig.Base.UUID, 10, 0, "", "")
	assert.NoError(t, err)
	assert.Equal(t, count, int64(2))
	assert.Equal(t, repoEnvironmentList.Meta.Count, count)
	assert.Equal(t, repoEnvironmentList.Data[0].Name, repoEnvironmentTest2.Name) // Asserts name:asc by default

	repoEnvironmentList, count, err = dao.List(orgIDTest, s.repoConfig.Base.UUID, 10, 0, "test-environment", "")
	assert.NoError(t, err)
	assert.Equal(t, count, int64(1))
	assert.Equal(t, repoEnvironmentList.Meta.Count, count)

	repoEnvironmentList, count, err = dao.List(orgIDTest, s.repoConfig.Base.UUID, 10, 0, "", "name:desc")
	assert.NoError(t, err)
	assert.Equal(t, count, int64(2))
	assert.Equal(t, repoEnvironmentList.Data[0].Name, repoEnvironmentTest1.Name) // Asserts name:desc

	repoEnvironmentList, count, err = dao.List(orgIDTest, s.repoConfig.Base.UUID, 10, 0, "non-existing-repo", "")
	assert.NoError(t, err)
	assert.Equal(t, count, int64(0))
}

func (s *EnvironmentSuite) TestEnvironmentListRepoNotFound() {
	t := s.Suite.T()
	dao := GetEnvironmentDao(s.tx)

	_, count, err := dao.List(orgIDTest, uuid.NewString(), 10, 0, "", "")
	assert.Equal(t, count, int64(0))
	assert.Error(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	environment1 := repoEnvironmentTest1.DeepCopy()
	err = s.tx.Create(&environment1).Error
	assert.NoError(t, err)
	err = s.tx.Create(&models.RepositoryEnvironment{
		RepositoryUUID:  s.repo.Base.UUID,
		EnvironmentUUID: environment1.Base.UUID,
	}).Error
	assert.NoError(t, err)

	_, count, err = dao.List(seeds.RandomOrgId(), s.repoConfig.Base.UUID, 10, 0, "", "")
	assert.Equal(t, count, int64(0))
	assert.Error(t, err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

func (s *EnvironmentSuite) TestEnvironmentSearch() {
	var err error
	t := s.Suite.T()
	tx := s.tx

	// Prepare environment records
	urls := []string{
		"https://repo-test-environment.org",
		"https://repo-demo-environment.org",
		"https://repo-private-environment.org",
	}
	environments := make([]models.Environment, 4)
	repoEnvironmentTest1.DeepCopyInto(&environments[0])
	repoEnvironmentTest2.DeepCopyInto(&environments[1])
	repoEnvironmentTest1.DeepCopyInto(&environments[2])
	repoEnvironmentTest2.DeepCopyInto(&environments[3])
	environments[0].ID = "test-environment-id"
	environments[1].ID = "demo-environment-id"
	environments[2].ID = "test-environment-id"
	environments[3].ID = "demo-environment-id"
	environments[0].Name = "test-environment"
	environments[1].Name = "demo-environment"
	environments[2].Name = "test-environment"
	environments[3].Name = "demo-environment"
	environments[0].Description = "test-environment description"
	environments[1].Description = "demo-environment description"
	environments[2].Description = "test-environment description"
	environments[3].Description = "demo-environment description"
	err = tx.Create(&environments).Error
	require.NoError(t, err)

	// Prepare Repository records
	repositories := make([]models.Repository, 3)
	repoPublicTest.DeepCopyInto(&repositories[0])
	repoPublicTest.DeepCopyInto(&repositories[1])
	repoPublicTest.DeepCopyInto(&repositories[2])
	repositories[0].URL = urls[0]
	repositories[1].URL = urls[1]
	repositories[2].URL = urls[2]
	repositories[0].Public = true
	repositories[1].Public = true
	repositories[2].Public = false
	err = tx.Create(&repositories).Error
	require.NoError(t, err)

	// Prepare RepositoryConfiguration records
	repositoryConfigurations := make([]models.RepositoryConfiguration, 1)
	repoConfigTest1.DeepCopyInto(&repositoryConfigurations[0])
	repositoryConfigurations[0].Name = "private-repository-configuration"
	repositoryConfigurations[0].RepositoryUUID = repositories[2].Base.UUID
	err = tx.Create(&repositoryConfigurations).Error
	require.NoError(t, err)

	// Prepare relations repositories_environments
	repositoriesEnvironments := make([]models.RepositoryEnvironment, 8)
	repositoriesEnvironments[0].RepositoryUUID = repositories[0].Base.UUID
	repositoriesEnvironments[0].EnvironmentUUID = environments[0].Base.UUID
	repositoriesEnvironments[1].RepositoryUUID = repositories[0].Base.UUID
	repositoriesEnvironments[1].EnvironmentUUID = environments[1].Base.UUID
	repositoriesEnvironments[2].RepositoryUUID = repositories[1].Base.UUID
	repositoriesEnvironments[2].EnvironmentUUID = environments[2].Base.UUID
	repositoriesEnvironments[3].RepositoryUUID = repositories[1].Base.UUID
	repositoriesEnvironments[3].EnvironmentUUID = environments[3].Base.UUID
	// Add environments to private repository
	repositoriesEnvironments[4].RepositoryUUID = repositories[2].Base.UUID
	repositoriesEnvironments[4].EnvironmentUUID = environments[0].Base.UUID
	repositoriesEnvironments[5].RepositoryUUID = repositories[2].Base.UUID
	repositoriesEnvironments[5].EnvironmentUUID = environments[1].Base.UUID
	repositoriesEnvironments[6].RepositoryUUID = repositories[2].Base.UUID
	repositoriesEnvironments[6].EnvironmentUUID = environments[2].Base.UUID
	repositoriesEnvironments[7].RepositoryUUID = repositories[2].Base.UUID
	repositoriesEnvironments[7].EnvironmentUUID = environments[3].Base.UUID
	err = tx.Create(&repositoriesEnvironments).Error
	require.NoError(t, err)

	uuids := []string{
		repositoryConfigurations[0].Base.UUID,
	}

	// Test Cases
	type TestCaseGiven struct {
		orgId string
		input api.ContentUnitSearchRequest
	}
	type TestCase struct {
		name     string
		given    TestCaseGiven
		expected []api.SearchEnvironmentResponse
	}
	testCases := []TestCase{
		{
			name: "The limit is applied correctly, and the order is respected",
			given: TestCaseGiven{
				orgId: orgIDTest,
				input: api.ContentUnitSearchRequest{
					URLs: []string{
						urls[0],
						urls[1],
					},
					Search: "",
					Limit:  pointy.Int(1),
				},
			},
			expected: []api.SearchEnvironmentResponse{
				{
					EnvironmentName: "demo-environment",
					Description:     "demo-environment description",
				},
			},
		},
		{
			name: "Search for the url[2] private repository",
			given: TestCaseGiven{
				orgId: orgIDTest,
				input: api.ContentUnitSearchRequest{
					URLs: []string{
						urls[2],
					},
					Search: "",
					Limit:  pointy.Int(50),
				},
			},
			expected: []api.SearchEnvironmentResponse{
				{
					EnvironmentName: "demo-environment",
					Description:     "demo-environment description",
				},
				{
					EnvironmentName: "test-environment",
					Description:     "test-environment description",
				},
			},
		},
		{
			name: "Search for url[0] and url[1] filtering for %%demo-%% environments and it returns 1 entry",
			given: TestCaseGiven{
				orgId: orgIDTest,
				input: api.ContentUnitSearchRequest{
					URLs: []string{
						urls[0],
						urls[1],
					},
					Search: "demo-",
					Limit:  pointy.Int(50),
				},
			},
			expected: []api.SearchEnvironmentResponse{
				{
					EnvironmentName: "demo-environment",
					Description:     "demo-environment description",
				},
			},
		},
		{
			name: "Search for url[0] and url[1] filtering for %%demo-%% environments testing case insensitivity and it returns 1 entry",
			given: TestCaseGiven{
				orgId: orgIDTest,
				input: api.ContentUnitSearchRequest{
					URLs: []string{
						urls[0],
						urls[1],
					},
					Search: "Demo-",
					Limit:  pointy.Int(50),
				},
			},
			expected: []api.SearchEnvironmentResponse{
				{
					EnvironmentName: "demo-environment",
					Description:     "demo-environment description",
				},
			},
		},
		{
			name: "Search for uuid[0] filtering for %%demo-%% environments and it returns 1 entry",
			given: TestCaseGiven{
				orgId: orgIDTest,
				input: api.ContentUnitSearchRequest{
					UUIDs: []string{
						uuids[0],
					},
					Search: "demo-",
					Limit:  pointy.Int(50),
				},
			},
			expected: []api.SearchEnvironmentResponse{
				{
					EnvironmentName: "demo-environment",
					Description:     "demo-environment description",
				},
			},
		},
		{
			name: "Search for (uuid[0] or URL) and filtering for demo-%% environments and it returns 1 entry",
			given: TestCaseGiven{
				orgId: orgIDTest,
				input: api.ContentUnitSearchRequest{
					URLs: []string{
						urls[0],
						urls[1],
					},
					UUIDs: []string{
						uuids[0],
					},
					Search: "demo-",
					Limit:  pointy.Int(50),
				},
			},
			expected: []api.SearchEnvironmentResponse{
				{
					EnvironmentName: "demo-environment",
					Description:     "demo-environment description",
				},
			},
		},
		{
			name: "Test Default limit parameter",
			given: TestCaseGiven{
				orgId: orgIDTest,
				input: api.ContentUnitSearchRequest{
					URLs: []string{
						urls[0],
						urls[1],
					},
					UUIDs: []string{
						uuids[0],
					},
					Search: "demo-",
					Limit:  nil,
				},
			},
			expected: []api.SearchEnvironmentResponse{
				{
					EnvironmentName: "demo-environment",
					Description:     "demo-environment description",
				},
			},
		},
		{
			name: "Test maximum limit parameter",
			given: TestCaseGiven{
				orgId: orgIDTest,
				input: api.ContentUnitSearchRequest{
					URLs: []string{
						urls[0],
						urls[1],
					},
					UUIDs: []string{
						uuids[0],
					},
					Search: "demo-",
					Limit:  pointy.Int(api.ContentUnitSearchRequestLimitMaximum * 2),
				},
			},
			expected: []api.SearchEnvironmentResponse{
				{
					EnvironmentName: "demo-environment",
					Description:     "demo-environment description",
				},
			},
		},
		{
			name: "Check sub-string search",
			given: TestCaseGiven{
				orgId: orgIDTest,
				input: api.ContentUnitSearchRequest{
					URLs: []string{
						urls[0],
						urls[1],
					},
					UUIDs: []string{
						uuids[0],
					},
					Search: "mo-env",
					Limit:  pointy.Int(50),
				},
			},
			expected: []api.SearchEnvironmentResponse{
				{
					EnvironmentName: "demo-environment",
					Description:     "demo-environment description",
				},
			},
		},
	}

	// Running all the test cases
	dao := GetEnvironmentDao(tx)
	for ict, caseTest := range testCases {
		t.Log(caseTest.name)
		var searchEnvironmentResponse []api.SearchEnvironmentResponse
		searchEnvironmentResponse, err = dao.Search(caseTest.given.orgId, caseTest.given.input)
		require.NoError(t, err)
		assert.Equal(t, len(caseTest.expected), len(searchEnvironmentResponse))
		for i, expected := range caseTest.expected {
			if i < len(searchEnvironmentResponse) {
				assert.Equal(t, expected.EnvironmentName, searchEnvironmentResponse[i].EnvironmentName, "TestCase: %i; expectedIndex: %i", ict, i)
				assert.Contains(t, searchEnvironmentResponse[i].Description, expected.Description, "TestCase: %i; expectedIndex: %i", ict, i)
			}
		}
	}
}

func randomEnvironmentID(size int) string {
	const lookup string = "0123456789abcdefghijklmnopqrstuvwxyz"
	return seeds.RandStringWithChars(size, lookup)
}

func randomYumEnvironment(environment *yum.Environment) {
	if environment == nil {
		return
	}
	environmentID := randomEnvironmentID(32)
	environment.ID = environmentID
	environment.Name = "test-environment-name"
	environment.Description = "test description"
}

func makeYumEnvironment(size int) []yum.Environment {
	var environments []yum.Environment = []yum.Environment{}

	if size < 0 {
		panic("size can not be a negative number")
	}

	if size == 0 {
		return environments
	}

	environments = make([]yum.Environment, size)
	for i := 0; i < size; i++ {
		randomYumEnvironment(&environments[i])
	}

	return environments
}

func (s *EnvironmentSuite) prepareScenarioEnvironments(scenario int, limit int) []yum.Environment {
	s.db.CreateBatchSize = limit

	switch scenario {
	case scenario0:
		{
			return makeYumEnvironment(0)
		}
	case scenario3:
		// The reason of this scenario is to make debugging easier
		{
			return makeYumEnvironment(3)
		}
	case scenarioUnderThreshold:
		{
			return makeYumEnvironment(limit - 1)
		}
	case scenarioThreshold:
		{
			return makeYumEnvironment(limit)
		}
	case scenarioOverThreshold:
		{
			return makeYumEnvironment(limit + 1)
		}
	default:
		{
			return makeYumEnvironment(0)
		}
	}
}

func (s *EnvironmentSuite) TestEnvironmentSearchError() {
	var err error
	t := s.Suite.T()
	tx := s.tx
	txSP := strings.ToLower("TestEnvironmentSearchError")

	var searchEnvironmentResponse []api.SearchEnvironmentResponse
	dao := GetEnvironmentDao(tx)
	// We are going to launch database operations that evoke errors, so we need to restore
	// the state previous to the error to let the test do more actions
	tx.SavePoint(txSP)

	searchEnvironmentResponse, err = dao.Search("", api.ContentUnitSearchRequest{Search: "", URLs: []string{"https:/noreturn.org"}, Limit: pointy.Int(100)})
	require.Error(t, err)
	assert.Equal(t, int(0), len(searchEnvironmentResponse))
	assert.Equal(t, err.Error(), "orgID can not be an empty string")
	tx.RollbackTo(txSP)

	searchEnvironmentResponse, err = dao.Search(orgIDTest, api.ContentUnitSearchRequest{Search: "", Limit: pointy.Int(100)})
	require.Error(t, err)
	assert.Equal(t, int(0), len(searchEnvironmentResponse))
	assert.Equal(t, err.Error(), "must contain at least 1 URL or 1 UUID")
	tx.RollbackTo(txSP)
}

func (s *EnvironmentSuite) genericInsertForRepository(testCase TestInsertForRepositoryCase) {
	t := s.Suite.T()
	tx := s.tx

	dao := GetEnvironmentDao(tx)

	e := s.prepareScenarioEnvironments(testCase.given, 10)
	records, err := dao.InsertForRepository(s.repo.Base.UUID, e)

	var environmentCount int = 0
	tx.Select("count(*) as environment_count").
		Table(models.TableNameEnvironment).
		Joins("inner join "+models.TableNameEnvironmentsRepositories+" on environments.uuid = "+models.TableNameEnvironmentsRepositories+".environment_uuid").
		Where(models.TableNameEnvironmentsRepositories+".repository_uuid = ?", s.repo.Base.UUID).
		Scan(&environmentCount)
	require.NoError(t, tx.Error)

	if testCase.expected != "" {
		assert.Error(t, err)
		assert.Contains(t, err.Error(), testCase.expected)
	} else {
		assert.NoError(t, err)
		assert.Equal(t, int64(len(e)), records)
		assert.Equal(t, int64(environmentCount), records)
	}
}

func (s *EnvironmentSuite) TestInsertForRepositoryScenario0() {
	s.genericInsertForRepository(testCases[scenario0])
}

func (s *EnvironmentSuite) TestInsertForRepositoryScenario3() {
	s.genericInsertForRepository(testCases[scenario3])
}

func (s *EnvironmentSuite) TestInsertForRepositoryScenarioUnderThreshold() {
	s.genericInsertForRepository(testCases[scenarioUnderThreshold])
}
func (s *EnvironmentSuite) TestInsertForRepositoryScenarioThreshold() {
	s.genericInsertForRepository(testCases[scenarioThreshold])
}
func (s *EnvironmentSuite) TestInsertForRepositoryScenarioOverThreshold() {
	s.genericInsertForRepository(testCases[scenarioOverThreshold])
}

func repoEnvironmentCount(db *gorm.DB, repoUuid string) (int64, error) {
	var environmentCount int64
	err := db.
		Table("environments").
		Joins("inner join repositories_environments on repositories_environments.environment_uuid = environments.uuid").
		Where("repositories_environments.repository_uuid = ?", repoUuid).
		Count(&environmentCount).
		Error
	return environmentCount, err
}
func (s *EnvironmentSuite) TestInsertForRepositoryWithExistingEnvironments() {
	t := s.Suite.T()
	tx := s.tx
	var environmentCount int64

	pagedEnvironmentInsertsLimit := 10
	groupCount := 5

	dao := GetEnvironmentDao(tx)
	e := s.prepareScenarioEnvironments(scenarioThreshold, pagedEnvironmentInsertsLimit)
	records, err := dao.InsertForRepository(s.repo.Base.UUID, e[0:groupCount])
	assert.NoError(t, err)
	assert.Equal(t, int64(len(e[0:groupCount])), records)
	environmentCount, err = repoEnvironmentCount(tx, s.repo.UUID)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(e[0:groupCount])), environmentCount)

	records, err = dao.InsertForRepository(s.repo.Base.UUID, e[groupCount:])
	assert.NoError(t, err)
	assert.Equal(t, int64(len(e[groupCount:])), records)
	environmentCount, err = repoEnvironmentCount(tx, s.repo.UUID)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(e[groupCount:])), environmentCount)

	records, err = dao.InsertForRepository(s.repoPrivate.Base.UUID, e[1:groupCount+1])
	assert.NoError(t, err)

	assert.Equal(t, int64(groupCount), records)
	environmentCount, err = repoEnvironmentCount(tx, s.repoPrivate.UUID)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(e[1:groupCount+1])), environmentCount)

	records, err = dao.InsertForRepository(s.repoPrivate.Base.UUID, e[1:groupCount+1])
	assert.NoError(t, err)
	assert.Equal(t, int64(0), records) // Environments have already been inserted

	environmentCount, err = repoEnvironmentCount(tx, s.repoPrivate.Base.UUID)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(e[1:groupCount+1])), environmentCount)
}

func (s *EnvironmentSuite) TestInsertForRepositoryWithWrongRepoUUID() {
	t := s.Suite.T()
	tx := s.tx

	pagedEnvironmentInsertsLimit := 100

	dao := GetEnvironmentDao(tx)
	e := s.prepareScenarioEnvironments(scenario3, pagedEnvironmentInsertsLimit)
	records, err := dao.InsertForRepository(uuid.NewString(), e)

	assert.Error(t, err)
	assert.Equal(t, records, int64(0))
}

func (s *EnvironmentSuite) TestOrphanCleanup() {
	var err error
	var count int64

	t := s.Suite.T()

	// Prepare RepositoryEnvironment records
	environment1 := repoEnvironmentTest1.DeepCopy()
	dao := GetEnvironmentDao(s.tx)

	err = s.tx.Create(&environment1).Error
	assert.NoError(t, err)

	s.tx.Model(&environment1).Where("uuid = ?", environment1.UUID).Count(&count)
	assert.Equal(t, int64(1), count)

	err = dao.OrphanCleanup()
	assert.NoError(t, err)

	s.tx.Model(&environment1).Where("uuid = ?", environment1.UUID).Count(&count)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Repeat the call for 'len(danglingEnvironmentUuids) == 0'
	err = dao.OrphanCleanup()
	assert.NoError(t, err)
}

func (s *EnvironmentSuite) TestEmptyOrphanCleanup() {
	var count int64
	var countAfter int64
	dao := GetEnvironmentDao(s.tx)
	err := dao.OrphanCleanup() // Clear out any existing orphaned environments in the db
	assert.NoError(s.T(), err)

	s.tx.Model(&repoEnvironmentTest1).Count(&count)
	err = dao.OrphanCleanup()
	assert.NoError(s.T(), err)

	s.tx.Model(&repoEnvironmentTest1).Count(&countAfter)
	assert.Equal(s.T(), count, countAfter)
}

func (s *EnvironmentSuite) TestFilteredConvertEnvironments() {
	t := s.T()
	givenYumEnvironments := []yum.Environment{
		{
			ID:          "environment-1",
			Name:        "environment-name-1",
			Description: "description",
		},
		{
			ID:          "environment-2",
			Name:        "environment-name-2",
			Description: "description",
		},
	}
	givenExcludedEnvs := []string{"environment-2"}

	expected := []models.Environment{
		{
			ID:          givenYumEnvironments[0].ID,
			Name:        string(givenYumEnvironments[0].Name),
			Description: string(givenYumEnvironments[0].Description),
		},
	}

	result := FilteredConvertEnvironments(givenYumEnvironments, givenExcludedEnvs)
	assert.Equal(t, len(expected), len(result))
	assert.Equal(t, expected[0].ID, givenYumEnvironments[0].ID)
	assert.Equal(t, expected[0].Name, string(givenYumEnvironments[0].Name))
	assert.Equal(t, expected[0].Description, string(givenYumEnvironments[0].Description))
}

func (s *EnvironmentSuite) TestSearchSnapshotEnvironments() {
	orgId := seeds.RandomOrgId()
	mTangy, origTangy := mockTangy(s.T())
	defer func() { config.Tang = origTangy }()
	ctx := context.Background()

	hrefs := []string{"some_pulp_version_href"}
	expected := []tangy.RpmEnvironmentSearch{{
		Name:        "Foodidly",
		Description: "there was a great foo",
		ID:          "Foddidly",
	}}

	// Create a repo config, and snapshot, update its version_href to expected href
	err := seeds.SeedRepositoryConfigurations(s.tx, 1, seeds.SeedOptions{
		OrgID:     orgId,
		BatchSize: 0,
	})
	require.NoError(s.T(), err)
	repoConfig := models.RepositoryConfiguration{}
	res := s.tx.Where("org_id = ?", orgId).First(&repoConfig)
	require.NoError(s.T(), res.Error)
	snaps, err := seeds.SeedSnapshots(s.tx, repoConfig.UUID, 1)
	require.NoError(s.T(), err)
	res = s.tx.Model(models.Snapshot{}).Where("repository_configuration_uuid = ?", repoConfig.UUID).Update("version_href", hrefs[0])
	require.NoError(s.T(), res.Error)

	// pulpHrefs, request.Search, *request.Limit)
	mTangy.On("RpmRepositoryVersionEnvironmentSearch", ctx, hrefs, "Foo", 55).Return(expected, nil)

	dao := GetEnvironmentDao(s.tx)
	ret, err := dao.SearchSnapshotEnvironments(ctx, orgId, api.SnapshotSearchRpmRequest{
		UUIDs:  []string{snaps[0].UUID},
		Search: "Foo",
		Limit:  pointy.Pointer(55),
	})
	require.NoError(s.T(), err)

	assert.Equal(s.T(), []api.SearchEnvironmentResponse{{
		EnvironmentName: expected[0].Name,
		Description:     expected[0].Description,
		ID:              expected[0].ID,
	}}, ret)
}
