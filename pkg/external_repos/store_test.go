package external_repos

import "github.com/stretchr/testify/assert"

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
