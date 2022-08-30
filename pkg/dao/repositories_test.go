package dao

import (
	"github.com/stretchr/testify/assert"
)

func (s *RepositorySuite) TestFetchForUrl() {
	tx := s.tx
	t := s.T()

	var (
		err  error
		repo Repository
	)

	urlPublic := s.repo.URL
	dao := GetRepositoryDao(tx)
	err, repo = dao.FetchForUrl(urlPublic)
	assert.NoError(t, err)
	assert.Equal(t, Repository{
		UUID: s.repo.UUID,
		URL:  s.repo.URL,
	}, repo)

	urlPrivate := s.repoPrivate.URL
	err, repo = dao.FetchForUrl(urlPrivate)
	assert.NoError(t, err)
	assert.Equal(t, Repository{
		UUID: s.repoPrivate.UUID,
		URL:  s.repoPrivate.URL,
	}, repo)

	url := "https://it-does-not-exist.com/base"
	err, repo = dao.FetchForUrl(url)
	assert.Error(t, err)
	assert.Equal(t, Repository{
		UUID: "",
		URL:  "",
	}, repo)
}

func (s *RepositorySuite) TestList() {
	tx := s.tx
	t := s.T()

	expected := Repository{
		UUID: s.repo.UUID,
		URL:  s.repo.URL,
	}

	dao := GetRepositoryDao(tx)
	err, repoList := dao.List()
	assert.NoError(t, err)
	assert.Contains(t, repoList, expected)
}

func (s *RepositorySuite) TestUpdateRepository() {
	tx := s.tx
	t := s.T()

	var (
		err  error
		repo Repository
	)

	dao := GetRepositoryDao(tx)
	err, repo = dao.FetchForUrl(s.repo.URL)
	assert.NoError(t, err)
	assert.Equal(t, Repository{
		UUID: s.repo.UUID,
		URL:  s.repo.URL,
	}, repo)

	err = dao.Update(Repository{
		UUID:     s.repo.UUID,
		URL:      s.repo.URL,
		Revision: "123456",
	})
	assert.NoError(t, err)

	err, repo = dao.FetchForUrl(s.repo.URL)
	assert.NoError(t, err)
	assert.Equal(t, Repository{
		UUID:     s.repo.UUID,
		URL:      s.repo.URL,
		Revision: "123456",
	}, repo)
}
