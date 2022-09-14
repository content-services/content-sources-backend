package models

import (
	"time"

	"github.com/stretchr/testify/assert"
)

func (s *ModelsSuite) TestRepositoriesCreate() {
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
