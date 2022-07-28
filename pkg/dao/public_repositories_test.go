package dao

import (
	"github.com/stretchr/testify/assert"
)

func (s *PublicRepositorySuite) TestFetchForUrl() {
	tx := s.tx
	t := s.T()

	url := s.repo.URL
	dao := GetPublicRepositoryDao(tx)
	err, repo := dao.FetchForUrl(url)
	assert.NoError(t, err)
	assert.Equal(t, PublicRepository{
		UUID: s.repo.UUID,
		URL:  s.repo.URL,
	}, repo)

	url = "https://it-does-not-exist.com/base"
	err, repo = dao.FetchForUrl(url)
	assert.Error(t, err)
	assert.Equal(t, PublicRepository{
		UUID: "",
		URL:  "",
	}, repo)
}

func (s *PublicRepositorySuite) TestList() {
	tx := s.tx
	t := s.T()

	expected := []PublicRepository{
		{
			UUID: s.repo.UUID,
			URL:  s.repo.URL,
		},
	}

	dao := GetPublicRepositoryDao(tx)
	err, repoList := dao.List()
	assert.NoError(t, err)
	assert.Equal(t, int(1), len(repoList))
	assert.Equal(t, expected, repoList)
}
