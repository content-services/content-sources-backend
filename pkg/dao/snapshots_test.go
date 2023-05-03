package dao

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/models"
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

func (s *RepositorySnapshotSuite) TestCreateAndList() {
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

	snap := Snapshot{
		Base:             models.Base{},
		VersionHref:      "/pulp/version",
		PublicationHref:  "/pulp/publication",
		DistributionPath: "/path/to/distr",
		OrgId:            "someOrg",
		RepositoryUUID:   testRepository.UUID,
		ContentCounts:    ContentCounts{"packages": int64(3)},
	}

	sDao := snapshotDaoImpl{db: tx}
	err = sDao.Create(&snap)
	assert.NoError(t, err)

	list, err := sDao.List(rConfig.UUID)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(list))
	assert.Equal(t, snap.ContentCounts, list[0].ContentCounts)
}
