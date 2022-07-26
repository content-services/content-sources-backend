package dao

import (
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	scenario0 int = iota
	scenario3
	scenarioUnderThreshold
	scenarioThreshold
	scenarioOverThreshold
)

//
// Implement the unit tests
//

func (s *RpmSuite) TestRpmList() {
	var err error
	t := s.Suite.T()

	// Prepare RepositoryRpm records
	rpm1 := repoRpmTest1.DeepCopy()
	rpm2 := repoRpmTest2.DeepCopy()
	dao := GetRpmDao(s.tx, map[string]interface{}{})

	err = s.tx.Create(&rpm1).Error
	assert.NoError(t, err)
	err = s.tx.Create(&rpm2).Error
	assert.NoError(t, err)
	err = s.tx.Create(&models.RepositoryRpm{
		RepositoryUUID: s.repo.Base.UUID,
		RpmUUID:        rpm1.Base.UUID,
	}).Error
	assert.NoError(t, err)
	err = s.tx.Create(&models.RepositoryRpm{
		RepositoryUUID: s.repo.Base.UUID,
		RpmUUID:        rpm2.Base.UUID,
	}).Error
	assert.NoError(t, err)

	var repoRpmList api.RepositoryRpmCollectionResponse
	var count int64
	repoRpmList, count, err = dao.List(orgIdTest, s.repoConfig.Base.UUID, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, count, int64(2))
	assert.Equal(t, repoRpmList.Meta.Count, count)
}

func (s *RpmSuite) TestRpmSearch() {
	var err error
	t := s.Suite.T()
	tx := s.tx

	// Prepare Rpm records
	urls := []string{
		"https://repo-test-package.org",
		"https://repo-demo-package.org",
		"https://repo-private-package.org",
	}
	rpms := make([]models.Rpm, 4)
	repoRpmTest1.DeepCopyInto(&rpms[0])
	repoRpmTest2.DeepCopyInto(&rpms[1])
	repoRpmTest1.DeepCopyInto(&rpms[2])
	repoRpmTest2.DeepCopyInto(&rpms[3])
	rpms[0].Name = "test-package"
	rpms[1].Name = "demo-package"
	rpms[2].Name = "test-package"
	rpms[3].Name = "demo-package"
	rpms[0].Epoch = 0
	rpms[1].Epoch = 0
	rpms[2].Epoch = 1
	rpms[3].Epoch = 1
	rpms[0].Summary = "test-package Epoch 0"
	rpms[1].Summary = "demo-package Epoch 0"
	rpms[2].Summary = "test-package Epoch 1"
	rpms[3].Summary = "demo-package Epoch 1"
	rpms[0].Checksum = "SHA256:" + uuid.NewString()
	rpms[1].Checksum = "SHA256:" + uuid.NewString()
	rpms[2].Checksum = "SHA256:" + uuid.NewString()
	rpms[3].Checksum = "SHA256:" + uuid.NewString()
	err = tx.Create(&rpms).Error
	require.NoError(t, err)

	// Prepare Repository records
	repositories := make([]models.Repository, 3)
	repoTest1.DeepCopyInto(&repositories[0])
	repoTest1.DeepCopyInto(&repositories[1])
	repoTest1.DeepCopyInto(&repositories[2])
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

	// Prepare relations repositories_rpms
	repositoriesRpms := make([]models.RepositoryRpm, 8)
	repositoriesRpms[0].RepositoryUUID = repositories[0].Base.UUID
	repositoriesRpms[0].RpmUUID = rpms[0].Base.UUID
	repositoriesRpms[1].RepositoryUUID = repositories[0].Base.UUID
	repositoriesRpms[1].RpmUUID = rpms[1].Base.UUID
	repositoriesRpms[2].RepositoryUUID = repositories[1].Base.UUID
	repositoriesRpms[2].RpmUUID = rpms[2].Base.UUID
	repositoriesRpms[3].RepositoryUUID = repositories[1].Base.UUID
	repositoriesRpms[3].RpmUUID = rpms[3].Base.UUID
	// Add rpms to private repository
	repositoriesRpms[4].RepositoryUUID = repositories[2].Base.UUID
	repositoriesRpms[4].RpmUUID = rpms[0].Base.UUID
	repositoriesRpms[5].RepositoryUUID = repositories[2].Base.UUID
	repositoriesRpms[5].RpmUUID = rpms[1].Base.UUID
	repositoriesRpms[6].RepositoryUUID = repositories[2].Base.UUID
	repositoriesRpms[6].RpmUUID = rpms[2].Base.UUID
	repositoriesRpms[7].RepositoryUUID = repositories[2].Base.UUID
	repositoriesRpms[7].RpmUUID = rpms[3].Base.UUID
	err = tx.Create(&repositoriesRpms).Error
	require.NoError(t, err)

	// Test Cases
	type TestCaseGiven struct {
		orgId string
		input api.SearchRpmRequest
		limit int
	}
	type TestCase struct {
		given    TestCaseGiven
		expected []api.SearchRpmResponse
	}
	testCases := []TestCase{
		// The returned items are ordered by epoch
		{
			given: TestCaseGiven{
				orgId: orgIdTest,
				input: api.SearchRpmRequest{
					URLs: []string{
						urls[0],
						urls[1],
					},
					Search: "",
				},
				limit: 50,
			},
			expected: []api.SearchRpmResponse{
				{
					PackageName: "demo-package",
					Summary:     "demo-package Epoch",
				},
				{
					PackageName: "test-package",
					Summary:     "test-package Epoch",
				},
			},
		},
		// The limit is applied correctly, and the order is respected
		{
			given: TestCaseGiven{
				orgId: orgIdTest,
				input: api.SearchRpmRequest{
					URLs: []string{
						urls[0],
						urls[1],
					},
					Search: "",
				},
				limit: 1,
			},
			expected: []api.SearchRpmResponse{
				{
					PackageName: "demo-package",
					Summary:     "demo-package Epoch",
				},
			},
		},
		// Search for the url[2] private repository
		{
			given: TestCaseGiven{
				orgId: orgIdTest,
				input: api.SearchRpmRequest{
					URLs: []string{
						urls[2],
					},
					Search: "",
				},
				limit: 50,
			},
			expected: []api.SearchRpmResponse{
				{
					PackageName: "demo-package",
					Summary:     "demo-package Epoch",
				},
				{
					PackageName: "test-package",
					Summary:     "test-package Epoch",
				},
			},
		},
		// Search for url[0] and url[1] filtering for demo-% packages and it returns 1 entry
		{
			given: TestCaseGiven{
				orgId: orgIdTest,
				input: api.SearchRpmRequest{
					URLs: []string{
						urls[0],
						urls[1],
					},
					Search: "demo-",
				},
				limit: 50,
			},
			expected: []api.SearchRpmResponse{
				{
					PackageName: "demo-package",
					Summary:     "demo-package Epoch",
				},
			},
		},
	}

	// Running all the test cases
	dao := GetRpmDao(tx, map[string]interface{}{
		OptionPagedRpmInsertsLimit: 100,
	})
	for ict, caseTest := range testCases {
		var searchRpmResponse []api.SearchRpmResponse
		searchRpmResponse, err = dao.Search(caseTest.given.orgId, caseTest.given.input, caseTest.given.limit)
		require.NoError(t, err)
		assert.Equal(t, len(caseTest.expected), len(searchRpmResponse))
		for i, expected := range caseTest.expected {
			if i < len(searchRpmResponse) {
				assert.Equal(t, expected.PackageName, searchRpmResponse[i].PackageName, "TestCase: %i; expectedIndex: %i", ict, i)
				assert.Contains(t, searchRpmResponse[i].Summary, expected.Summary, "TestCase: %i; expectedIndex: %i", ict, i)
			}
		}
	}
}

