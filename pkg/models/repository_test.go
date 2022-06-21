package models

import (
	"time"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/stretchr/testify/assert"
)

// func TestRepositorySuite(t *testing.T) {
// 	suite.Run(t, new(ModelsSuite))
// }

func (s *ModelsSuite) TestRepositoryCreate() {
	var now = time.Now()
	var repo = Repository{
		URL:           "https://example.com",
		LastReadTime:  &now,
		LastReadError: nil,
	}
	var found = Repository{}

	tx := db.DB.Create(&repo)
	assert.Nil(s.T(), tx.Error)

	err := tx.Where("url = ?", repo.URL).First(&found).Error
	assert.Nil(s.T(), err)
	assert.NotEmpty(s.T(), found.UUID)
}
