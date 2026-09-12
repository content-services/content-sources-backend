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

func createTestTask(t *testing.T, tx *gorm.DB, status string) models.TaskInfo {
	t.Helper()
	task := models.TaskInfo{
		Id:     uuid.New(),
		Status: status,
		OrgId:  seeds.RandomOrgId(),
	}
	require.NoError(t, tx.Create(&task).Error)
	return task
}

func createTestSnapshotWithPublishTask(t *testing.T, tx *gorm.DB, repoConfigUUID string, published bool, task *models.TaskInfo) models.Snapshot {
	t.Helper()
	snap := models.Snapshot{
		Base:                        models.Base{UUID: uuid.NewString()},
		VersionHref:                 "/pulp/version/" + uuid.NewString(),
		PublicationHref:             "/pulp/publication/" + uuid.NewString(),
		DistributionPath:            "/content/" + uuid.NewString(),
		RepositoryPath:              "/content/" + uuid.NewString(),
		DistributionHref:            "/pulp/distribution/" + uuid.NewString(),
		RepositoryConfigurationUUID: repoConfigUUID,
		ContentCounts:               models.ContentCountsType{},
		AddedCounts:                 models.ContentCountsType{},
		RemovedCounts:               models.ContentCountsType{},
		DetectedOSVersion:           "9",
		Published:                   published,
	}
	if task != nil {
		snap.PublishTaskUUID = task.Id.String()
	}
	require.NoError(t, tx.Create(&snap).Error)
	return snap
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_Empty() {
	t := s.T()
	ctx := context.Background()

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_NoSnapshots() {
	t := s.T()
	ctx := context.Background()

	repo := createTestUploadRepository(t, s.tx)
	repoConfig := createTestPartnerRepoConfig(t, s.tx, repo, seeds.RandomOrgId(), "no-snapshots", true)

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{repoConfig.UUID})
	require.NoError(t, err)
	require.Contains(t, result, repoConfig.UUID)
	assert.False(t, result[repoConfig.UUID].Publishing)
	assert.False(t, result[repoConfig.UUID].Unpublishing)
	assert.False(t, result[repoConfig.UUID].Published)
	assert.False(t, result[repoConfig.UUID].Stopped)
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_Publishing() {
	t := s.T()
	ctx := context.Background()

	repo := createTestUploadRepository(t, s.tx)
	repoConfig := createTestPartnerRepoConfig(t, s.tx, repo, seeds.RandomOrgId(), "publishing", true)

	// Publishing: snapshot.published=true with pending/running task (handler sets published=true before enqueueing)
	for _, status := range []string{"pending", "running"} {
		task := createTestTask(t, s.tx, status)
		createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, true, &task)
	}

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{repoConfig.UUID})
	require.NoError(t, err)
	assert.True(t, result[repoConfig.UUID].Publishing)
	assert.False(t, result[repoConfig.UUID].Unpublishing)
	assert.False(t, result[repoConfig.UUID].Published)
	assert.False(t, result[repoConfig.UUID].Stopped)
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_Unpublishing() {
	t := s.T()
	ctx := context.Background()

	repo := createTestUploadRepository(t, s.tx)
	repoConfig := createTestPartnerRepoConfig(t, s.tx, repo, seeds.RandomOrgId(), "unpublishing", true)

	// Unpublishing: snapshot.published=false with pending/running task (handler sets published=false before enqueueing)
	for _, status := range []string{"pending", "running"} {
		task := createTestTask(t, s.tx, status)
		createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, false, &task)
	}

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{repoConfig.UUID})
	require.NoError(t, err)
	assert.False(t, result[repoConfig.UUID].Publishing)
	assert.True(t, result[repoConfig.UUID].Unpublishing)
	assert.False(t, result[repoConfig.UUID].Published)
	assert.False(t, result[repoConfig.UUID].Stopped)
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_Published() {
	t := s.T()
	ctx := context.Background()

	repo := createTestUploadRepository(t, s.tx)
	repoConfig := createTestPartnerRepoConfig(t, s.tx, repo, seeds.RandomOrgId(), "published", true)

	task := createTestTask(t, s.tx, "completed")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, true, &task)

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{repoConfig.UUID})
	require.NoError(t, err)
	assert.False(t, result[repoConfig.UUID].Publishing)
	assert.False(t, result[repoConfig.UUID].Unpublishing)
	assert.True(t, result[repoConfig.UUID].Published)
	assert.False(t, result[repoConfig.UUID].Stopped)
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_Stopped() {
	t := s.T()
	ctx := context.Background()

	repo := createTestUploadRepository(t, s.tx)
	repoConfig := createTestPartnerRepoConfig(t, s.tx, repo, seeds.RandomOrgId(), "stopped", true)

	for _, status := range []string{"failed", "canceled"} {
		task := createTestTask(t, s.tx, status)
		createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, true, &task)
	}

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{repoConfig.UUID})
	require.NoError(t, err)
	assert.False(t, result[repoConfig.UUID].Publishing)
	assert.False(t, result[repoConfig.UUID].Unpublishing)
	assert.False(t, result[repoConfig.UUID].Published)
	assert.True(t, result[repoConfig.UUID].Stopped)
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_StoppedUnpublish() {
	t := s.T()
	ctx := context.Background()

	repo := createTestUploadRepository(t, s.tx)
	repoConfig := createTestPartnerRepoConfig(t, s.tx, repo, seeds.RandomOrgId(), "stopped-unpublish", true)

	// Unpublish that failed: published=false with a failed task
	failedTask := createTestTask(t, s.tx, "failed")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, false, &failedTask)

	// Unpublish that was canceled: published=false with a canceled task
	canceledTask := createTestTask(t, s.tx, "canceled")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, false, &canceledTask)

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{repoConfig.UUID})
	require.NoError(t, err)
	assert.False(t, result[repoConfig.UUID].Publishing)
	assert.False(t, result[repoConfig.UUID].Unpublishing)
	assert.False(t, result[repoConfig.UUID].Published)
	assert.True(t, result[repoConfig.UUID].Stopped)
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_PublishedAndPublishing() {
	t := s.T()
	ctx := context.Background()

	repo := createTestUploadRepository(t, s.tx)
	repoConfig := createTestPartnerRepoConfig(t, s.tx, repo, seeds.RandomOrgId(), "pub-and-publishing", true)

	// One snapshot already published (completed task)
	completedTask := createTestTask(t, s.tx, "completed")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, true, &completedTask)

	// Another snapshot being published (published=true, pending task)
	pendingTask := createTestTask(t, s.tx, "pending")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, true, &pendingTask)

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{repoConfig.UUID})
	require.NoError(t, err)
	assert.True(t, result[repoConfig.UUID].Publishing)
	assert.False(t, result[repoConfig.UUID].Unpublishing)
	assert.True(t, result[repoConfig.UUID].Published)
	assert.False(t, result[repoConfig.UUID].Stopped)
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_PublishedAndUnpublishing() {
	t := s.T()
	ctx := context.Background()

	repo := createTestUploadRepository(t, s.tx)
	repoConfig := createTestPartnerRepoConfig(t, s.tx, repo, seeds.RandomOrgId(), "pub-and-unpublishing", true)

	// One snapshot already published (completed task)
	completedTask := createTestTask(t, s.tx, "completed")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, true, &completedTask)

	// Another snapshot being unpublished (published=false, pending task)
	pendingTask := createTestTask(t, s.tx, "pending")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, false, &pendingTask)

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{repoConfig.UUID})
	require.NoError(t, err)
	assert.False(t, result[repoConfig.UUID].Publishing)
	assert.True(t, result[repoConfig.UUID].Unpublishing)
	assert.True(t, result[repoConfig.UUID].Published)
	assert.False(t, result[repoConfig.UUID].Stopped)
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_PublishingAndUnpublishing() {
	t := s.T()
	ctx := context.Background()

	repo := createTestUploadRepository(t, s.tx)
	repoConfig := createTestPartnerRepoConfig(t, s.tx, repo, seeds.RandomOrgId(), "publishing-and-unpublishing", true)

	// One snapshot being published (published=true, running task)
	publishTask := createTestTask(t, s.tx, "running")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, true, &publishTask)

	// Another snapshot being unpublished (published=false, running task)
	unpublishTask := createTestTask(t, s.tx, "running")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, false, &unpublishTask)

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{repoConfig.UUID})
	require.NoError(t, err)
	assert.True(t, result[repoConfig.UUID].Publishing)
	assert.True(t, result[repoConfig.UUID].Unpublishing)
	assert.False(t, result[repoConfig.UUID].Published)
	assert.False(t, result[repoConfig.UUID].Stopped)
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_AllFourStates() {
	t := s.T()
	ctx := context.Background()

	repo := createTestUploadRepository(t, s.tx)
	repoConfig := createTestPartnerRepoConfig(t, s.tx, repo, seeds.RandomOrgId(), "all-states", true)

	// Published: completed task
	completedTask := createTestTask(t, s.tx, "completed")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, true, &completedTask)

	// Publishing: published=true, running task
	publishingTask := createTestTask(t, s.tx, "running")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, true, &publishingTask)

	// Unpublishing: published=false, running task
	unpublishingTask := createTestTask(t, s.tx, "running")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, false, &unpublishingTask)

	// Stopped: failed task
	failedTask := createTestTask(t, s.tx, "failed")
	createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, true, &failedTask)

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{repoConfig.UUID})
	require.NoError(t, err)
	assert.True(t, result[repoConfig.UUID].Publishing)
	assert.True(t, result[repoConfig.UUID].Unpublishing)
	assert.True(t, result[repoConfig.UUID].Published)
	assert.True(t, result[repoConfig.UUID].Stopped)
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_MultipleBatch() {
	t := s.T()
	ctx := context.Background()

	repo1 := createTestUploadRepository(t, s.tx)
	rc1 := createTestPartnerRepoConfig(t, s.tx, repo1, seeds.RandomOrgId(), "batch-1", true)
	completedTask := createTestTask(t, s.tx, "completed")
	createTestSnapshotWithPublishTask(t, s.tx, rc1.UUID, true, &completedTask)

	repo2 := createTestUploadRepository(t, s.tx)
	rc2 := createTestPartnerRepoConfig(t, s.tx, repo2, seeds.RandomOrgId(), "batch-2", true)
	pendingTask := createTestTask(t, s.tx, "pending")
	createTestSnapshotWithPublishTask(t, s.tx, rc2.UUID, false, &pendingTask)

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{rc1.UUID, rc2.UUID})
	require.NoError(t, err)

	assert.False(t, result[rc1.UUID].Publishing)
	assert.False(t, result[rc1.UUID].Unpublishing)
	assert.True(t, result[rc1.UUID].Published)
	assert.False(t, result[rc1.UUID].Stopped)

	assert.False(t, result[rc2.UUID].Publishing)
	assert.True(t, result[rc2.UUID].Unpublishing)
	assert.False(t, result[rc2.UUID].Published)
	assert.False(t, result[rc2.UUID].Stopped)
}

func (s *PartnerVisibilitySuite) TestComputeSnapshotPublishStates_DeletedSnapshotsIgnored() {
	t := s.T()
	ctx := context.Background()

	repo := createTestUploadRepository(t, s.tx)
	repoConfig := createTestPartnerRepoConfig(t, s.tx, repo, seeds.RandomOrgId(), "deleted-snap", true)

	completedTask := createTestTask(t, s.tx, "completed")
	snap := createTestSnapshotWithPublishTask(t, s.tx, repoConfig.UUID, true, &completedTask)

	// Soft-delete the snapshot
	deletedAt := gorm.DeletedAt{Time: time.Now(), Valid: true}
	require.NoError(t, s.tx.Model(&snap).Update("deleted_at", deletedAt).Error)

	result, err := computeSnapshotPublishStates(ctx, s.tx, []string{repoConfig.UUID})
	require.NoError(t, err)
	assert.False(t, result[repoConfig.UUID].Publishing)
	assert.False(t, result[repoConfig.UUID].Unpublishing)
	assert.False(t, result[repoConfig.UUID].Published)
	assert.False(t, result[repoConfig.UUID].Stopped)
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
