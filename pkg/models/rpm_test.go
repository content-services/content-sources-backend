package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type RpmSuite struct {
	*ModelsSuite
}

func TestRpmSuite(t *testing.T) {
	m := ModelsSuite{}
	r := RpmSuite{&m}
	suite.Run(t, &r)
}

func (s *RpmSuite) TestRpmCreate() {
	t := s.T()
	tx := s.tx

	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	rpm := rpmTest1.DeepCopy()
	var found = Rpm{}
	var err error

	// Create the Repository record
	err = tx.Create(repo).Error
	assert.NoError(t, err)

	// Create the RepositoryConfig record
	repoConfig.RepositoryUUID = repo.Base.UUID
	err = tx.Create(repoConfig).Error
	assert.NoError(t, err)

	// Create the RepositoryRpm record
	err = tx.Create(&rpm).Error
	assert.NoError(t, err)

	// Create the ralationship between Rpm and Repository
	var repositories_rpms map[string]interface{} = map[string]interface{}{
		"repository_uuid": repo.UUID,
		"rpm_uuid":        rpm.Base.UUID,
	}
	err = tx.Table(TableNameRpmsRepositories).Create(&repositories_rpms).Error
	assert.Nil(t, err)

	// Retrieve the just created record
	found.Base.UUID = rpm.UUID
	err = tx.First(&found).Error
	assert.Nil(t, err)
	assert.NotEmpty(t, found.UUID)
	assert.NotEmpty(t, found.Name)
	assert.NotEmpty(t, found.Arch)
	assert.NotEmpty(t, found.Version)
	assert.NotEmpty(t, found.Release)
	assert.Equal(t, int32(0), found.Epoch)
	assert.NotEmpty(t, found.Summary)

	// Check the read record is equal to the created one
	assert.Equal(t, rpm.Name, found.Name)
	assert.Equal(t, rpm.Arch, found.Arch)
	assert.Equal(t, rpm.Summary, found.Summary)
	assert.Equal(t, rpm.Version, found.Version)
	assert.Equal(t, rpm.Release, found.Release)
	assert.Equal(t, rpm.Epoch, found.Epoch)
}

func (s *RpmSuite) TestRpmUpdate() {
	t := s.T()
	tx := s.tx

	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	repoRpm := rpmTest1.DeepCopy()
	var found = Rpm{}
	var err error

	// Create the Repository record
	err = tx.Create(repo).Error
	assert.Nil(t, err)

	// Create the RepositoryConfig record
	repoConfig.RepositoryUUID = repo.UUID
	err = tx.Create(repoConfig).Error
	assert.Nil(t, err)

	// Create the RepositoryRpm record
	tx.Create(&repoRpm)
	assert.NotNil(t, tx)
	assert.Nil(t, tx.Error)

	err = tx.Table(TableNameRpmsRepositories).Create([]map[string]interface{}{
		{
			"repository_uuid": repo.UUID,
			"rpm_uuid":        repoRpm.UUID,
		},
	}).Error
	assert.Nil(t, err)

	// Update RepositoryRpm record
	repoRpm.CreatedAt = time.Now()
	repoRpm.UpdatedAt = time.Now()
	repoRpm.Name = "updated-package"
	repoRpm.Arch = "noarch"
	repoRpm.Version = "0.2.3"
	repoRpm.Release = "12312"
	repoRpm.Epoch = 1
	repoRpm.Summary = "Updated summary"

	tx.Save(&repoRpm)

	// Retrieve the just created record
	err = tx.Where("uuid = ?", repoRpm.UUID).First(&found).Error
	assert.Nil(t, err)
	assert.NotEmpty(t, found.UUID)

	// Check the read record is equal to the updated one
	assert.Equal(t, repoRpm.Name, found.Name)
	assert.Equal(t, repoRpm.Arch, found.Arch)
	assert.Equal(t, repoRpm.Summary, found.Summary)
	assert.Equal(t, repoRpm.Version, found.Version)
	assert.Equal(t, repoRpm.Release, found.Release)
	assert.Equal(t, repoRpm.Epoch, found.Epoch)
}

