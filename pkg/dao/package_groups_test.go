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
	"github.com/lib/pq"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type PackageGroupSuite struct {
	*DaoSuite
	repoConfig  *models.RepositoryConfiguration
	repo        *models.Repository
	repoPrivate *models.Repository
}

func (s *PackageGroupSuite) SetupTest() {
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

func TestPackageGroupSuite(t *testing.T) {
	m := DaoSuite{}
	r := PackageGroupSuite{DaoSuite: &m}
	suite.Run(t, &r)
}

func (s *PackageGroupSuite) TestPackageGroupList() {
	var err error
	t := s.Suite.T()

	// Prepare RepositoryPackageGroup records
	packageGroup1 := repoPackageGroupTest1.DeepCopy()
	packageGroup2 := repoPackageGroupTest2.DeepCopy()
	dao := GetPackageGroupDao(s.tx)

	err = s.tx.Create(&packageGroup1).Error
	assert.NoError(t, err)
	err = s.tx.Create(&packageGroup2).Error
	assert.NoError(t, err)
	err = s.tx.Create(&models.RepositoryPackageGroup{
		RepositoryUUID:   s.repo.Base.UUID,
		PackageGroupUUID: packageGroup1.Base.UUID,
	}).Error
	assert.NoError(t, err)
	err = s.tx.Create(&models.RepositoryPackageGroup{
		RepositoryUUID:   s.repo.Base.UUID,
		PackageGroupUUID: packageGroup2.Base.UUID,
	}).Error
	assert.NoError(t, err)

	var repoPackageGroupList api.RepositoryPackageGroupCollectionResponse
	var count int64
	repoPackageGroupList, count, err = dao.List(orgIDTest, s.repoConfig.Base.UUID, 10, 0, "", "")
	assert.NoError(t, err)
	assert.Equal(t, count, int64(2))
	assert.Equal(t, repoPackageGroupList.Meta.Count, count)
	assert.Equal(t, repoPackageGroupList.Data[0].Name, repoPackageGroupTest2.Name) // Asserts name:asc by default

	repoPackageGroupList, count, err = dao.List(orgIDTest, s.repoConfig.Base.UUID, 10, 0, "test-package-group", "")
	assert.NoError(t, err)
	assert.Equal(t, count, int64(1))
	assert.Equal(t, repoPackageGroupList.Meta.Count, count)

	repoPackageGroupList, count, err = dao.List(orgIDTest, s.repoConfig.Base.UUID, 10, 0, "", "name:desc")
	assert.NoError(t, err)
	assert.Equal(t, count, int64(2))
	assert.Equal(t, repoPackageGroupList.Data[0].Name, repoPackageGroupTest1.Name) // Asserts name:desc

	repoPackageGroupList, count, err = dao.List(orgIDTest, s.repoConfig.Base.UUID, 10, 0, "non-existing-repo", "")
	assert.NoError(t, err)
	assert.Equal(t, count, int64(0))
}

func (s *PackageGroupSuite) TestPackageGroupListRepoNotFound() {
	t := s.Suite.T()
	dao := GetPackageGroupDao(s.tx)

	_, count, err := dao.List(orgIDTest, uuid.NewString(), 10, 0, "", "")
	assert.Equal(t, count, int64(0))
	assert.Error(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)

	packageGroup1 := repoPackageGroupTest1.DeepCopy()
	err = s.tx.Create(&packageGroup1).Error
	assert.NoError(t, err)
	err = s.tx.Create(&models.RepositoryPackageGroup{
		RepositoryUUID:   s.repo.Base.UUID,
		PackageGroupUUID: packageGroup1.Base.UUID,
	}).Error
	assert.NoError(t, err)

	_, count, err = dao.List(seeds.RandomOrgId(), s.repoConfig.Base.UUID, 10, 0, "", "")
	assert.Equal(t, count, int64(0))
	assert.Error(t, err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
}

func (s *PackageGroupSuite) TestPackageGroupSearch() {
	var err error
	t := s.Suite.T()
	tx := s.tx

	// Prepare package group records
	urls := []string{
		"https://repo-test-package-group.org",
		"https://repo-demo-package-group.org",
		"https://repo-private-package-group.org",
	}
	packageGroups := make([]models.PackageGroup, 4)
	repoPackageGroupTest1.DeepCopyInto(&packageGroups[0])
	repoPackageGroupTest2.DeepCopyInto(&packageGroups[1])
	repoPackageGroupTest1.DeepCopyInto(&packageGroups[2])
	repoPackageGroupTest2.DeepCopyInto(&packageGroups[3])
	packageGroups[0].ID = "test-package-group-id"
	packageGroups[1].ID = "demo-package-group-id"
	packageGroups[2].ID = "test-package-group-id"
	packageGroups[3].ID = "demo-package-group-id"
	packageGroups[0].Name = "test-package-group"
	packageGroups[1].Name = "demo-package-group"
	packageGroups[2].Name = "test-package-group"
	packageGroups[3].Name = "demo-package-group"
	packageGroups[0].Description = "test-package-group description"
	packageGroups[1].Description = "demo-package-group description"
	packageGroups[2].Description = "test-package-group description"
	packageGroups[3].Description = "demo-package-group description"
	packageGroups[0].PackageList = []string{"package1"}
	packageGroups[1].PackageList = []string{"package1", "package2"}
	packageGroups[2].PackageList = []string{"package1"}
	packageGroups[3].PackageList = []string{"package2", "package3"}
	err = tx.Create(&packageGroups).Error
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

	// Prepare relations repositories_package_groups
	repositoriesPackageGroups := make([]models.RepositoryPackageGroup, 8)
	repositoriesPackageGroups[0].RepositoryUUID = repositories[0].Base.UUID
	repositoriesPackageGroups[0].PackageGroupUUID = packageGroups[0].Base.UUID
	repositoriesPackageGroups[1].RepositoryUUID = repositories[0].Base.UUID
	repositoriesPackageGroups[1].PackageGroupUUID = packageGroups[1].Base.UUID
	repositoriesPackageGroups[2].RepositoryUUID = repositories[1].Base.UUID
	repositoriesPackageGroups[2].PackageGroupUUID = packageGroups[2].Base.UUID
	repositoriesPackageGroups[3].RepositoryUUID = repositories[1].Base.UUID
	repositoriesPackageGroups[3].PackageGroupUUID = packageGroups[3].Base.UUID
	// Add package groups to private repository
	repositoriesPackageGroups[4].RepositoryUUID = repositories[2].Base.UUID
	repositoriesPackageGroups[4].PackageGroupUUID = packageGroups[0].Base.UUID
	repositoriesPackageGroups[5].RepositoryUUID = repositories[2].Base.UUID
	repositoriesPackageGroups[5].PackageGroupUUID = packageGroups[1].Base.UUID
	repositoriesPackageGroups[6].RepositoryUUID = repositories[2].Base.UUID
	repositoriesPackageGroups[6].PackageGroupUUID = packageGroups[2].Base.UUID
	repositoriesPackageGroups[7].RepositoryUUID = repositories[2].Base.UUID
	repositoriesPackageGroups[7].PackageGroupUUID = packageGroups[3].Base.UUID
	err = tx.Create(&repositoriesPackageGroups).Error
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
		expected []api.SearchPackageGroupResponse
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
			expected: []api.SearchPackageGroupResponse{
				{
					PackageGroupName: "demo-package-group",
					Description:      "demo-package-group description",
					PackageList:      []string{"package1", "package2", "package3"},
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
			expected: []api.SearchPackageGroupResponse{
				{
					PackageGroupName: "demo-package-group",
					Description:      "demo-package-group description",
					PackageList:      []string{"package1", "package2", "package3"},
				},
				{
					PackageGroupName: "test-package-group",
					Description:      "test-package-group description",
					PackageList:      []string{"package1"},
				},
			},
		},
		{
			name: "Search for url[0] and url[1] filtering for %%demo-%% package groups and it returns 1 entry",
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
			expected: []api.SearchPackageGroupResponse{
				{
					PackageGroupName: "demo-package-group",
					Description:      "demo-package-group description",
					PackageList:      []string{"package1", "package2", "package3"},
				},
			},
		},
		{
			name: "Search for url[0] and url[1] filtering for %%demo-%% package groups testing case insensitivity and it returns 1 entry",
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
			expected: []api.SearchPackageGroupResponse{
				{
					PackageGroupName: "demo-package-group",
					Description:      "demo-package-group description",
					PackageList:      []string{"package1", "package2", "package3"},
				},
			},
		},
		{
			name: "Search for uuid[0] filtering for %%demo-%% package groups and it returns 1 entry",
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
			expected: []api.SearchPackageGroupResponse{
				{
					PackageGroupName: "demo-package-group",
					Description:      "demo-package-group description",
					PackageList:      []string{"package1", "package2", "package3"},
				},
			},
		},
		{
			name: "Search for (uuid[0] or URL) and filtering for demo-%% package groups and it returns 1 entry",
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
			expected: []api.SearchPackageGroupResponse{
				{
					PackageGroupName: "demo-package-group",
					Description:      "demo-package-group description",
					PackageList:      []string{"package1", "package2", "package3"},
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
			expected: []api.SearchPackageGroupResponse{
				{
					PackageGroupName: "demo-package-group",
					Description:      "demo-package-group description",
					PackageList:      []string{"package1", "package2", "package3"},
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
			expected: []api.SearchPackageGroupResponse{
				{
					PackageGroupName: "demo-package-group",
					Description:      "demo-package-group description",
					PackageList:      []string{"package1", "package2", "package3"},
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
					Search: "mo-pack",
					Limit:  pointy.Int(50),
				},
			},
			expected: []api.SearchPackageGroupResponse{
				{
					PackageGroupName: "demo-package-group",
					Description:      "demo-package-group description",
					PackageList:      []string{"package1", "package2", "package3"},
				},
			},
		},
		{
			name: "Check deduplication / concatenation of packages within groups of same name",
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
					Search: "demo",
					Limit:  pointy.Int(50),
				},
			},
			expected: []api.SearchPackageGroupResponse{
				{
					PackageGroupName: "demo-package-group",
					Description:      "demo-package-group description",
					PackageList:      []string{"package1", "package2", "package3"},
				},
			},
		},
	}

	// Running all the test cases
	dao := GetPackageGroupDao(tx)
	for ict, caseTest := range testCases {
		t.Log(caseTest.name)
		var searchPackageGroupResponse []api.SearchPackageGroupResponse
		searchPackageGroupResponse, err = dao.Search(caseTest.given.orgId, caseTest.given.input)
		require.NoError(t, err)
		assert.Equal(t, len(caseTest.expected), len(searchPackageGroupResponse))
		for i, expected := range caseTest.expected {
			if i < len(searchPackageGroupResponse) {
				assert.Equal(t, expected.PackageGroupName, searchPackageGroupResponse[i].PackageGroupName, "TestCase: %i; expectedIndex: %i", ict, i)
				assert.Contains(t, searchPackageGroupResponse[i].Description, expected.Description, "TestCase: %i; expectedIndex: %i", ict, i)
				assert.Equal(t, expected.PackageList, searchPackageGroupResponse[i].PackageList, "TestCase: %i; expectedIndex: %i", ict, i)
			}
		}
	}
}

func randomPackageGroupID(size int) string {
	const lookup string = "0123456789abcdefghijklmnopqrstuvwxyz"
	return seeds.RandStringWithChars(size, lookup)
}

func randomYumPackageGroup(pkgGroup *yum.PackageGroup) {
	if pkgGroup == nil {
		return
	}
	pkgGroupID := randomPackageGroupID(32)
	pkgGroup.ID = pkgGroupID
	pkgGroup.Name = "test-group-name"
	pkgGroup.Description = "test description"
	pkgGroup.PackageList = []string{"package1"}
}

func makeYumPackageGroup(size int) []yum.PackageGroup {
	var pkgGroups []yum.PackageGroup = []yum.PackageGroup{}

	if size < 0 {
		panic("size can not be a negative number")
	}

	if size == 0 {
		return pkgGroups
	}

	pkgGroups = make([]yum.PackageGroup, size)
	for i := 0; i < size; i++ {
		randomYumPackageGroup(&pkgGroups[i])
	}

	return pkgGroups
}

func (s *PackageGroupSuite) prepareScenarioPackageGroups(scenario int, limit int) []yum.PackageGroup {
	s.db.CreateBatchSize = limit

	switch scenario {
	case scenario0:
		{
			return makeYumPackageGroup(0)
		}
	case scenario3:
		// The reason of this scenario is to make debugging easier
		{
			return makeYumPackageGroup(3)
		}
	case scenarioUnderThreshold:
		{
			return makeYumPackageGroup(limit - 1)
		}
	case scenarioThreshold:
		{
			return makeYumPackageGroup(limit)
		}
	case scenarioOverThreshold:
		{
			return makeYumPackageGroup(limit + 1)
		}
	default:
		{
			return makeYumPackageGroup(0)
		}
	}
}

func (s *PackageGroupSuite) TestPackageGroupSearchError() {
	var err error
	t := s.Suite.T()
	tx := s.tx
	txSP := strings.ToLower("TestPackageGroupSearchError")

	var searchPackageGroupResponse []api.SearchPackageGroupResponse
	dao := GetPackageGroupDao(tx)
	// We are going to launch database operations that evoke errors, so we need to restore
	// the state previous to the error to let the test do more actions
	tx.SavePoint(txSP)

	searchPackageGroupResponse, err = dao.Search("", api.ContentUnitSearchRequest{Search: "", URLs: []string{"https:/noreturn.org"}, Limit: pointy.Int(100)})
	require.Error(t, err)
	assert.Equal(t, int(0), len(searchPackageGroupResponse))
	assert.Equal(t, err.Error(), "orgID can not be an empty string")
	tx.RollbackTo(txSP)

	searchPackageGroupResponse, err = dao.Search(orgIDTest, api.ContentUnitSearchRequest{Search: "", Limit: pointy.Int(100)})
	require.Error(t, err)
	assert.Equal(t, int(0), len(searchPackageGroupResponse))
	assert.Equal(t, err.Error(), "must contain at least 1 URL or 1 UUID")
	tx.RollbackTo(txSP)
}

func (s *PackageGroupSuite) genericInsertForRepository(testCase TestInsertForRepositoryCase) {
	t := s.Suite.T()
	tx := s.tx

	dao := GetPackageGroupDao(tx)

	p := s.prepareScenarioPackageGroups(testCase.given, 10)
	records, err := dao.InsertForRepository(s.repo.Base.UUID, p)

	var packageGroupCount int = 0
	tx.Select("count(*) as package_group_count").
		Table(models.TableNamePackageGroup).
		Joins("inner join "+models.TableNamePackageGroupsRepositories+" on package_groups.uuid = "+models.TableNamePackageGroupsRepositories+".package_group_uuid").
		Where(models.TableNamePackageGroupsRepositories+".repository_uuid = ?", s.repo.Base.UUID).
		Scan(&packageGroupCount)
	require.NoError(t, tx.Error)

	if testCase.expected != "" {
		assert.Error(t, err)
		assert.Contains(t, err.Error(), testCase.expected)
	} else {
		assert.NoError(t, err)
		assert.Equal(t, int64(len(p)), records)
		assert.Equal(t, int64(packageGroupCount), records)
	}
}

func (s *PackageGroupSuite) TestInsertForRepositoryScenario0() {
	s.genericInsertForRepository(testCases[scenario0])
}

func (s *PackageGroupSuite) TestInsertForRepositoryScenario3() {
	s.genericInsertForRepository(testCases[scenario3])
}

func (s *PackageGroupSuite) TestInsertForRepositoryScenarioUnderThreshold() {
	s.genericInsertForRepository(testCases[scenarioUnderThreshold])
}
func (s *PackageGroupSuite) TestInsertForRepositoryScenarioThreshold() {
	s.genericInsertForRepository(testCases[scenarioThreshold])
}
func (s *PackageGroupSuite) TestInsertForRepositoryScenarioOverThreshold() {
	s.genericInsertForRepository(testCases[scenarioOverThreshold])
}

func repoPackageGroupCount(db *gorm.DB, repoUuid string) (int64, error) {
	var packageGroupCount int64
	err := db.
		Table("package_groups").
		Joins("inner join repositories_package_groups on repositories_package_groups.package_group_uuid = package_groups.uuid").
		Where("repositories_package_groups.repository_uuid = ?", repoUuid).
		Count(&packageGroupCount).
		Error
	return packageGroupCount, err
}

func (s *PackageGroupSuite) TestInsertForRepositoryWithExistingGroups() {
	t := s.Suite.T()
	tx := s.tx
	var packageGroupCount int64

	pagedPackageGroupInsertsLimit := 10
	groupCount := 5

	dao := GetPackageGroupDao(tx)
	p := s.prepareScenarioPackageGroups(scenarioThreshold, pagedPackageGroupInsertsLimit)
	records, err := dao.InsertForRepository(s.repo.Base.UUID, p[0:groupCount])
	assert.NoError(t, err)
	assert.Equal(t, int64(len(p[0:groupCount])), records)
	packageGroupCount, err = repoPackageGroupCount(tx, s.repo.UUID)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(p[0:groupCount])), packageGroupCount)

	records, err = dao.InsertForRepository(s.repo.Base.UUID, p[groupCount:])
	assert.NoError(t, err)
	assert.Equal(t, int64(len(p[groupCount:])), records)
	packageGroupCount, err = repoPackageGroupCount(tx, s.repo.UUID)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(p[groupCount:])), packageGroupCount)

	records, err = dao.InsertForRepository(s.repoPrivate.Base.UUID, p[1:groupCount+1])
	assert.NoError(t, err)

	assert.Equal(t, int64(groupCount), records)
	packageGroupCount, err = repoPackageGroupCount(tx, s.repoPrivate.UUID)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(p[1:groupCount+1])), packageGroupCount)

	records, err = dao.InsertForRepository(s.repoPrivate.Base.UUID, p[1:groupCount+1])
	assert.NoError(t, err)
	assert.Equal(t, int64(0), records) // Package groups have already been inserted

	packageGroupCount, err = repoPackageGroupCount(tx, s.repoPrivate.Base.UUID)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(p[1:groupCount+1])), packageGroupCount)
}

