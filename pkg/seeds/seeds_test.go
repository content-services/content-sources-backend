package seeds

import (
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/stretchr/testify/assert"
)

func (s *SeedSuite) TestSeedRepositoryConfigurations() {
	t := s.T()
	tx := s.tx

	err := SeedRepositoryConfigurations(tx, 1001, SeedOptions{
		OrgID: RandomOrgId(),
	})
	assert.Nil(t, err, "Error seeding RepositoryConfigurations")
}

func (s *SeedSuite) TestSeedRepository() {
	t := s.T()
	var err error
	tx := s.tx

	err = SeedRepository(tx, 505)
	assert.Nil(t, err, "Error seeding Repositories")
}

func (s *SeedSuite) TestSeedRpms() {
	t := s.T()
	var err error
	org_id := RandomOrgId()
	tx := s.tx

	err = SeedRepositoryConfigurations(tx, 505, SeedOptions{
		OrgID: org_id,
	})
	assert.Nil(t, err, "Error seeding Repositories")

	var repo = []models.Repository{}
	err = tx.Limit(1).Find(&repo).Error
	assert.Nil(t, err)
	assert.Greater(t, len(repo), 0)

	err = SeedRpms(tx, &repo[0], 505)
	assert.Nil(t, err, "Error seeding RepositoryRpms")
}
