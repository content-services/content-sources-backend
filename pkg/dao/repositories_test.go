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
		Status:                       s.repo.Status,
		LastIntrospectionTime:        s.repo.LastIntrospectionTime,
		LastIntrospectionUpdateTime:  s.repo.LastIntrospectionUpdateTime,
		LastIntrospectionSuccessTime: s.repo.LastIntrospectionSuccessTime,
		LastIntrospectionError:       s.repo.LastIntrospectionError,
		PackageCount:                 s.repo.PackageCount,
	}, repo)

	urlPrivate := s.repoPrivate.URL
	err, repo = dao.FetchForUrl(urlPrivate)
	assert.NoError(t, err)
	assert.Equal(t, Repository{
		UUID:                         s.repoPrivate.UUID,
		URL:                          s.repoPrivate.URL,
		Status:                       s.repoPrivate.Status,
		LastIntrospectionTime:        s.repoPrivate.LastIntrospectionTime,
		LastIntrospectionUpdateTime:  s.repoPrivate.LastIntrospectionUpdateTime,
		LastIntrospectionSuccessTime: s.repoPrivate.LastIntrospectionSuccessTime,
		LastIntrospectionError:       s.repoPrivate.LastIntrospectionError,
		PackageCount:                 s.repoPrivate.PackageCount,
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
		Status:                       s.repo.Status,
		LastIntrospectionTime:        s.repo.LastIntrospectionTime,
		LastIntrospectionUpdateTime:  s.repo.LastIntrospectionUpdateTime,
		LastIntrospectionSuccessTime: s.repo.LastIntrospectionSuccessTime,
		LastIntrospectionError:       s.repo.LastIntrospectionError,
		PackageCount:                 s.repo.PackageCount,
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
		PackageCount:                 s.repo.PackageCount,
	}, repo)

	expectedTimestamp := time.Now()
	expected := RepositoryUpdate{
		UUID:                         s.repo.UUID,
		URL:                          pointy.String(s.repo.URL),
		Revision:                     pointy.String("123456"),
		LastIntrospectionTime:        &expectedTimestamp,
		LastIntrospectionSuccessTime: &expectedTimestamp,
		LastIntrospectionUpdateTime:  &expectedTimestamp,
		LastIntrospectionError:       pointy.String("expected error"),
		PackageCount:                 pointy.Int(123),
		Status:                       pointy.String(config.StatusUnavailable),
	}

	err = dao.Update(expected)
	assert.NoError(t, err)

	err, repo = dao.FetchForUrl(s.repo.URL)
	assert.NoError(t, err)
	assert.Equal(t, expected.UUID, repo.UUID)
	assert.Equal(t, *expected.URL, repo.URL)
	assert.Equal(t, "123456", repo.Revision)
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionTime.Format("060102"))
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionUpdateTime.Format("060102"))
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionSuccessTime.Format("060102"))
	assert.Equal(t, expected.LastIntrospectionError, repo.LastIntrospectionError)
	assert.Equal(t, config.StatusUnavailable, repo.Status)
	assert.Equal(t, 123, repo.PackageCount)

	// Test that it updates zero values but not nil values
	zeroValues := RepositoryUpdate{
		UUID:     s.repo.UUID,
		URL:      &s.repo.URL,
		Revision: pointy.String(""),
	}

	err = dao.Update(zeroValues)
	assert.NoError(t, err)

	err, repo = dao.FetchForUrl(s.repo.URL)
	assert.NoError(t, err)
	assert.Equal(t, s.repo.UUID, repo.UUID)
	assert.Equal(t, s.repo.URL, repo.URL)
	assert.Equal(t, *zeroValues.Revision, repo.Revision)
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionTime.Format("060102"))
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionUpdateTime.Format("060102"))
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionSuccessTime.Format("060102"))
	assert.Equal(t, expected.LastIntrospectionError, repo.LastIntrospectionError)
	assert.Equal(t, *expected.PackageCount, repo.PackageCount)
	assert.Equal(t, *expected.Status, repo.Status)
}
