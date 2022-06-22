package seeds

import (
	"github.com/stretchr/testify/assert"
)

func (s *SeedSuite) TestSeedRepositoryConfigurations() {
	t := s.T()
	var err error
	tx := s.tx

	err = SeedRepositoryConfigurations(tx, 1001, SeedOptions{
		OrgID: "acme",
	})
	assert.Nil(t, err, "Error seeding RepositoryConfigurations")
}

func (s *SeedSuite) TestSeedRepository() {
	t := s.T()
	var err error
	tx := s.tx

	err = SeedRepositoryConfigurations(tx, 5, SeedOptions{
		OrgID: "acme",
	})
	assert.Nil(t, err, "Error seeding RepositoryConfigurations")

	err = SeedRepository(tx, 505)
	assert.Nil(t, err, "Error seeding Repositories")
}

func (s *SeedSuite) TestSeedRepositoryRpms() {
	t := s.T()
	var err error
	tx := s.tx

	err = SeedRepositoryConfigurations(tx, 5, SeedOptions{
		OrgID: "acme",
	})
	assert.Nil(t, err, "Error seeding RepositoryConfigurations")

	err = SeedRepository(tx, 5)
	assert.Nil(t, err, "Error seeding Repositories")

	err = SeedRepositoryRpms(tx, 505)
	assert.Nil(t, err, "Error seeding RepositoryRpms")
}
