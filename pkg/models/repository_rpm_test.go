package models

import (
	"time"

	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
)

// func TestRepoRpmSuite(t *testing.T) {
// 	suite.Run(t, new(ModelsSuite))
// }

func (s *ModelsSuite) TestRepositoryRpmCreate() {
	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	repoRpm := repoRpmTest1.DeepCopy()
	var found = RepositoryRpm{}
	var err error

	t := s.T()
	tx := s.tx

	// Create the RepositoryConfig record
	err = tx.Create(repoConfig).Error
	assert.Nil(t, err)

	// Create the Repository record
	repo.ReferRepoConfig = &repoConfig.UUID
	err = tx.Create(repo).Error
	assert.Nil(t, err)

	// Create the RepositoryRpm record
	repoRpm.ReferRepo = repo.UUID
	tx.Create(&repoRpm)
	assert.NotNil(t, tx)
	assert.Nil(t, tx.Error)

	// Retrieve the just created record
	err = tx.Where("uuid = ?", repoRpm.UUID).First(&found).Error
	assert.Nil(t, err)
	assert.NotEmpty(t, found.UUID)

	// Check the read record is equal to the created one
	assert.Equal(t, repoRpm.Name, found.Name)
	assert.Equal(t, repoRpm.Arch, found.Arch)
	assert.Equal(t, repoRpm.Summary, found.Summary)
	assert.Equal(t, repoRpm.Description, found.Description)
	assert.Equal(t, repoRpm.Version, found.Version)
	assert.Equal(t, repoRpm.Release, found.Release)
	if repoRpm.Epoch == nil {
		assert.Nil(t, found.Epoch)
	} else {
		assert.NotNil(t, found.Epoch)
		assert.Equal(t, *repoRpm.Epoch, *found.Epoch)
	}
}

func (s *ModelsSuite) TestRepositoryRpmUpdate() {
	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	repoRpm := repoRpmTest1.DeepCopy()
	var found = RepositoryRpm{}
	var err error

	t := s.T()
	tx := s.tx

	// Create the RepositoryConfig record
	err = tx.Create(repoConfig).Error
	assert.Nil(t, err)

	// Create the Repository record
	repo.ReferRepoConfig = &repoConfig.UUID
	err = tx.Create(repo).Error
	assert.Nil(t, err)

	// Create the RepositoryRpm record
	repoRpm.ReferRepo = repo.UUID
	tx.Create(&repoRpm)
	assert.NotNil(t, tx)
	assert.Nil(t, tx.Error)

	// Update RepositoryRpm record
	repoRpm.CreatedAt = time.Now()
	repoRpm.UpdatedAt = time.Now()
	repoRpm.Name = "updated-package"
	repoRpm.Arch = "noarch"
	repoRpm.Version = "0.2.3"
	repoRpm.Release = "12312"
	repoRpm.Epoch = pointy.Int32(1)
	repoRpm.Summary = "Updated summary"
	repoRpm.Description = "Updated description"

	tx.Save(&repoRpm)

	// Retrieve the just created record
	err = tx.Where("uuid = ?", repoRpm.UUID).First(&found).Error
	assert.Nil(t, err)
	assert.NotEmpty(t, found.UUID)

	// Check the read record is equal to the updated one
	assert.Equal(t, repoRpm.Name, found.Name)
	assert.Equal(t, repoRpm.Arch, found.Arch)
	assert.Equal(t, repoRpm.Summary, found.Summary)
	assert.Equal(t, repoRpm.Description, found.Description)
	assert.Equal(t, repoRpm.Version, found.Version)
	assert.Equal(t, repoRpm.Release, found.Release)
	if repoRpm.Epoch == nil {
		assert.Nil(t, found.Epoch)
	} else {
		assert.NotNil(t, found.Epoch)
		assert.Equal(t, *repoRpm.Epoch, *found.Epoch)
	}
}

func (s *ModelsSuite) TestRepositoryRpmDelete() {
	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	repoRpm := repoRpmTest1.DeepCopy()
	var found = RepositoryRpm{}
	var err error

	t := s.T()
	tx := s.tx

	// Create the RepositoryConfig record
	err = tx.Create(repoConfig).Error
	assert.Nil(t, err)

	// Create the Repository record
	repo.ReferRepoConfig = &repoConfig.UUID
	err = tx.Create(repo).Error
	assert.Nil(t, err)

	// Create the RepositoryRpm record
	repoRpm.ReferRepo = repo.UUID
	tx.Create(&repoRpm)
	assert.NotNil(t, tx)
	assert.Nil(t, tx.Error)

	// Retrieve the just created record
	err = tx.Where("uuid = ?", repoRpm.UUID).First(&found).Error
	assert.Nil(t, err)

	// Delete the new record
	err = tx.Delete(&found).Error
	assert.Nil(t, err)

	// Check the record does not exist
	err = tx.Where("uuid = ?", repoRpm.UUID).First(&found).Error
	assert.NotNil(t, err)
	assert.Equal(t, "record not found", err.Error())
}

func (t *ModelsSuite) TestRepoRpmDeepCopy() {
	copy := repoRpmTest1.DeepCopy()

	assert.NotNil(t.T(), copy)

	assert.Equal(t.T(), copy.Base.UUID, repoRpmTest1.Base.UUID)
	assert.Equal(t.T(), copy.Base.CreatedAt, repoRpmTest1.Base.CreatedAt)
	assert.Equal(t.T(), copy.Base.UpdatedAt, repoRpmTest1.Base.UpdatedAt)

	assert.Equal(t.T(), copy.Name, repoRpmTest1.Name)
	assert.Equal(t.T(), copy.Arch, repoRpmTest1.Arch)
	assert.Equal(t.T(), copy.Version, repoRpmTest1.Version)
	assert.Equal(t.T(), copy.Release, repoRpmTest1.Release)
	if copy.Epoch != nil && repoRpmTest1.Epoch != nil {
		assert.Equal(t.T(), *copy.Epoch, *repoRpmTest1.Epoch)
	} else {
		assert.Nil(t.T(), *copy.Epoch)
		assert.Nil(t.T(), *repoRpmTest1.Epoch)
	}
	assert.Equal(t.T(), copy.Summary, repoRpmTest1.Summary)
	assert.Equal(t.T(), copy.Description, repoRpmTest1.Description)
}
