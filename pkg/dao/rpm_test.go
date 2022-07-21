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

//
// Implement the unit tests
//

func (s *RpmSuite) TestRpmList() {
	var err error
	t := s.Suite.T()

	// Prepare RepositoryRpm records
	rpm1 := repoRpmTest1.DeepCopy()
	rpm2 := repoRpmTest2.DeepCopy()
	dao := GetRpmDao(s.tx)

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
	dao := GetRpmDao(tx)
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

const (
	scenario0 int = iota
	scenario3
	scenarioUnder500
	scenario500
	scenarioOver500
	scenarioLen
)

func (s *RpmSuite) randomPackageName(size int) string {
	const lookup string = "0123456789abcdefghijklmnopqrstuvwxyz"
	return seeds.RandStringWithTable(size, lookup)
}

func (s *RpmSuite) randomHexadecimal(size int) string {
	const lookup string = "0123456789abcdef"
	return seeds.RandStringWithTable(size, lookup)
}

func (s *RpmSuite) randomYumPackage() yum.Package {
	pkgName := s.randomPackageName(32)
	return yum.Package{
		Name:    pkgName,
		Arch:    "x86_64",
		Summary: pkgName + " summary",
		Version: yum.Version{
			Version: "1.0.0",
			Release: "dev",
			Epoch:   0,
		},
		Type: "rpm",
		Checksum: yum.Checksum{
			Type:  "sha256",
			Value: s.randomHexadecimal(64),
		},
	}
}

func (s *RpmSuite) preparePagedRpmInsert(scenario int) []yum.Package {
	var pkgs []yum.Package
	switch scenario {
	case scenario0:
		{
			return pkgs
		}
	case scenario3:
		// The reason of this scenario is to make debugging easier
		{
			for i := 0; i < 3; i++ {
				pkgs = append(pkgs, s.randomYumPackage())
			}
			return pkgs
		}
	case scenarioUnder500:
		{
			for i := 0; i < 499; i++ {
				pkgs = append(pkgs, s.randomYumPackage())
			}
			return pkgs
		}
	case scenario500:
		{
			for i := 0; i < 500; i++ {
				pkgs = append(pkgs, s.randomYumPackage())
			}
			return pkgs
		}
	case scenarioOver500:
		{
			for i := 0; i < 501; i++ {
				pkgs = append(pkgs, s.randomYumPackage())
			}
			return pkgs
		}
	default:
		{
			return pkgs
		}
	}
}

func (s *RpmSuite) TestRpmSearchError() {
	var err error
	t := s.Suite.T()
	tx := s.tx
	txSP := strings.ToLower("TestRpmSearchError")

	var searchRpmResponse []api.SearchRpmResponse
	dao := GetRpmDao(tx)
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

func (s *RpmSuite) TestInsertForRepository() {
	const spName = "testinsertforrepository"
	t := s.Suite.T()
	tx := s.tx

	type TestCase struct {
		given    int
		expected string
	}
	var testCases []TestCase = []TestCase{
		{
			given:    scenario0,
			expected: "empty slice found",
		},
		{
			given:    scenario3,
			expected: "",
		},
		{
			given:    scenarioUnder500,
			expected: "",
		},
		{
			given:    scenario500,
			expected: "",
		},
		{
			given:    scenarioOver500,
			expected: "",
		},
	}

	tx.SavePoint(spName)
	dao := GetRpmDao(tx)
	for _, testCase := range testCases {
		pkgs := s.preparePagedRpmInsert(testCase.given)
		records, err := dao.InsertForRepository(s.repo.Base.UUID, pkgs)

		if testCase.expected != "" {
			assert.Error(t, err)
			assert.Contains(t, err.Error(), testCase.expected)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, records, int64(len(pkgs)))
		}
		tx.RollbackTo(spName)
	}
}
