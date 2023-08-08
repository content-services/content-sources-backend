package models

import (
	"testing"

	uuid2 "github.com/google/uuid"
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

	testRepoConfig := RepositoryConfiguration{
		RepositoryUUID: testRepository.UUID,
		OrgID:          "someOrg",
		Name:           uuid2.NewString(),
	}
	err = tx.Create(&testRepoConfig).Error
	assert.NoError(t, err)

	snap := Snapshot{
		Base:                        Base{},
		VersionHref:                 "/pulp/version",
		PublicationHref:             "/pulp/publication",
		DistributionPath:            "/path/to/distr",
		RepositoryConfigurationUUID: testRepoConfig.UUID,
		ContentCounts:               ContentCounts{"packages": int64(3)},
	}
	insert := tx.Create(&snap)
	assert.NoError(t, insert.Error)

	readSnap := Snapshot{}
	result := tx.Where("uuid = ?", snap.UUID).First(&readSnap)
	assert.NoError(t, result.Error)
	assert.Equal(t, testRepoConfig.UUID, readSnap.RepositoryConfigurationUUID)
	assert.Equal(t, int64(3), readSnap.ContentCounts["packages"])
}
