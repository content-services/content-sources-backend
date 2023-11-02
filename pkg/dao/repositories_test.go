package dao

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/google/uuid"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		FailedIntrospectionsCount:    s.repo.FailedIntrospectionsCount,
		Public:                       s.repo.Public,
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
		FailedIntrospectionsCount:    s.repoPrivate.FailedIntrospectionsCount,
		Public:                       s.repoPrivate.Public,
	}, repo)

	url := "https://it-does-not-exist.com/base"
	repo, err = dao.FetchForUrl(url)
	assert.Error(t, err)
	assert.Equal(t, Repository{
		UUID: "",
		URL:  "",
	}, repo)
}

func (s *RepositorySuite) TestListPublic() {
	tx := s.tx
	t := s.T()

	tx.SavePoint("testlistpublic")
	tx.Exec("TRUNCATE repositories, snapshots, repositories_rpms, repositories_package_groups, repositories_environments, repository_configurations")

	dao := GetRepositoryDao(tx)
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	err := tx.Create(s.repo).Error
	require.NoError(t, err)
	err = tx.Create(s.repoPrivate).Error
	require.NoError(t, err)

	repos, totalRepos, err := dao.ListPublic(pageData, api.FilterData{})
	assert.NoError(t, err)
	assert.Len(t, repos.Data, 1)
	assert.Equal(t, repos.Data[0].URL, s.repo.URL)
	assert.Equal(t, int64(1), totalRepos)

	tx.RollbackTo("testlistpublic")
}

func (s *RepositorySuite) TestListPublicNoRepositories() {
	tx := s.tx
	t := s.T()

	tx.SavePoint("testlistpublic")
	tx.Exec("TRUNCATE repositories, snapshots, repositories_rpms, repositories_package_groups, repositories_environments, repository_configurations")

	dao := GetRepositoryDao(tx)
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	err := tx.Create(s.repoPrivate).Error
	require.NoError(t, err)

	repos, totalRepos, err := dao.ListPublic(pageData, api.FilterData{})
	assert.NoError(t, err)
	assert.Len(t, repos.Data, 0)
	assert.Equal(t, int64(0), totalRepos)

	tx.RollbackTo("testlistpublic")
}

