package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PackageGroupSuite struct {
	*ModelsSuite
}

func TestPackageGroupSuite(t *testing.T) {
	m := ModelsSuite{}
	r := PackageGroupSuite{&m}
	suite.Run(t, &r)
}

func (s *PackageGroupSuite) TestPackageGroupCreate() {
	t := s.T()
	tx := s.tx

	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	packageGroup := packageGroupTest1.DeepCopy()
	var found = PackageGroup{}
	var err error

	// Create the Repository record
	err = tx.Create(repo).Error
	assert.NoError(t, err)

	// Create the RepositoryConfig record
	repoConfig.RepositoryUUID = repo.Base.UUID
	err = tx.Create(repoConfig).Error
	assert.NoError(t, err)

	// Create the RepositoryPackageGroup record
	err = tx.Create(&packageGroup).Error
	assert.NoError(t, err)

	// Create the relationship between PackageGroup and Repository
	var repositories_package_groups map[string]interface{} = map[string]interface{}{
		"repository_uuid":    repo.UUID,
		"package_group_uuid": packageGroup.Base.UUID,
	}
	err = tx.Table(TableNamePackageGroupsRepositories).Create(&repositories_package_groups).Error
	assert.Nil(t, err)

	// Retrieve the just created record
	found.Base.UUID = packageGroup.UUID
	err = tx.First(&found).Error
	assert.Nil(t, err)
	assert.NotEmpty(t, found.UUID)
	assert.NotEmpty(t, found.ID)
	assert.NotEmpty(t, found.Name)

	// Check the read record is equal to the created one
	assert.Equal(t, packageGroup.ID, found.ID)
	assert.Equal(t, packageGroup.Name, found.Name)
	assert.Equal(t, packageGroup.Description, found.Description)
	assert.Equal(t, packageGroup.PackageList, found.PackageList)
}

func (s *PackageGroupSuite) TestPackageGroupUpdate() {
	t := s.T()
	tx := s.tx

	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	repoPackageGroup := packageGroupTest1.DeepCopy()
	var found = PackageGroup{}
	var err error

	// Create the Repository record
	err = tx.Create(repo).Error
	assert.Nil(t, err)

	// Create the RepositoryConfig record
	repoConfig.RepositoryUUID = repo.UUID
	err = tx.Create(repoConfig).Error
	assert.Nil(t, err)

	// Create the RepositoryPackageGroup record
	tx.Create(&repoPackageGroup)
	assert.NotNil(t, tx)
	assert.Nil(t, tx.Error)

	err = tx.Table(TableNamePackageGroupsRepositories).Create([]map[string]interface{}{
		{
			"repository_uuid":    repo.UUID,
			"package_group_uuid": repoPackageGroup.UUID,
		},
	}).Error
	assert.Nil(t, err)

	// Update RepositoryPackageGroup record
	repoPackageGroup.CreatedAt = time.Now()
	repoPackageGroup.UpdatedAt = time.Now()
	repoPackageGroup.ID = "updated-package-group"
	repoPackageGroup.Name = "updated-package-group"
	repoPackageGroup.Description = "package group description"
	repoPackageGroup.PackageList = []string{"package"}

	tx.Save(&repoPackageGroup)

	// Retrieve the just created record
	err = tx.Where("uuid = ?", repoPackageGroup.UUID).First(&found).Error
	assert.Nil(t, err)
	assert.NotEmpty(t, found.UUID)

	// Check the read record is equal to the updated one
	assert.Equal(t, repoPackageGroup.ID, found.ID)
	assert.Equal(t, repoPackageGroup.Name, found.Name)
	assert.Equal(t, repoPackageGroup.Description, found.Description)
	assert.Equal(t, repoPackageGroup.PackageList, found.PackageList)
}

func (s *PackageGroupSuite) TestPackageGroupDelete() {
	t := s.T()
	tx := s.tx

	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	repoPackageGroup := packageGroupTest1.DeepCopy()
	var found = PackageGroup{}
	var err error

	// Create the Repository record
	err = tx.Create(repo).Error
	assert.Nil(t, err)

	// Create the RepositoryConfig record
	repoConfig.RepositoryUUID = repo.Base.UUID
	err = tx.Create(repoConfig).Error
	assert.Nil(t, err)

	// Create the RepositoryPackageGroup record
	tx.Create(&repoPackageGroup)
	assert.NotNil(t, tx)
	assert.Nil(t, tx.Error)

	// Create the many-to-many entry
	err = tx.Table(TableNamePackageGroupsRepositories).Create([]map[string]interface{}{
		{
			"repository_uuid":    repo.UUID,
			"package_group_uuid": repoPackageGroup.UUID,
		},
	}).Error
	assert.Nil(t, err)

	// Retrieve the just created record
	err = tx.Where("uuid = ?", repoPackageGroup.UUID).First(&found).Error
	assert.Nil(t, err)

	// Delete the new record
	err = tx.Delete(&found).Error
	assert.Nil(t, err)

	// Check the record does not exist
	err = tx.Where("uuid = ?", repoPackageGroup.UUID).First(&found).Error
	assert.NotNil(t, err)
	assert.Equal(t, "record not found", err.Error())
}

func (t *PackageGroupSuite) TestPackageGroupDeepCopy() {
	copy := packageGroupTest1.DeepCopy()

	assert.NotNil(t.T(), copy)

	assert.Equal(t.T(), copy.Base.UUID, packageGroupTest1.Base.UUID)
	assert.Equal(t.T(), copy.Base.CreatedAt, packageGroupTest1.Base.CreatedAt)
	assert.Equal(t.T(), copy.Base.UpdatedAt, packageGroupTest1.Base.UpdatedAt)

	assert.Equal(t.T(), copy.ID, packageGroupTest1.ID)
	assert.Equal(t.T(), copy.Name, packageGroupTest1.Name)
	assert.Equal(t.T(), copy.Description, packageGroupTest1.Description)
	assert.Equal(t.T(), copy.PackageList, packageGroupTest1.PackageList)
}

func (s *PackageGroupSuite) TestPackageGroupValidations() {
	t := s.T()
	tx := s.tx

	testID := "test-package-group"
	testName := "test-package-group"
	testDescription := "description"
	testPackageList := []string{"package"}

	var testCases []struct {
		given    PackageGroup
		expected string
	} = []struct {
		given    PackageGroup
		expected string
	}{
		{
			given: PackageGroup{
				ID:          testID,
				Name:        testName,
				Description: testDescription,
				PackageList: testPackageList,
			},
			expected: "",
		},
		{
			given: PackageGroup{
				ID:          testID,
				Name:        "",
				Description: testDescription,
				PackageList: testPackageList,
			},
			expected: "Name cannot be empty",
		},
	}

	tx.SavePoint("testpackagegroupvalidations")
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
		tx.RollbackTo("testpackagegroupvalidations")
	}
}
