package dao

import (
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type RepositorySuite struct {
	*DaoSuite
	repoConfig  *models.RepositoryConfiguration
	repo        *models.Repository
	repoPrivate *models.Repository
}

func (s *RepositorySuite) SetupTest() {
	if db.DB == nil {
		if err := db.Connect(); err != nil {
			s.FailNow(err.Error())
		}
	}
	s.db = db.DB.Session(&gorm.Session{
		SkipDefaultTransaction: true,
		Logger: logger.New(
			log.New(os.Stderr, "\r\n", log.LstdFlags),
			logger.Config{
				LogLevel: logger.Info,
			}),
	})
	s.tx = s.db.Begin()

	repo := repoPublicTest.DeepCopy()
	if err := s.tx.Create(repo).Error; err != nil {
		s.FailNow("Preparing Repository record UUID=" + repo.UUID)
	}
	s.repo = repo

	repoConfig := repoConfigTest1.DeepCopy()
	repoConfig.RepositoryUUID = repo.Base.UUID
	if err := s.tx.Create(repoConfig).Error; err != nil {
		s.FailNow("Preparing RepositoryConfiguration record UUID=" + repoConfig.UUID)
	}
	s.repoConfig = repoConfig

	repoPrivate := repoPrivateTest.DeepCopy()
	if err := s.tx.Create(&repoPrivate).Error; err != nil {
		s.FailNow(err.Error())
	}
	s.repoPrivate = repoPrivate
}

func TestRepositorySuite(t *testing.T) {
	m := DaoSuite{}
	r := RepositorySuite{DaoSuite: &m}
	suite.Run(t, &r)
}

func (s *RepositorySuite) TestFetchForUrl() {
	tx := s.tx
	t := s.T()

	var (
		err  error
		repo Repository
	)

	urlPublic := s.repo.URL
	dao := GetRepositoryDao(tx)
	repo, err = dao.FetchForUrl(urlPublic)
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

	// Trim the trailing slash, and verify we still find the repo
	noSlashUrl := strings.TrimSuffix(urlPublic, "/")
	assert.NotEqual(t, noSlashUrl, urlPublic)
	repo, err = dao.FetchForUrl(noSlashUrl)
	assert.NoError(t, err)
	assert.Equal(t, s.repo.UUID, repo.UUID)

	urlPrivate := s.repoPrivate.URL
	repo, err = dao.FetchForUrl(urlPrivate)
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
	repo, err = dao.FetchForUrl(url)
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
	repoList, err := dao.List()
	assert.NoError(t, err)
	assert.Contains(t, repoList, expected)
}

func (s *RepositorySuite) TestOrphanCleanup() {
	dao := GetRepositoryDao(s.tx)

	unusedExpired := models.Repository{
		Base: models.Base{
			CreatedAt: time.Now().AddDate(0, 0, -8),
		},
		URL:    "http://expired.example.com",
		Public: false,
	}
	unusedCurrent := models.Repository{
		Base: models.Base{
			CreatedAt: time.Now().AddDate(0, 0, -3),
		},
		URL:    "http://currents.example.com",
		Public: false,
	}
	unusedPublic := models.Repository{
		Base: models.Base{
			CreatedAt: time.Now().AddDate(0, 0, -8),
		},
		URL:    "http://public.example.com",
		Public: true,
	}
	usedExpired := models.Repository{
		Base: models.Base{
			CreatedAt: time.Now().AddDate(0, 0, -8),
		},
		URL:    "http://used-expired.example.com",
		Public: false,
	}
	tx := s.tx.Create(&unusedExpired)
	assert.NoError(s.T(), tx.Error)
	tx = s.tx.Create(&unusedCurrent)
	assert.NoError(s.T(), tx.Error)
	tx = s.tx.Create(&unusedPublic)
	assert.NoError(s.T(), tx.Error)
	tx = s.tx.Create(&usedExpired)
	assert.NoError(s.T(), tx.Error)

	usedExpireConfig := models.RepositoryConfiguration{
		Name:           "this one is used, even though its older than 7 days",
		OrgID:          "asdf",
		RepositoryUUID: usedExpired.UUID,
	}
	tx = s.tx.Create(&usedExpireConfig)
	assert.NoError(s.T(), tx.Error)

	err := dao.OrphanCleanup()
	assert.NoError(s.T(), err)

	count := int64(0)
	s.tx.Model(&unusedExpired).Where("uuid = ?", unusedExpired.UUID).Count(&count)
	assert.Equal(s.T(), int64(0), count)

	s.tx.Model(&unusedCurrent).Where("uuid = ?", unusedCurrent.UUID).Count(&count)
	assert.Equal(s.T(), int64(1), count)

	s.tx.Model(&unusedPublic).Where("uuid = ?", unusedPublic.UUID).Count(&count)
	assert.Equal(s.T(), int64(1), count)

	s.tx.Model(&usedExpired).Where("uuid = ?", usedExpired.UUID).Count(&count)
	assert.Equal(s.T(), int64(1), count)
}
func (s *RepositorySuite) TestUpdateRepository() {
	tx := s.tx
	t := s.T()

	var (
		err  error
		repo Repository
	)

	dao := GetRepositoryDao(tx)
	repo, err = dao.FetchForUrl(s.repo.URL)
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
		RepomdChecksum:               pointy.String("123456"),
		LastIntrospectionTime:        &expectedTimestamp,
		LastIntrospectionSuccessTime: &expectedTimestamp,
		LastIntrospectionUpdateTime:  &expectedTimestamp,
		LastIntrospectionError:       pointy.String("expected error"),
		PackageCount:                 pointy.Int(123),
		Status:                       pointy.String(config.StatusUnavailable),
	}

	err = dao.Update(expected)
	assert.NoError(t, err)

	repo, err = dao.FetchForUrl(s.repo.URL)
	assert.NoError(t, err)
	assert.Equal(t, expected.UUID, repo.UUID)
	assert.Equal(t, *expected.URL, repo.URL)
	assert.Equal(t, "123456", repo.RepomdChecksum)
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionTime.Format("060102"))
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionUpdateTime.Format("060102"))
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionSuccessTime.Format("060102"))
	assert.Equal(t, expected.LastIntrospectionError, repo.LastIntrospectionError)
	assert.Equal(t, config.StatusUnavailable, repo.Status)
	assert.Equal(t, 123, repo.PackageCount)

	// Test that it updates zero values but not nil values
	zeroValues := RepositoryUpdate{
		UUID:           s.repo.UUID,
		URL:            &s.repo.URL,
		RepomdChecksum: pointy.String(""),
	}

	err = dao.Update(zeroValues)
	assert.NoError(t, err)

	repo, err = dao.FetchForUrl(s.repo.URL)
	assert.NoError(t, err)
	assert.Equal(t, s.repo.UUID, repo.UUID)
	assert.Equal(t, s.repo.URL, repo.URL)
	assert.Equal(t, *zeroValues.RepomdChecksum, repo.RepomdChecksum)
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionTime.Format("060102"))
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionUpdateTime.Format("060102"))
	assert.Equal(t, expectedTimestamp.Format("060102"), repo.LastIntrospectionSuccessTime.Format("060102"))
	assert.Equal(t, expected.LastIntrospectionError, repo.LastIntrospectionError)
	assert.Equal(t, *expected.PackageCount, repo.PackageCount)
	assert.Equal(t, *expected.Status, repo.Status)
}

func (s *RepositorySuite) TestFetchRpmCount() {
	tx := s.tx
	t := s.T()
	var err error
	expected := 20
	err = seeds.SeedRpms(tx, s.repo, expected)
	assert.Nil(t, err, "Error seeding Rpms")

	dao := GetRepositoryDao(tx)
	count, err := dao.FetchRepositoryRPMCount(s.repo.UUID)
	assert.NoError(t, err)
	assert.Equal(t, expected, count)
}