func (s *RepositorySuite) TestListPageLimit() {
	tx := s.tx
	t := s.T()

	tx.SavePoint("testlistpublic")
	tx.Exec("TRUNCATE repositories, snapshots, repositories_rpms, repositories_package_groups, repositories_environments, repository_configurations")

	dao := GetRepositoryDao(tx)
	pageData := api.PaginationData{
		Limit:  1,
		Offset: 0,
	}
	repo2 := repoPublicTest.DeepCopy()
	repo2.URL = "https://public2.example.com"
	err := tx.Create(repo2).Error
	require.NoError(t, err)
	err = tx.Create(s.repo).Error
	require.NoError(t, err)
	err = tx.Create(s.repoPrivate).Error
	require.NoError(t, err)

	repos, totalRepos, err := dao.ListPublic(pageData, api.FilterData{})
	assert.NoError(t, err)
	assert.Len(t, repos.Data, 1)
	assert.Equal(t, int64(2), totalRepos)

	tx.RollbackTo("testlistpublic")
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
		FailedIntrospectionsCount:    s.repo.FailedIntrospectionsCount,
		Public:                       s.repo.Public,
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
		FailedIntrospectionsCount:    pointy.Int(30),
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
	assert.Equal(t, 30, repo.FailedIntrospectionsCount)

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
	assert.Equal(t, *expected.FailedIntrospectionsCount, repo.FailedIntrospectionsCount)
	assert.Equal(t, *expected.Status, repo.Status)

	errorMsg := ""
	for i := 0; i < 300; i++ {
		errorMsg = errorMsg + "a"
	}
	// Test that trims introspection error
	err = dao.Update(RepositoryUpdate{
		UUID:                   s.repo.UUID,
		LastIntrospectionError: pointy.Pointer(errorMsg[0:254]),
	})
	assert.NoError(t, err)

	err = dao.Update(RepositoryUpdate{
		UUID:                   s.repo.UUID,
		LastIntrospectionError: pointy.Pointer(errorMsg[0:255]),
	})
	assert.NoError(t, err)

	err = dao.Update(RepositoryUpdate{
		UUID:                   s.repo.UUID,
		LastIntrospectionError: pointy.Pointer(errorMsg[0:256]),
	})
	assert.NoError(t, err)
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

func (s *RepositorySuite) TestListRepositoriesForIntrospection() {
	type TestCaseExpected struct {
		result bool
	}
	type TestCase struct {
		description string
		given       *Repository
		expected    TestCaseExpected
	}

	var (
		thresholdBefore24 time.Time = time.Now().Add(-(config.IntrospectTimeInterval - 2*time.Hour)) // Subtract 22 hours to the current time
		thresholdAfter24  time.Time = time.Now().Add(-(config.IntrospectTimeInterval + time.Hour))   // Subtract 25 hours to the current time

		testCases []TestCase = []TestCase{
			// BEGIN: Cover all the no valid status
			{
				description: "When Status is not Valid it returns true",
				given: &Repository{
					Status: config.StatusInvalid,
				},
				expected: TestCaseExpected{
					result: true,
				},
			},
			{
				description: "Test pending",
				given: &Repository{
					Status: config.StatusPending,
				},
				expected: TestCaseExpected{
					result: true,
				},
			},
			{
				description: "Test unavail",
				given: &Repository{
					Status: config.StatusUnavailable,
				},
				expected: TestCaseExpected{
					result: true,
				},
			},
			// END: Cover all the no valid status

			{
				description: "When Status is Valid  and LastIntrospectionTime is nil it returns true",
				given: &Repository{
					Status:                config.StatusValid,
					LastIntrospectionTime: nil,
				},
				expected: TestCaseExpected{
					result: true,
				},
			},
			{
				description: "When Status is Valid and LastIntrospectionTime does not reach the threshold interval (24hours) it returns false indicating that no introspection is needed",
				given: &Repository{
					Status:                config.StatusValid,
					LastIntrospectionTime: &thresholdBefore24,
				},
				expected: TestCaseExpected{
					result: false,
				},
			},
			{
				description: "When Status is Valid and LastIntrospectionTime does reach the threshold interval (24hours)  it returns true indicating that an introspection is needed",
				given: &Repository{
					Status:                config.StatusValid,
					LastIntrospectionTime: &thresholdAfter24,
				},
				expected: TestCaseExpected{
					result: true,
				},
			},

			{
				description: "Test around FailedIntrospectionsCount doesn't exceed the count",
				given: &Repository{
					Status:                    config.StatusInvalid,
					FailedIntrospectionsCount: config.FailedIntrospectionsLimit,
					Public:                    false,
				},
				expected: TestCaseExpected{
					result: true,
				},
			},
			{
				description: "Exceeds the count",
				given: &Repository{
					Status:                    config.StatusInvalid,
					FailedIntrospectionsCount: config.FailedIntrospectionsLimit + 1,
					Public:                    false,
				},
				expected: TestCaseExpected{
					result: false,
				},
			},

			{
				description: "Exceeds the count but is public",
				given: &Repository{
					Status:                    config.StatusInvalid,
					FailedIntrospectionsCount: config.FailedIntrospectionsLimit,
					Public:                    true,
				},
				expected: TestCaseExpected{
					result: true,
				},
			},
		}
	)

	for _, tCase := range testCases {
		tCase.given.URL = "https://" + uuid.NewString() + "/"
		tCase.given.UUID = uuid.NewString()
		result := s.tx.Create(&tCase.given)
		assert.NoError(s.T(), result.Error)
	}

	dao := GetRepositoryDao(s.tx)
	repos, err := dao.ListForIntrospection(nil, false)
	assert.NoError(s.T(), err)
	repoIncluded := func(expected *Repository) bool {
		for _, repo := range repos {
			if repo.UUID == expected.UUID {
				return true
			}
		}
		return false
	}
	for _, tCase := range testCases {
		found := repoIncluded(tCase.given)
		assert.Equal(s.T(), tCase.expected.result, found, tCase.description)
	}

	// Force them all
	repos, err = dao.ListForIntrospection(nil, true)
	assert.NoError(s.T(), err)
	for _, tCase := range testCases {
		found := repoIncluded(tCase.given)
		assert.True(s.T(), found, fmt.Sprintf("Forced: %v", tCase.description))
	}

	// Query a single one
	repos, err = dao.ListForIntrospection(&[]string{repos[0].URL}, true)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 1, len(repos))
}
