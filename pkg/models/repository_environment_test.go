package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestRepositoryEnvironmentSuite(t *testing.T) {
	m := ModelsSuite{}
	r := RepositoryEnvironmentSuite{&m}
	suite.Run(t, &r)
}

func (s *RepositoryEnvironmentSuite) TestRepositoriesEnvironmentsValidations() {
	t := s.T()
	tx := s.tx

	const spName = "testrepositoriesenvironmentsvalidations"

	testRepository := Repository{
		URL:                    "https://example.com",
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
	}
	testEnvironment := Environment{
		ID:          "test-environment",
		Name:        "test-environment",
		Description: "test environment description",
	}

	var err error

	err = tx.Create(&testRepository).Error
	assert.NoError(t, err)

	err = tx.Create(&testEnvironment).Error
	assert.NoError(t, err)

	var testCases []struct {
		given    RepositoryEnvironment
		expected string
	} = []struct {
		given    RepositoryEnvironment
		expected string
	}{
		{
			given: RepositoryEnvironment{
				RepositoryUUID:  testRepository.UUID,
				EnvironmentUUID: testEnvironment.UUID,
			},
			expected: "",
		},
		{
			given: RepositoryEnvironment{
				RepositoryUUID:  "",
				EnvironmentUUID: testEnvironment.UUID,
			},
			expected: "RepositoryUUID cannot be empty",
		},
		{
			given: RepositoryEnvironment{
				RepositoryUUID:  testRepository.UUID,
				EnvironmentUUID: "",
			},
			expected: "EnvironmentUUID cannot be empty",
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
