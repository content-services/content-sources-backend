package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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

func (s *RpmSuite) TestInsertForRepositoryWithExistingChecksums() {
	t := s.Suite.T()
	tx := s.tx

	dao := GetRpmDao(tx)
	pkgs := s.preparePagedRpmInsert(scenario500)
	records, err := dao.InsertForRepository(s.repo.Base.UUID, pkgs[0:250])
	assert.NoError(t, err)
	assert.Equal(t, records, int64(len(pkgs[0:250])))
	records, err = dao.InsertForRepository(s.repo.Base.UUID, pkgs[250:])
	assert.NoError(t, err)
	assert.Equal(t, records, int64(len(pkgs[250:])))
	records, err = dao.InsertForRepository(s.repo.Base.UUID, pkgs[0:250])
	assert.NoError(t, err)
	assert.Equal(t, records, int64(0))
}

func (s *RpmSuite) TestInsertForRepositoryWithWrongRepoUUID() {
	t := s.Suite.T()
	tx := s.tx

	dao := GetRpmDao(tx)
	pkgs := s.preparePagedRpmInsert(scenario3)
	records, err := dao.InsertForRepository(uuid.NewString(), pkgs)

	assert.Error(t, err)
	assert.Equal(t, records, int64(0))

}
