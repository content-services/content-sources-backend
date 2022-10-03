package models

import (
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
}
