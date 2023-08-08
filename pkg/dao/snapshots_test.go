package dao

import (
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
	uuid2 "github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type SnapshotsSuite struct {
	*DaoSuite
}

func TestSnapshotsSuite(t *testing.T) {
	m := DaoSuite{}
	r := SnapshotsSuite{&m}
	suite.Run(t, &r)
}

func (s *SnapshotsSuite) TestCreateAndList() {
	t := s.T()
	tx := s.tx
	sDao := snapshotDaoImpl{db: tx}
	rConfig := s.createRepository()
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: "",
	}

	snap := s.createSnapshot(rConfig)

	collection, total, err := sDao.List(rConfig.UUID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 1, len(collection.Data))
	if len(collection.Data) > 0 {
		assert.Equal(t, snap.RepositoryPath, collection.Data[0].RepositoryPath)
		assert.Equal(t, snap.ContentCounts, models.ContentCounts(collection.Data[0].ContentCounts))
		assert.False(t, collection.Data[0].CreatedAt.IsZero())
	}
}

func (s *SnapshotsSuite) TestListNoSnapshots() {
	t := s.T()
	tx := s.tx
	sDao := snapshotDaoImpl{db: tx}
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: "",
	}

	testRepository := models.Repository{
		URL:                    "https://example.com",
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
	}
	err := tx.Create(&testRepository).Error
	assert.NoError(t, err)

	rConfig := models.RepositoryConfiguration{
		Name:           "toSnapshot",
		OrgID:          "someOrg",
		RepositoryUUID: testRepository.UUID,
	}

	err = tx.Create(&rConfig).Error
	assert.NoError(t, err)

	collection, total, err := sDao.List(rConfig.UUID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, 0, len(collection.Data))
}

func (s *SnapshotsSuite) TestListPageLimit() {
	t := s.T()
	tx := s.tx
	sDao := snapshotDaoImpl{db: tx}
	rConfig := s.createRepository()
	pageData := api.PaginationData{
		Limit:  10,
		Offset: 0,
	}
	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: "",
	}

	for i := 0; i < 11; i++ {
		s.createSnapshot(rConfig)
	}

	collection, total, err := sDao.List(rConfig.UUID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(11), total)
	assert.Equal(t, 10, len(collection.Data))
}

func (s *SnapshotsSuite) createRepository() models.RepositoryConfiguration {
	t := s.T()
	tx := s.tx

	testRepository := models.Repository{
		URL:                    "https://example.com",
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
	}
	err := tx.Create(&testRepository).Error
	assert.NoError(t, err)

	rConfig := models.RepositoryConfiguration{
		Name:           "toSnapshot",
		OrgID:          "someOrg",
		RepositoryUUID: testRepository.UUID,
	}

	err = tx.Create(&rConfig).Error
	assert.NoError(t, err)
	return rConfig
}

func (s *SnapshotsSuite) createSnapshot(rConfig models.RepositoryConfiguration) models.Snapshot {
	t := s.T()
	tx := s.tx

	snap := models.Snapshot{
		Base:                        models.Base{},
		VersionHref:                 "/pulp/version",
		PublicationHref:             "/pulp/publication",
		DistributionPath:            fmt.Sprintf("/path/to/%v", uuid2.NewString()),
		RepositoryConfigurationUUID: rConfig.UUID,
		ContentCounts:               models.ContentCounts{"rpm.package": int64(3), "rpm.advisory": int64(1)},
	}

	sDao := snapshotDaoImpl{db: tx}
	err := sDao.Create(&snap)
	assert.NoError(t, err)
	return snap
}

func (s *SnapshotsSuite) TestFetchForRepoUUID() {
	t := s.T()
	tx := s.tx

	repoConfig := s.createRepository()
	s.createSnapshot(repoConfig)

	sDao := snapshotDaoImpl{db: tx}
	snaps, err := sDao.FetchForRepoConfigUUID(repoConfig.UUID)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(snaps))
	assert.Equal(t, snaps[0].RepositoryConfigurationUUID, repoConfig.UUID)
}