// func (s *RpmSuite) randomPackageName(size int) string {
func randomPackageName(size int) string {
	const lookup string = "0123456789abcdefghijklmnopqrstuvwxyz"
	return seeds.RandStringWithTable(size, lookup)
}

// func (s *RpmSuite) randomHexadecimal(size int) string {
func randomHexadecimal(size int) string {
	const lookup string = "0123456789abcdef"
	return seeds.RandStringWithTable(size, lookup)
}

// func (s *RpmSuite) randomYumPackage() yum.Package {
func randomYumPackage(pkg *yum.Package) {
	if pkg == nil {
		return
	}
	pkgName := randomPackageName(32)
	pkg.Name = pkgName
	pkg.Arch = "x86_64"
	pkg.Summary = pkgName + " summary"
	pkg.Version = yum.Version{
		Version: "1.0.0",
		Release: "dev",
		Epoch:   0,
	}
	pkg.Type = "rpm"
	pkg.Checksum = yum.Checksum{
		Type:  "sha256",
		Value: randomHexadecimal(64),
	}
}

func (s *RpmSuite) preparePagedRpmInsert(scenario int, limit int) []yum.Package {
	switch scenario {
	case scenario0:
		{
			return []yum.Package{}
		}
	case scenario3:
		// The reason of this scenario is to make debugging easier
		{
			pkgs := make([]yum.Package, 3)
			for i := 0; i < 3; i++ {
				randomYumPackage(&pkgs[i])
			}
			return pkgs
		}
	case scenarioUnderThreshold:
		{
			pkgs := make([]yum.Package, limit-1)
			for i := 0; i < limit-1; i++ {
				randomYumPackage(&pkgs[i])
			}
			return pkgs
		}
	case scenarioThreshold:
		{
			pkgs := make([]yum.Package, limit)
			for i := 0; i < limit; i++ {
				randomYumPackage(&pkgs[i])
			}
			return pkgs
		}
	case scenarioOverThreshold:
		{
			pkgs := make([]yum.Package, limit+1)
			for i := 0; i < limit+1; i++ {
				randomYumPackage(&pkgs[i])
			}
			return pkgs
		}
	default:
		{
			return []yum.Package{}
		}
	}
}