func (s *RpmSuite) TestRpmDelete() {
	t := s.T()
	tx := s.tx

	repoConfig := repoConfigTest1.DeepCopy()
	repo := repoTest1.DeepCopy()
	repoRpm := rpmTest1.DeepCopy()
	var found = Rpm{}
	var err error

	// Create the Repository record
	err = tx.Create(repo).Error
	assert.Nil(t, err)

	// Create the RepositoryConfig record
	repoConfig.RepositoryUUID = repo.Base.UUID
	err = tx.Create(repoConfig).Error
	assert.Nil(t, err)

	// Create the RepositoryRpm record
	tx.Create(&repoRpm)
	assert.NotNil(t, tx)
	assert.Nil(t, tx.Error)

	// Create the mant-to-many entry
	err = tx.Table(TableNameRpmsRepositories).Create([]map[string]interface{}{
		{
			"repository_uuid": repo.UUID,
			"rpm_uuid":        repoRpm.UUID,
		},
	}).Error
	assert.Nil(t, err)

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

func (t *RpmSuite) TestRpmDeepCopy() {
	copy := rpmTest1.DeepCopy()

	assert.NotNil(t.T(), copy)

	assert.Equal(t.T(), copy.Base.UUID, rpmTest1.Base.UUID)
	assert.Equal(t.T(), copy.Base.CreatedAt, rpmTest1.Base.CreatedAt)
	assert.Equal(t.T(), copy.Base.UpdatedAt, rpmTest1.Base.UpdatedAt)

	assert.Equal(t.T(), copy.Name, rpmTest1.Name)
	assert.Equal(t.T(), copy.Arch, rpmTest1.Arch)
	assert.Equal(t.T(), copy.Version, rpmTest1.Version)
	assert.Equal(t.T(), copy.Release, rpmTest1.Release)
	assert.Equal(t.T(), copy.Epoch, rpmTest1.Epoch)
	assert.Equal(t.T(), copy.Summary, rpmTest1.Summary)
}

func (s *RpmSuite) TestRpmValidations() {
	t := s.T()
	tx := s.tx

	testName := "test-package"
	testArch := "x86_64"
	testVersion := "1.3.0"
	testRelease := ""
	testEpoch := 0
	testSummary := "test package"
	testChecksum := "SHA256:934e8895f778a2e31d2a65cba048a4085537fc819a8acd40b534bf98e1e42ffd"

	var testCases []struct {
		given    Rpm
		expected string
	} = []struct {
		given    Rpm
		expected string
	}{
		{
			given: Rpm{
				Name:     testName,
				Arch:     testArch,
				Version:  testVersion,
				Release:  testRelease,
				Epoch:    int32(testEpoch),
				Summary:  testSummary,
				Checksum: testChecksum,
			},
			expected: "",
		},
		{
			given: Rpm{
				Name:     "",
				Arch:     testArch,
				Version:  testVersion,
				Release:  testRelease,
				Epoch:    int32(testEpoch),
				Summary:  testSummary,
				Checksum: testChecksum,
			},
			expected: "Name cannot be empty",
		},
		{
			given: Rpm{
				Name:     testName,
				Arch:     "",
				Version:  testVersion,
				Release:  testRelease,
				Epoch:    int32(testEpoch),
				Summary:  testSummary,
				Checksum: testChecksum,
			},
			expected: "Arch cannot be empty",
		},
		{
			given: Rpm{
				Name:     testName,
				Arch:     testArch,
				Version:  "",
				Release:  testRelease,
				Epoch:    int32(testEpoch),
				Summary:  testSummary,
				Checksum: testChecksum,
			},
			expected: "Version cannot be empty",
		},
		{
			given: Rpm{
				Name:     testName,
				Arch:     testArch,
				Version:  testVersion,
				Release:  testRelease,
				Epoch:    -1,
				Summary:  testSummary,
				Checksum: testChecksum,
			},
			expected: "Epoch cannot be lower than 0",
		},
		{
			given: Rpm{
				Name:     testName,
				Arch:     testArch,
				Version:  testVersion,
				Release:  testRelease,
				Epoch:    int32(testEpoch),
				Summary:  "",
				Checksum: testChecksum,
			},
			expected: "Summary cannot be empty",
		},
		{
			given: Rpm{
				Name:     testName,
				Arch:     testArch,
				Version:  testVersion,
				Release:  testRelease,
				Epoch:    int32(testEpoch),
				Summary:  testSummary,
				Checksum: "",
			},
			expected: "Sha256 cannot be empty",
		},
	}

	tx.SavePoint("testrpmvalidations")
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
		tx.RollbackTo("testrpmvalidations")
	}
}
