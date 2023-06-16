package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type RepositorySnapshotSuite struct {
	*ModelsSuite
}

func TestRepositorySnapshotSuite(t *testing.T) {
	m := ModelsSuite{}
	r := RepositorySnapshotSuite{&m}
	suite.Run(t, &r)
}

func (s *RepositorySnapshotSuite) TestSnapshot() {
	t := s.T()
	tx := s.tx

	testRepository := Repository{
		URL:                    "https://example.com",
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
	}
	err := tx.Create(&testRepository).Error
	assert.NoError(t, err)

	snap := Snapshot{
		Base:             Base{},
		VersionHref:      "/pulp/version",
		PublicationHref:  "/pulp/publication",
		DistributionPath: "/path/to/distr",
		OrgId:            "someOrg",
		RepositoryUUID:   testRepository.UUID,
		ContentCounts:    ContentCounts{"packages": int64(3)},
	}
	insert := tx.Create(&snap)
	assert.NoError(t, insert.Error)

	readSnap := Snapshot{}
	result := tx.Where("uuid = ?", snap.UUID).First(&readSnap)
	assert.NoError(t, result.Error)
	assert.Equal(t, "someOrg", readSnap.OrgId)
	assert.Equal(t, int64(3), readSnap.ContentCounts["packages"])
}
