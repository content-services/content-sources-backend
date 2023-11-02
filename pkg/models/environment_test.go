package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type EnvironmentSuite struct {
	*ModelsSuite
}

func TestEnvironmentSuite(t *testing.T) {
	m := ModelsSuite{}
	r := EnvironmentSuite{&m}
	suite.Run(t, &r)
}

func (s *EnvironmentSuite) TestEnvironmentCreate() {
	t := s.T()
	tx := s.tx

	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	environment := environmentTest1.DeepCopy()
	var found = Environment{}
	var err error

	// Create the Repository record
	err = tx.Create(repo).Error
	assert.NoError(t, err)

	// Create the RepositoryConfig record
	repoConfig.RepositoryUUID = repo.Base.UUID
	err = tx.Create(repoConfig).Error
	assert.NoError(t, err)

	// Create the RepositoryEnvironment record
	err = tx.Create(&environment).Error
	assert.NoError(t, err)

	// Create the relationship between Environment and Repository
	var repositories_environments map[string]interface{} = map[string]interface{}{
		"repository_uuid":  repo.UUID,
		"environment_uuid": environment.Base.UUID,
	}
	err = tx.Table(TableNameEnvironmentsRepositories).Create(&repositories_environments).Error
	assert.Nil(t, err)

	// Retrieve the just created record
	found.Base.UUID = environment.UUID
	err = tx.First(&found).Error
	assert.Nil(t, err)
	assert.NotEmpty(t, found.UUID)
	assert.NotEmpty(t, found.ID)
	assert.NotEmpty(t, found.Name)

	// Check the read record is equal to the created one
	assert.Equal(t, environment.ID, found.ID)
	assert.Equal(t, environment.Name, found.Name)
	assert.Equal(t, environment.Description, found.Description)
}

func (s *EnvironmentSuite) TestEnvironmentUpdate() {
	t := s.T()
	tx := s.tx

	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	repoEnvironment := environmentTest1.DeepCopy()
	var found = Environment{}
	var err error

	// Create the Repository record
	err = tx.Create(repo).Error
	assert.Nil(t, err)

	// Create the RepositoryConfig record
	repoConfig.RepositoryUUID = repo.UUID
	err = tx.Create(repoConfig).Error
	assert.Nil(t, err)

	// Create the RepositoryEnvironment record
	tx.Create(&repoEnvironment)
	assert.NotNil(t, tx)
	assert.Nil(t, tx.Error)

	err = tx.Table(TableNameEnvironmentsRepositories).Create([]map[string]interface{}{
		{
			"repository_uuid":  repo.UUID,
			"environment_uuid": repoEnvironment.UUID,
		},
	}).Error
	assert.Nil(t, err)

	// Update RepositoryEnvironment record
	repoEnvironment.CreatedAt = time.Now()
	repoEnvironment.UpdatedAt = time.Now()
	repoEnvironment.ID = "updated-environment"
	repoEnvironment.Name = "updated-environment"
	repoEnvironment.Description = "environment description"

	tx.Save(&repoEnvironment)

	// Retrieve the just created record
	err = tx.Where("uuid = ?", repoEnvironment.UUID).First(&found).Error
	assert.Nil(t, err)
	assert.NotEmpty(t, found.UUID)

	// Check the read record is equal to the updated one
	assert.Equal(t, repoEnvironment.ID, found.ID)
	assert.Equal(t, repoEnvironment.Name, found.Name)
	assert.Equal(t, repoEnvironment.Description, found.Description)
}

func (s *EnvironmentSuite) TestEnvironmentDelete() {
	t := s.T()
	tx := s.tx

	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	repoEnvironment := environmentTest1.DeepCopy()
	var found = Environment{}
	var err error

	// Create the Repository record
	err = tx.Create(repo).Error
	assert.Nil(t, err)

	// Create the RepositoryConfig record
	repoConfig.RepositoryUUID = repo.Base.UUID
	err = tx.Create(repoConfig).Error
	assert.Nil(t, err)

	// Create the RepositoryEnvironment record
	tx.Create(&repoEnvironment)
	assert.NotNil(t, tx)
	assert.Nil(t, tx.Error)

	// Create the many-to-many entry
	err = tx.Table(TableNameEnvironmentsRepositories).Create([]map[string]interface{}{
		{
			"repository_uuid":  repo.UUID,
			"environment_uuid": repoEnvironment.UUID,
		},
	}).Error
	assert.Nil(t, err)

	// Retrieve the just created record
	err = tx.Where("uuid = ?", repoEnvironment.UUID).First(&found).Error
	assert.Nil(t, err)

	// Delete the new record
	err = tx.Delete(&found).Error
	assert.Nil(t, err)

	// Check the record does not exist
	err = tx.Where("uuid = ?", repoEnvironment.UUID).First(&found).Error
	assert.NotNil(t, err)
	assert.Equal(t, "record not found", err.Error())
}

func (t *EnvironmentSuite) TestEnvironmentDeepCopy() {
	copy := environmentTest1.DeepCopy()

	assert.NotNil(t.T(), copy)

	assert.Equal(t.T(), copy.Base.UUID, environmentTest1.Base.UUID)
	assert.Equal(t.T(), copy.Base.CreatedAt, environmentTest1.Base.CreatedAt)
	assert.Equal(t.T(), copy.Base.UpdatedAt, environmentTest1.Base.UpdatedAt)

	assert.Equal(t.T(), copy.ID, environmentTest1.ID)
	assert.Equal(t.T(), copy.Name, environmentTest1.Name)
	assert.Equal(t.T(), copy.Description, environmentTest1.Description)
}

func (s *EnvironmentSuite) TestEnvironmentValidations() {
	t := s.T()
	tx := s.tx

	testID := "test-environment"
	testName := "test-environment"
	testDescription := "description"

	var testCases []struct {
		given    Environment
		expected string
	} = []struct {
		given    Environment
		expected string
	}{
		{
			given: Environment{
				ID:          testID,
				Name:        testName,
				Description: testDescription,
			},
			expected: "",
		},
		{
			given: Environment{
				ID:          testID,
				Name:        "",
				Description: testDescription,
			},
			expected: "Name cannot be empty",
		},
	}

	tx.SavePoint("testenvironmentvalidations")
	for _, item := range testCases {
		err := tx.Create(&item.given).Error
		if item.expected == "" {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
			if err != nil {
				assert.Equal(t, item.expected, err.Error())
			}
		}
		tx.RollbackTo("testenvironmentvalidations")
	}
}
