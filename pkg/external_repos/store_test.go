package external_repos

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestExternalRepoSuite(t *testing.T) {
	suite.Run(t, new(ExternalRepoSuite))
}

func (s *ExternalRepoSuite) TestLoadFromFile() {
	extRepos, error := LoadFromFile()
	assert.NoError(s.T(), error)
	assert.NotEmpty(s.T(), extRepos)
}

func (s *ExternalRepoSuite) TestGetBaseURLs() {
	extRepos := []ExternalRepository{{
		BaseUrl: "http://somerepourl/",
	}}
	urls := GetBaseURLs(extRepos)

	assert.Equal(s.T(), []string{"http://somerepourl/"}, urls)
}

func (s *ExternalRepoSuite) TestLoadCA() {
	t := s.T()
	ca, err := LoadCA()
	assert.NoError(t, err)
	assert.Greater(t, len(ca), 0)
}