func (s *PackageGroupSuite) TestInsertForRepositoryWithWrongRepoUUID() {
	t := s.Suite.T()
	tx := s.tx

	pagedPackageGroupInsertsLimit := 100

	dao := GetPackageGroupDao(tx)
	p := s.prepareScenarioPackageGroups(scenario3, pagedPackageGroupInsertsLimit)
	records, err := dao.InsertForRepository(uuid.NewString(), p)

	assert.Error(t, err)
	assert.Equal(t, records, int64(0))
}

func (s *PackageGroupSuite) TestOrphanCleanup() {
	var err error
	var count int64

	t := s.Suite.T()

	// Prepare RepositoryPackageGroup records
	packageGroup1 := repoPackageGroupTest1.DeepCopy()
	dao := GetPackageGroupDao(s.tx)

	err = s.tx.Create(&packageGroup1).Error
	assert.NoError(t, err)

	s.tx.Model(&packageGroup1).Where("uuid = ?", packageGroup1.UUID).Count(&count)
	assert.Equal(t, int64(1), count)

	err = dao.OrphanCleanup()
	assert.NoError(t, err)

	s.tx.Model(&packageGroup1).Where("uuid = ?", packageGroup1.UUID).Count(&count)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Repeat the call for 'len(danglingPackageGroupUuids) == 0'
	err = dao.OrphanCleanup()
	assert.NoError(t, err)
}

