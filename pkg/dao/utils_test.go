package dao

import (
	"context"
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	uuid2 "github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func createRepository(t *testing.T, tx *gorm.DB, prefix string, redhatRepo bool) models.RepositoryConfiguration {
	const lookup string = "0123456789abcdefghijklmnopqrstuvwxyz"
	randomName := seeds.RandStringWithChars(10, lookup)
	URL := "https://example.com/" + randomName
	origin := config.OriginExternal
	name := "toSnapshot" + prefix + randomName
	orgID := "someOrg"
	if redhatRepo {
		URL = "https://example.redhat.com"
		origin = ""
		name = "redhatSnapshot"
		orgID = config.RedHatOrg
	}

	testRepository := models.Repository{
		URL:                    URL,
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
		Origin:                 origin,
	}
	err := tx.Create(&testRepository).Error
	assert.NoError(t, err)

	rConfig := models.RepositoryConfiguration{
		Name:           name,
		OrgID:          orgID,
		RepositoryUUID: testRepository.UUID,
	}

	err = tx.Create(&rConfig).Error
	assert.NoError(t, err)
	return rConfig
}

func createSnapshot(t *testing.T, tx *gorm.DB, rConfig models.RepositoryConfiguration) models.Snapshot {
	snap := models.Snapshot{
		Base:                        models.Base{},
		VersionHref:                 "/pulp/version",
		PublicationHref:             "/pulp/publication",
		DistributionPath:            fmt.Sprintf("/path/to/%v", uuid2.NewString()),
		RepositoryConfigurationUUID: rConfig.UUID,
		ContentCounts:               models.ContentCountsType{"rpm.package": int64(3), "rpm.advisory": int64(1)},
		AddedCounts:                 models.ContentCountsType{"rpm.package": int64(1), "rpm.advisory": int64(3)},
		RemovedCounts:               models.ContentCountsType{"rpm.package": int64(2), "rpm.advisory": int64(2)},
	}

	sDao := GetSnapshotDao(tx)
	err := sDao.Create(context.Background(), &snap)
	assert.NoError(t, err)
	return snap
}
