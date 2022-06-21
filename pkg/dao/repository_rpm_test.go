package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/stretchr/testify/assert"
)

//
// Implement the unit tests
//

func (t *DaoSuite) TestRepositoryRpmCreate() {
	repoRpm := repoRpmTest1.DeepCopy()

	var err error
	s := t.Suite.T()
	err = GetRepositoryRpmDao(t.db).Create("", accountIdTest, repoRpm)
	assert.NotNil(s, err)
	assert.Equal(s, err.Error(), "OrgID can not be an empty string")

	err = GetRepositoryRpmDao(t.db).Create(orgIdTest, "", repoRpm)
	assert.NotNil(s, err)
	assert.Equal(s, err.Error(), "AccountId can not be an empty string")

	err = GetRepositoryRpmDao(t.db).Create(orgIdTest, accountIdTest, nil)
	assert.NotNil(s, err)
	assert.Equal(s, err.Error(), "It can not create a nil RepositoryRpm record")

	repoRpm.ReferRepo = ""
	err = GetRepositoryRpmDao(t.db).Create(orgIdTest, accountIdTest, repoRpm)
	assert.NotNil(s, err)
	assert.Equal(s, err.Error(), "The referenced repository uuid can not be an empty string")

	repoRpm.ReferRepo = repoTest1.Base.UUID
	err = GetRepositoryRpmDao(t.db).Create(orgIdTest, accountIdTest, repoRpm)
	assert.Nil(s, err)
}

func (t *DaoSuite) TestRepositoryRpmFetch() {
	var err error
	s := t.Suite.T()

	// Create a new RepositoryRpm record to be retrieved later
	repoRpmNew := repoRpmTest1.DeepCopy()
	repoRpmNew.ReferRepo = repoTest1.UUID
	err = GetRepositoryRpmDao(t.db).Create(orgIdTest, accountIdTest, repoRpmNew)
	assert.Nil(s, err)

	var repoRpmApiFetched *api.RepositoryRpm
	repoRpmApiFetched, err = GetRepositoryRpmDao(t.db).Fetch(orgIdTest, accountIdTest, repoRpmNew.Base.UUID)
	assert.Nil(s, err)
	assert.NotNil(s, repoRpmApiFetched)
	assert.Equal(s, repoRpmNew.Base.UUID, repoRpmApiFetched.UUID)
	assert.Equal(s, repoRpmNew.Name, repoRpmApiFetched.Name)
	assert.Equal(s, repoRpmNew.Arch, repoRpmApiFetched.Arch)
	assert.Equal(s, repoRpmNew.Version, repoRpmApiFetched.Version)
	assert.Equal(s, repoRpmNew.Release, repoRpmApiFetched.Release)
	assert.Equal(s, repoRpmNew.Summary, repoRpmApiFetched.Summary)
	assert.Equal(s, repoRpmNew.Description, repoRpmApiFetched.Description)
}
