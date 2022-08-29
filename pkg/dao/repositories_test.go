package dao

import (
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/openlyinc/pointy"
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
		UUID:                         s.repo.UUID,
		URL:                          s.repo.URL,
		Public:                       s.repo.Public,
		Status:                       s.repo.Status,
		LastIntrospectionTime:        s.repo.LastIntrospectionTime,
		LastIntrospectionUpdateTime:  s.repo.LastIntrospectionUpdateTime,
		LastIntrospectionSuccessTime: s.repo.LastIntrospectionSuccessTime,
		LastIntrospectionError:       s.repo.LastIntrospectionError,
	}, repo)

	urlPrivate := s.repoPrivate.URL
	err, repo = dao.FetchForUrl(urlPrivate)
	assert.NoError(t, err)
	assert.Equal(t, Repository{
		UUID:                         s.repoPrivate.UUID,
		URL:                          s.repoPrivate.URL,
		Public:                       s.repoPrivate.Public,
		Status:                       s.repo.Status,
		LastIntrospectionTime:        s.repo.LastIntrospectionTime,
		LastIntrospectionUpdateTime:  s.repo.LastIntrospectionUpdateTime,
		LastIntrospectionSuccessTime: s.repo.LastIntrospectionSuccessTime,
		LastIntrospectionError:       s.repo.LastIntrospectionError,
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
		UUID:                         s.repo.UUID,
		URL:                          s.repo.URL,
		Public:                       s.repo.Public,
		Status:                       s.repo.Status,
		LastIntrospectionTime:        s.repo.LastIntrospectionTime,
		LastIntrospectionUpdateTime:  s.repo.LastIntrospectionUpdateTime,
		LastIntrospectionSuccessTime: s.repo.LastIntrospectionSuccessTime,
		LastIntrospectionError:       s.repo.LastIntrospectionError,
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
		UUID:                         s.repo.UUID,
		URL:                          s.repo.URL,
		Status:                       s.repo.Status,
		LastIntrospectionTime:        s.repo.LastIntrospectionTime,
		LastIntrospectionUpdateTime:  s.repo.LastIntrospectionUpdateTime,
		LastIntrospectionSuccessTime: s.repo.LastIntrospectionSuccessTime,
		LastIntrospectionError:       s.repo.LastIntrospectionError,
		Public:                       s.repo.Public,
	}, repo)

	expectedTimestamp := time.Now()
	expected := Repository{
		UUID:                         s.repo.UUID,
		URL:                          s.repo.URL,
		Revision:                     "123456",
		Public:                       !s.repo.Public,
		LastIntrospectionTime:        &expectedTimestamp,
		LastIntrospectionSuccessTime: &expectedTimestamp,
		LastIntrospectionUpdateTime:  &expectedTimestamp,
		LastIntrospectionError:       pointy.String("expected error"),
		Status:                       config.StatusUnavailable,
	}

	err = dao.Update(expected)
	assert.NoError(t, err)

	err, repo = dao.FetchForUrl(s.repo.URL)
	assert.NoError(t, err)
	assert.Equal(t, expected.UUID, repo.UUID)
	assert.Equal(t, expected.URL, repo.URL)
	assert.Equal(t, expected.Public, repo.Public)
	assert.Equal(t, "123456", repo.Revision)
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionTime.Format("060102"))
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionUpdateTime.Format("060102"))
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionSuccessTime.Format("060102"))
	assert.Equal(t, expected.LastIntrospectionError, repo.LastIntrospectionError)
	assert.Equal(t, config.StatusUnavailable, repo.Status)
}
