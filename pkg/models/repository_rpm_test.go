package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestRepositoryRpmSuite(t *testing.T) {
	m := ModelsSuite{}
	r := RepositoryRpmSuite{&m}
	suite.Run(t, &r)
}

func (s *RepositoryRpmSuite) TestRepositoriesRpmsValidations() {
	t := s.T()
	tx := s.tx

	const spName = "testrepositoriesrpmsvalidations"

	testRepository := Repository{
		URL:                    "https://example.com",
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
	}
	testRpm := Rpm{
		Name:     "test-package",
		Arch:     "x86_64",
		Version:  "1.3.0",
		Release:  "",
		Epoch:    0,
		Summary:  "test package",
		Checksum: "SHA256:934e8895f778a2e31d2a65cba048a4085537fc819a8acd40b534bf98e1e42ffd",
	}

	var err error

	err = tx.Create(&testRepository).Error
	assert.NoError(t, err)

	err = tx.Create(&testRpm).Error
	assert.NoError(t, err)

	var testCases []struct {
		given    RepositoryRpm
		expected string
	} = []struct {
		given    RepositoryRpm
		expected string
	}{
		{
			given: RepositoryRpm{
				RepositoryUUID: testRepository.UUID,
				RpmUUID:        testRpm.UUID,
			},
			expected: "",
		},
		{
			given: RepositoryRpm{
				RepositoryUUID: "",
				RpmUUID:        testRpm.UUID,
			},
			expected: "RepositoryUUID cannot be empty",
		},
		{
			given: RepositoryRpm{
				RepositoryUUID: testRepository.UUID,
				RpmUUID:        "",
			},
			expected: "RpmUUID cannot be empty",
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
