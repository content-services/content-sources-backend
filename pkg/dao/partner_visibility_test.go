package dao

import (
	"context"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

func TestIsForeignPartnerView(t *testing.T) {
	ownerOrg := seeds.RandomOrgId()
	viewerOrg := seeds.RandomOrgId()

	tests := []struct {
		name       string
		repoConfig models.RepositoryConfiguration
		viewerOrg  string
		want       bool
	}{
		{
			name: "partner repo viewed by owner",
			repoConfig: models.RepositoryConfiguration{
				Partner: true,
				OrgID:   ownerOrg,
			},
			viewerOrg: ownerOrg,
			want:      false,
		},
		{
			name: "partner repo viewed by foreign org",
			repoConfig: models.RepositoryConfiguration{
				Partner: true,
				OrgID:   ownerOrg,
			},
			viewerOrg: viewerOrg,
			want:      true,
		},
		{
			name: "non-partner repo viewed by foreign org",
			repoConfig: models.RepositoryConfiguration{
				Partner: false,
				OrgID:   ownerOrg,
			},
			viewerOrg: viewerOrg,
			want:      false,
		},
		{
			name: "non-partner repo viewed by owner",
			repoConfig: models.RepositoryConfiguration{
				Partner: false,
				OrgID:   ownerOrg,
			},
			viewerOrg: ownerOrg,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsForeignPartnerView(tt.repoConfig, tt.viewerOrg))
		})
	}
}

type PartnerVisibilitySuite struct {
	*DaoSuite
}

func TestPartnerVisibilitySuite(t *testing.T) {
	suite.Run(t, &PartnerVisibilitySuite{DaoSuite: &DaoSuite{}})
}

func createTestUploadRepository(t *testing.T, tx *gorm.DB) models.Repository {
	t.Helper()
	repo := models.Repository{
		Base:        models.Base{UUID: uuid.NewString()},
		Origin:      config.OriginUpload,
		ContentType: config.ContentTypeRpm,
	}
	require.NoError(t, tx.Create(&repo).Error)
	return repo
}

func createTestPartnerRepoConfig(t *testing.T, tx *gorm.DB, repo models.Repository, orgID, name string, partner bool) models.RepositoryConfiguration {
	t.Helper()
	repoConfig := models.RepositoryConfiguration{
		Base:           models.Base{UUID: uuid.NewString()},
		Name:           name,
		OrgID:          orgID,
		AccountID:      seeds.RandomAccountId(),
		RepositoryUUID: repo.UUID,
		Snapshot:       true,
		Partner:        partner,
	}
	require.NoError(t, tx.Create(&repoConfig).Error)
	return repoConfig
}

func (s *PartnerVisibilitySuite) TestHasPublishedSnapshot() {
	t := s.T()
	ctx := context.Background()

	repo := createTestUploadRepository(t, s.tx)

	repoConfig := createTestPartnerRepoConfig(t, s.tx, repo, seeds.RandomOrgId(), "partner visibility repo", true)

	hasPublished, err := HasPublishedSnapshot(ctx, s.tx, repoConfig.UUID)
	require.NoError(t, err)
	assert.False(t, hasPublished)

	unpublishedSnapshot := models.Snapshot{
		Base:                        models.Base{UUID: uuid.NewString()},
		VersionHref:                 "/pulp/version/unpublished",
		PublicationHref:             "/pulp/publication/unpublished",
		DistributionPath:            "/content/unpublished",
		RepositoryPath:              "/content/unpublished",
		DistributionHref:            "/pulp/distribution/unpublished",
		RepositoryConfigurationUUID: repoConfig.UUID,
		ContentCounts:               models.ContentCountsType{},
		AddedCounts:                 models.ContentCountsType{},
		RemovedCounts:               models.ContentCountsType{},
		DetectedOSVersion:           "9",
		Published:                   false,
	}
	require.NoError(t, s.tx.Create(&unpublishedSnapshot).Error)

	hasPublished, err = HasPublishedSnapshot(ctx, s.tx, repoConfig.UUID)
	require.NoError(t, err)
	assert.False(t, hasPublished)

	publishedSnapshot := models.Snapshot{
		Base:                        models.Base{UUID: uuid.NewString()},
		VersionHref:                 "/pulp/version/published",
		PublicationHref:             "/pulp/publication/published",
		DistributionPath:            "/content/published",
		RepositoryPath:              "/content/published",
		DistributionHref:            "/pulp/distribution/published",
		RepositoryConfigurationUUID: repoConfig.UUID,
		ContentCounts:               models.ContentCountsType{},
		AddedCounts:                 models.ContentCountsType{},
		RemovedCounts:               models.ContentCountsType{},
		DetectedOSVersion:           "9",
		Published:                   true,
	}
	require.NoError(t, s.tx.Create(&publishedSnapshot).Error)

	hasPublished, err = HasPublishedSnapshot(ctx, s.tx, repoConfig.UUID)
	require.NoError(t, err)
	assert.True(t, hasPublished)

	deletedAt := gorm.DeletedAt{Time: time.Now(), Valid: true}
	require.NoError(t, s.tx.Model(&publishedSnapshot).Update("deleted_at", deletedAt).Error)

	hasPublished, err = HasPublishedSnapshot(ctx, s.tx, repoConfig.UUID)
	require.NoError(t, err)
	assert.False(t, hasPublished)
}

func (s *PartnerVisibilitySuite) TestForeignPartnerVisibleExpr() {
	t := s.T()

	ownerOrg := seeds.RandomOrgId()
	viewerOrg := seeds.RandomOrgId()

	visibleRepoConfig := createTestPartnerRepoConfig(t, s.tx, createTestUploadRepository(t, s.tx), ownerOrg, "visible foreign partner repo", true)
	createTestPartnerRepoConfig(t, s.tx, createTestUploadRepository(t, s.tx), ownerOrg, "unpublished foreign partner repo", true)
	createTestPartnerRepoConfig(t, s.tx, createTestUploadRepository(t, s.tx), ownerOrg, "non-partner repo", false)

	publishedSnapshot := models.Snapshot{
		Base:                        models.Base{UUID: uuid.NewString()},
		VersionHref:                 "/pulp/version/visible",
		PublicationHref:             "/pulp/publication/visible",
		DistributionPath:            "/content/visible",
		RepositoryPath:              "/content/visible",
		DistributionHref:            "/pulp/distribution/visible",
		RepositoryConfigurationUUID: visibleRepoConfig.UUID,
		ContentCounts:               models.ContentCountsType{},
		AddedCounts:                 models.ContentCountsType{},
		RemovedCounts:               models.ContentCountsType{},
		DetectedOSVersion:           "9",
		Published:                   true,
	}
	require.NoError(t, s.tx.Create(&publishedSnapshot).Error)

	var visibleUUIDs []string
	err := s.tx.Model(&models.RepositoryConfiguration{}).
		Scopes(ForeignPartnerVisibleExpr(viewerOrg)).
		Pluck("uuid", &visibleUUIDs).Error
	require.NoError(t, err)
	assert.Contains(t, visibleUUIDs, visibleRepoConfig.UUID)
}
