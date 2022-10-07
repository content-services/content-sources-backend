package models

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type RepositorySuite struct {
	*ModelsSuite
}

func TestRepositorySuite(t *testing.T) {
	m := ModelsSuite{}
	r := RepositorySuite{&m}
	suite.Run(t, &r)
}

func (s *RepositorySuite) TestRepositoriesCreate() {
	var now = time.Now()
	var repo = Repository{
		URL:                    "https://example.com",
		LastIntrospectionTime:  &now,
		LastIntrospectionError: nil,
	}
	var found = Repository{}
	tx := s.tx

	tx.Create(&repo)
	assert.Nil(s.T(), tx.Error)

	err := tx.Where("url = ?", repo.URL).First(&found).Error
	assert.Nil(s.T(), err)
	assert.NotEmpty(s.T(), found.UUID)
	assert.True(s.T(), strings.HasSuffix(found.URL, "/")) // test trailing slash added during creation
}

func (s *ModelsSuite) TestCleanupURL() {
	tx := s.tx
	var found Repository

	type TestCase struct {
		given    string
		expected string
	}

	testCases := []TestCase{
		{
			given:    "https://one.example.com",
			expected: "https://one.example.com/",
		},
		{
			given:    "   https://two.example.com   ",
			expected: "https://two.example.com/",
		},
		{
			given:    "https://three.example.com/path/////",
			expected: "https://three.example.com/path/",
		},
	}

	for i := 0; i < len(testCases); i++ {
		repo := Repository{
			URL: testCases[i].given,
		}

		tx.Create(&repo)
		assert.Nil(s.T(), tx.Error)

		err := tx.Where("url = ?", testCases[i].expected).Find(&found).Error
		assert.Nil(s.T(), err)
		assert.NotEmpty(s.T(), found.UUID)
	}
}