func (s *PackageGroupSuite) TestEmptyOrphanCleanup() {
	var count int64
	var countAfter int64
	dao := GetPackageGroupDao(s.tx)
	err := dao.OrphanCleanup() // Clear out any existing orphaned package groups in the db
	assert.NoError(s.T(), err)

	s.tx.Model(&repoPackageGroupTest1).Count(&count)
	err = dao.OrphanCleanup()
	assert.NoError(s.T(), err)

	s.tx.Model(&repoPackageGroupTest1).Count(&countAfter)
	assert.Equal(s.T(), count, countAfter)
}

func (s *PackageGroupSuite) TestSearchSnapshotPackageGroups() {
	orgId := seeds.RandomOrgId()
	mTangy, origTangy := mockTangy(s.T())
	defer func() { config.Tang = origTangy }()
	ctx := context.Background()

	hrefs := []string{"some_pulp_version_href"}
	expected := []tangy.RpmPackageGroupSearch{{
		Name:        "Foodidly",
		ID:          "Fooddidly",
		Description: "there was a great foo",
		Packages:    []string{"foo"},
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
	mTangy.On("RpmRepositoryVersionPackageGroupSearch", ctx, hrefs, "Foo", 55).Return(expected, nil)

	dao := GetPackageGroupDao(s.tx)
	ret, err := dao.SearchSnapshotPackageGroups(ctx, orgId, api.SnapshotSearchRpmRequest{
		UUIDs:  []string{snaps[0].UUID},
		Search: "Foo",
		Limit:  pointy.Pointer(55),
	})
	require.NoError(s.T(), err)

	assert.Equal(s.T(), []api.SearchPackageGroupResponse{{
		PackageGroupName: expected[0].Name,
		ID:               expected[0].ID,
		Description:      expected[0].Description,
		PackageList:      expected[0].Packages,
	}}, ret)
}

func TestFilteredConvertPackageGroups(t *testing.T) {
	givenYumPackageGroups := []yum.PackageGroup{
		{
			ID:          "package-group-1",
			Name:        "package-group-name-1",
			Description: "description",
			PackageList: []string{"package1"},
		},
		{
			ID:          "package-group-2",
			Name:        "package-group-name-2",
			Description: "description",
			PackageList: []string{"package1"},
		},
	}

	givenExcludedHashes := []string{"aa0722135bfd094d0e98e55d81a8920c5440dc062e1e3ccc370488db032ca60c"}

	expected := []models.PackageGroup{
		{
			ID:          givenYumPackageGroups[0].ID,
			Name:        string(givenYumPackageGroups[0].Name),
			Description: string(givenYumPackageGroups[0].Description),
			PackageList: pq.StringArray(givenYumPackageGroups[0].PackageList),
			HashValue:   "daaf7c707f4b5b79d51122ba11314cdb036ba574c9504a7069efdf707d030f09",
		},
	}

	result := FilteredConvertPackageGroups(givenYumPackageGroups, givenExcludedHashes)
	assert.Equal(t, len(expected), len(result))
	assert.Equal(t, expected[0].ID, givenYumPackageGroups[0].ID)
	assert.Equal(t, expected[0].Name, string(givenYumPackageGroups[0].Name))
	assert.Equal(t, expected[0].Description, string(givenYumPackageGroups[0].Description))
	assert.Equal(t, expected[0].PackageList, pq.StringArray(givenYumPackageGroups[0].PackageList))
	assert.Equal(t, expected[0].HashValue, "daaf7c707f4b5b79d51122ba11314cdb036ba574c9504a7069efdf707d030f09")
}
