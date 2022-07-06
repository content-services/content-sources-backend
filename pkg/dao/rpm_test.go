package dao

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/stretchr/testify/assert"
)

//
// Implement the unit tests
//

func (s *RpmSuite) TestRpmCreate() {
	t := s.T()
	tx := s.tx
	var err error

	rpmData1 := repoRpmTest1.DeepCopy()
	rpmData2 := repoRpmTest2.DeepCopy()

	type testCases struct {
		given struct {
			orgId      string
			accountId  string
			repository *models.Repository
			rpm        *models.Rpm
		}
		expected string
	}

	var cases []testCases = []testCases{
		{
			given: struct {
				orgId      string
				accountId  string
				repository *models.Repository
				rpm        *models.Rpm
			}{
				orgId:      "",
				accountId:  accountIdTest,
				repository: s.repo,
				rpm:        rpmData1,
			},
			expected: fmt.Sprintf("repository_uuid = %s is not owned", s.repo.UUID),
		},
		{
			given: struct {
				orgId      string
				accountId  string
				repository *models.Repository
				rpm        *models.Rpm
			}{
				orgId:      orgIdTest,
				accountId:  accountIdTest,
				repository: nil,
				rpm:        rpmData1,
			},
			expected: "repo can not be nil",
		},
	}
	txSavePoint := s.tx.SavePoint("testrpmcreate")
	for _, item := range cases {
		dao := GetRpmDao(txSavePoint)
		err = dao.Create(
			item.given.orgId,
			item.given.repository,
			item.given.rpm)

		if item.expected == "" {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
			if err != nil {
				assert.Equal(t, err.Error(), item.expected)
			}
		}
		txSavePoint = txSavePoint.RollbackTo("testrpmcreate")
	}

	// Create two different records
	dao := GetRpmDao(tx)
	assert.NotNil(t, dao)

	err = dao.Create(orgIdTest, s.repo, rpmData1)
	assert.NoError(t, err)

	err = dao.Create(orgIdTest, s.repo, rpmData2)
	assert.NoError(t, err)
}

func (s *RpmSuite) TestRpmFetch() {
	t := s.T()
	var err error

	// Create a new RepositoryRpm record to be retrieved later
	repoRpmNew := repoRpmTest1.DeepCopy()
	dao := GetRpmDao(s.tx)

	err = dao.Create(orgIdTest, s.repo, repoRpmNew)
	assert.NoError(t, err)

	var repoRpmApiFetched *api.RepositoryRpm
	repoRpmApiFetched, err = dao.Fetch(orgIdTest, repoRpmNew.Base.UUID)
	assert.NoError(t, err)
	assert.NotNil(t, repoRpmApiFetched)
	assert.Equal(t, repoRpmNew.Base.UUID, repoRpmApiFetched.UUID)
	assert.Equal(t, repoRpmNew.Name, repoRpmApiFetched.Name)
	assert.Equal(t, repoRpmNew.Arch, repoRpmApiFetched.Arch)
	assert.Equal(t, repoRpmNew.Version, repoRpmApiFetched.Version)
	assert.Equal(t, repoRpmNew.Release, repoRpmApiFetched.Release)
	assert.Equal(t, repoRpmNew.Summary, repoRpmApiFetched.Summary)
}

func (s *RpmSuite) TestRpmList() {
	var err error
	t := s.Suite.T()

	// Prepare RepositoryRpm records
	rpm1 := repoRpmTest1.DeepCopy()
	rpm2 := repoRpmTest2.DeepCopy()
	dao := GetRpmDao(s.tx)

	// Create a new RepositoryRpm record to be retrieved later
	err = dao.Create(orgIdTest, s.repo, rpm1)
	assert.NoError(t, err)
	err = dao.Create(orgIdTest, s.repo, rpm2)
	assert.NoError(t, err)

	var repoRpmList api.RepositoryRpmCollectionResponse
	var count int64
	repoRpmList, count, err = dao.List(orgIdTest, s.repo.Base.UUID, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, count, int64(2))
	assert.Equal(t, repoRpmList.Meta.Count, count)
}
