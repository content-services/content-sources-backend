package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestRepositoryPackageGroupSuite(t *testing.T) {
	m := ModelsSuite{}
	r := RepositoryPackageGroupSuite{&m}
	suite.Run(t, &r)
}

func (s *RepositoryPackageGroupSuite) TestRepositoriesPackageGroupsValidations() {
	t := s.T()
	tx := s.tx

	const spName = "testrepositoriespackagegroupsvalidations"

	testRepository := Repository{
		URL:                    "https://example.com",
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
	}
	testPackageGroup := PackageGroup{
		ID:          "test-package-group",
		Name:        "test-package-group",
		Description: "test package group description",
		PackageList: []string{"package1"},
	}

	var err error

	err = tx.Create(&testRepository).Error
	assert.NoError(t, err)

	err = tx.Create(&testPackageGroup).Error
	assert.NoError(t, err)

	var testCases []struct {
		given    RepositoryPackageGroup
		expected string
	} = []struct {
		given    RepositoryPackageGroup
		expected string
	}{
		{
			given: RepositoryPackageGroup{
				RepositoryUUID:   testRepository.UUID,
				PackageGroupUUID: testPackageGroup.UUID,
			},
			expected: "",
		},
		{
			given: RepositoryPackageGroup{
				RepositoryUUID:   "",
				PackageGroupUUID: testPackageGroup.UUID,
			},
			expected: "RepositoryUUID cannot be empty",
		},
		{
			given: RepositoryPackageGroup{
				RepositoryUUID:   testRepository.UUID,
				PackageGroupUUID: "",
			},
			expected: "PackageGroupUUID cannot be empty",
		},
	}

	tx.SavePoint(spName)
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
		tx.RollbackTo(spName)
	}
}
