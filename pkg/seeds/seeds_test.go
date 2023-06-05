package seeds

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// TestSeedSuite Launch the test suite
func TestSeedSuite(t *testing.T) {
	suite.Run(t, new(SeedSuite))
}

func (s *SeedSuite) TestSeedRepositoryConfigurations() {
	t := s.T()
	tx := s.tx

	err := SeedRepositoryConfigurations(tx, 101, SeedOptions{
		BatchSize: 100,
		OrgID:     RandomOrgId(),
	})
	assert.Nil(t, err, "Error seeding RepositoryConfigurations")
}

func (s *SeedSuite) TestSeedRepository() {
	t := s.T()
	var err error
	tx := s.tx

	err = SeedRepository(tx, 505, SeedOptions{})
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

func (s *SeedSuite) TestSeedTasks() {
	t := s.T()
	var err error
	orgId := RandomOrgId()
	tx := s.tx

	err = SeedTasks(tx, 505, TaskSeedOptions{
		OrgID: orgId,
	})
	assert.NoError(t, err, "Error seeding Tasks")

	task := []models.TaskInfo{}
	err = tx.Limit(1).Find(&task).Error
	assert.NoError(t, err)
	assert.Greater(t, len(task), 0)
}
