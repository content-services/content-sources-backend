package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/stretchr/testify/assert"
)

//
// Implement the unit tests
//

func (s *RepositoryRpmSuite) TestRepositoryRpmCreate() {
	t := s.T()
	var err error

	repoRpm := repoRpmTest1.DeepCopy()
	assert.NotNil(t, repoRpm)

	// TODO Refactor in a struct slice and extract the validation
	//      scenarios in a new test case
	// Get dao instance out of the transaction
	var dao repositoryRpmDaoImpl
	dao = GetRepositoryRpmDao(s.db)

	// Check validations
	err = dao.Create("", accountIdTest, repoRpm)
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "OrgID can not be an empty string")

	err = dao.Create(orgIdTest, "", repoRpm)
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "AccountId can not be an empty string")

	err = dao.Create(orgIdTest, accountIdTest, nil)
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "It can not create a nil RepositoryRpm record")

	repoRpm.ReferRepo = ""
	err = dao.Create(orgIdTest, accountIdTest, repoRpm)
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "The referenced repository uuid can not be an empty string")

	// Get a new RepositoryRpmDao but using the transaction
	dao = GetRepositoryRpmDao(s.tx)
	repoRpm.ReferRepo = s.repo.Base.UUID
	err = dao.Create(orgIdTest, accountIdTest, repoRpm)
	assert.Nil(t, err)
}

func (s *RepositoryRpmSuite) TestRepositoryRpmFetch() {
	t := s.T()
	var err error

	repoRpm := repoRpmTest1.DeepCopy()
	assert.NotNil(t, repoRpm)
	dao := GetRepositoryRpmDao(s.tx)

	// Create a new RepositoryRpm record to be retrieved later
	repoRpmNew := repoRpmTest1.DeepCopy()
	repoRpmNew.ReferRepo = s.repo.Base.UUID
	err = dao.Create(orgIdTest, accountIdTest, repoRpmNew)
	assert.Nil(t, err)

	var repoRpmApiFetched *api.RepositoryRpm
	repoRpmApiFetched, err = dao.Fetch(orgIdTest, accountIdTest, repoRpmNew.Base.UUID)
	assert.Nil(t, err)
	assert.NotNil(t, repoRpmApiFetched)
	assert.Equal(t, repoRpmNew.Base.UUID, repoRpmApiFetched.UUID)
	assert.Equal(t, repoRpmNew.Name, repoRpmApiFetched.Name)
	assert.Equal(t, repoRpmNew.Arch, repoRpmApiFetched.Arch)
	assert.Equal(t, repoRpmNew.Version, repoRpmApiFetched.Version)
	assert.Equal(t, repoRpmNew.Release, repoRpmApiFetched.Release)
	assert.Equal(t, repoRpmNew.Summary, repoRpmApiFetched.Summary)
	assert.Equal(t, repoRpmNew.Description, repoRpmApiFetched.Description)
}

func (s *RepositoryRpmSuite) TestRepositoryRpmList() {
	var err error
	t := s.Suite.T()

	// Prepare RepositoryRpm records
	repoRpm1 := repoRpmTest1.DeepCopy()
	repoRpm1.ReferRepo = s.repo.Base.UUID
	repoRpm2 := repoRpmTest2.DeepCopy()
	repoRpm2.ReferRepo = s.repo.Base.UUID
	dao := GetRepositoryRpmDao(s.tx)

	// Create a new RepositoryRpm record to be retrieved later
	err = dao.Create(orgIdTest, accountIdTest, repoRpm1)
	assert.Nil(t, err)
	err = dao.Create(orgIdTest, accountIdTest, repoRpm2)
	assert.Nil(t, err)

	var repoRpmList api.RepositoryRpmCollectionResponse
	var count int64
	repoRpmList, count, err = dao.List(orgIdTest, accountIdTest, s.repo.Base.UUID, 0, 0)
	assert.Equal(t, count, int64(2))
	assert.Equal(t, repoRpmList.Meta.Count, count)
}