func (s *RpmSuite) TestRpmSearchError() {
	var err error
	t := s.Suite.T()
	tx := s.tx
	txSP := strings.ToLower("TestRpmSearchError")

	var searchRpmResponse []api.SearchRpmResponse
	dao := GetRpmDao(tx, map[string]interface{}{
		OptionPagedRpmInsertsLimit: 100,
	})
	tx.SavePoint(txSP)

	searchRpmResponse, err = dao.Search("", api.SearchRpmRequest{Search: "", URLs: []string{"https:/noreturn.org"}}, 100)
	require.Error(t, err)
	assert.Equal(t, int(0), len(searchRpmResponse))
	assert.Equal(t, err.Error(), "orgID can not be an empty string")
	tx.RollbackTo(txSP)

	searchRpmResponse, err = dao.Search(orgIdTest, api.SearchRpmRequest{Search: ""}, 100)
	require.Error(t, err)
	assert.Equal(t, int(0), len(searchRpmResponse))
	assert.Equal(t, err.Error(), "request.URLs must contain at least 1 URL")
	tx.RollbackTo(txSP)
}

type TestInsertForRepositoryCase struct {
	given    int
	expected string
}

var testCases []TestInsertForRepositoryCase = []TestInsertForRepositoryCase{
	{
		given:    scenario0,
		expected: "empty slice found",
	},
	{
		given:    scenario3,
		expected: "",
	},
	{
		given:    scenarioUnderThreshold,
		expected: "",
	},
	{
		given:    scenarioThreshold,
		expected: "",
	},
	{
		given:    scenarioOverThreshold,
		expected: "",
	},
}

func (s *RpmSuite) genericInsertForRepository(testCase TestInsertForRepositoryCase) {
	t := s.Suite.T()
	tx := s.tx

	pagedRpmInsertsLimit := 100
	dao := GetRpmDao(tx, map[string]interface{}{
		OptionPagedRpmInsertsLimit: pagedRpmInsertsLimit,
	})

	p := s.preparePagedRpmInsert(testCase.given, pagedRpmInsertsLimit)
	records, err := dao.InsertForRepository(s.repo.Base.UUID, p)

	if testCase.expected != "" {
		assert.Error(t, err)
		assert.Contains(t, err.Error(), testCase.expected)
	} else {
		assert.NoError(t, err)
		assert.Equal(t, int64(len(p)), records)
	}
}

func (s *RpmSuite) TestInsertForRepositoryScenario0() {
	s.genericInsertForRepository(testCases[scenario0])
}

func (s *RpmSuite) TestInsertForRepositoryScenario3() {
	s.genericInsertForRepository(testCases[scenario3])
}

func (s *RpmSuite) TestInsertForRepositoryScenarioUnderThreshold() {
	s.genericInsertForRepository(testCases[scenarioUnderThreshold])
}
func (s *RpmSuite) TestInsertForRepositoryScenarioThreshold() {
	s.genericInsertForRepository(testCases[scenarioThreshold])
}
func (s *RpmSuite) TestInsertForRepositoryScenarioOverThreshold() {
	s.genericInsertForRepository(testCases[scenarioOverThreshold])
}

func (s *RpmSuite) TestInsertForRepositoryWithExistingChecksums() {
	t := s.Suite.T()
	tx := s.tx

	pagedRpmInsertsLimit := 100

	dao := GetRpmDao(tx, map[string]interface{}{
		OptionPagedRpmInsertsLimit: pagedRpmInsertsLimit,
	})
	p := s.preparePagedRpmInsert(scenarioThreshold, pagedRpmInsertsLimit)
	records, err := dao.InsertForRepository(s.repo.Base.UUID, p[0:(pagedRpmInsertsLimit>>1)])
	assert.NoError(t, err)
	assert.Equal(t, records, int64(len(p[0:(pagedRpmInsertsLimit>>1)])))

	records, err = dao.InsertForRepository(s.repo.Base.UUID, p[(pagedRpmInsertsLimit>>1):])
	assert.NoError(t, err)
	assert.Equal(t, records, int64(len(p[(pagedRpmInsertsLimit>>1):])))

	records, err = dao.InsertForRepository(s.repo.Base.UUID, p[0:(pagedRpmInsertsLimit>>1)])
	assert.NoError(t, err)
	assert.Equal(t, records, int64(0))
}

func (s *RpmSuite) TestInsertForRepositoryWithWrongRepoUUID() {
	t := s.Suite.T()
	tx := s.tx

	pagedRpmInsertsLimit := 100

	dao := GetRpmDao(tx, map[string]interface{}{
		OptionPagedRpmInsertsLimit: pagedRpmInsertsLimit,
	})
	p := s.preparePagedRpmInsert(scenario3, pagedRpmInsertsLimit)
	records, err := dao.InsertForRepository(uuid.NewString(), p)

	assert.Error(t, err)
	assert.Equal(t, records, int64(0))
}
