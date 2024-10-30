package dao

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/content-services/yummy/pkg/yum"
	zest "github.com/content-services/zest/release/v2024"
	uuid2 "github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type SnapshotsSuite struct {
	*DaoSuite
}

func TestSnapshotsSuite(t *testing.T) {
	m := DaoSuite{}
	r := SnapshotsSuite{DaoSuite: &m}
	suite.Run(t, &r)
}

var testPulpStatusResponse = zest.StatusResponse{
	ContentSettings: zest.ContentSettingsResponse{
		ContentOrigin:     "http://pulp-content",
		ContentPathPrefix: "/pulp/content",
	},
}

var testContentPath = testPulpStatusResponse.ContentSettings.ContentOrigin + testPulpStatusResponse.ContentSettings.ContentPathPrefix

func (s *SnapshotsSuite) createRepository() models.RepositoryConfiguration {
	t := s.T()
	tx := s.tx
	const lookup string = "0123456789abcdefghijklmnopqrstuvwxyz"
	randomName := seeds.RandStringWithChars(10, lookup)
	testRepository := models.Repository{
		URL:                    "https://example.com/" + randomName,
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
		Origin:                 config.OriginExternal,
	}
	err := tx.Create(&testRepository).Error
	assert.NoError(t, err)

	rConfig := models.RepositoryConfiguration{
		Name:           "toSnapshot" + randomName,
		OrgID:          "someOrg",
		RepositoryUUID: testRepository.UUID,
	}

	err = tx.Create(&rConfig).Error
	assert.NoError(t, err)
	return rConfig
}

func (s *SnapshotsSuite) createRepositoryWithPrefix(prefix string) models.RepositoryConfiguration {
	t := s.T()
	tx := s.tx
	const lookup string = "0123456789abcdefghijklmnopqrstuvwxyz"
	randomName := seeds.RandStringWithChars(10, lookup)
	testRepository := models.Repository{
		URL:                    "https://example.com/" + randomName,
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
		Origin:                 config.OriginExternal,
	}
	err := tx.Create(&testRepository).Error
	assert.NoError(t, err)

	rConfig := models.RepositoryConfiguration{
		Name:           "toSnapshot" + prefix + randomName,
		OrgID:          "someOrg",
		RepositoryUUID: testRepository.UUID,
	}

	err = tx.Create(&rConfig).Error
	assert.NoError(t, err)
	return rConfig
}

func (s *SnapshotsSuite) createRedhatRepository() models.RepositoryConfiguration {
	t := s.T()
	tx := s.tx

	testRepository := models.Repository{
		URL:                    "https://example.redhat.com",
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
	}
	err := tx.Create(&testRepository).Error
	assert.NoError(t, err)

	rConfig := models.RepositoryConfiguration{
		Name:           "redhatSnapshot",
		OrgID:          config.RedHatOrg,
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
		ContentCounts:               models.ContentCountsType{"rpm.package": int64(3), "rpm.advisory": int64(1)},
		AddedCounts:                 models.ContentCountsType{"rpm.package": int64(1), "rpm.advisory": int64(3)},
		RemovedCounts:               models.ContentCountsType{"rpm.package": int64(2), "rpm.advisory": int64(2)},
	}

	sDao := snapshotDaoImpl{db: tx}
	err := sDao.Create(context.Background(), &snap)
	assert.NoError(t, err)
	return snap
}

func (s *SnapshotsSuite) createSnapshotAtSpecifiedTime(rConfig models.RepositoryConfiguration, CreatedAt time.Time) models.Snapshot {
	t := s.T()
	tx := s.tx

	snap := models.Snapshot{
		Base:                        models.Base{CreatedAt: CreatedAt},
		VersionHref:                 "/pulp/version",
		PublicationHref:             "/pulp/publication",
		DistributionPath:            fmt.Sprintf("/path/to/%v", uuid2.NewString()),
		RepositoryConfigurationUUID: rConfig.UUID,
		ContentCounts:               models.ContentCountsType{"rpm.package": int64(3), "rpm.advisory": int64(1)},
		AddedCounts:                 models.ContentCountsType{"rpm.package": int64(1), "rpm.advisory": int64(3)},
		RemovedCounts:               models.ContentCountsType{"rpm.package": int64(2), "rpm.advisory": int64(2)},
	}

	sDao := snapshotDaoImpl{db: tx}
	err := sDao.Create(context.Background(), &snap)
	assert.NoError(t, err)
	return snap
}

func (s *SnapshotsSuite) createTemplate(orgID string, rConfigs ...models.RepositoryConfiguration) api.TemplateResponse {
	t := s.T()
	tx := s.tx

	var repoUUIDs []string
	for _, repo := range rConfigs {
		repoUUIDs = append(repoUUIDs, repo.UUID)
		s.createSnapshot(repo)
	}

	timeNow := time.Now()
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr("template test"),
		Description:     utils.Ptr("template test description"),
		RepositoryUUIDS: repoUUIDs,
		Arch:            utils.Ptr(config.AARCH64),
		Version:         utils.Ptr(config.El8),
		Date:            (*api.EmptiableDate)(&timeNow),
		OrgID:           &orgID,
		UseLatest:       utils.Ptr(false),
	}

	tDao := templateDaoImpl{db: tx}
	template, err := tDao.Create(context.Background(), reqTemplate)
	assert.NoError(t, err)
	return template
}

func (s *SnapshotsSuite) TestCreateAndList() {
	t := s.T()
	tx := s.tx
	ctx := context.Background()

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	sDao := snapshotDaoImpl{db: tx, pulpClient: mockPulpClient}

	if config.Get().Features.Snapshots.Enabled {
		mockPulpClient.WithDomainMock().On("GetContentPath", ctx).Return(testContentPath, nil)
	} else {
		mockPulpClient.On("GetContentPath", ctx).Return(testContentPath, nil)
	}

	repoDaoImpl := repositoryConfigDaoImpl{db: tx, yumRepo: &yum.MockYumRepository{}, pulpClient: mockPulpClient}
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

	collection, total, err := sDao.List(ctx, rConfig.OrgID, rConfig.UUID, pageData, filterData)

	repository, _ := repoDaoImpl.fetchRepoConfig(ctx, rConfig.OrgID, rConfig.UUID, false)
	repositoryList, repoCount, _ := repoDaoImpl.List(ctx, rConfig.OrgID, api.PaginationData{Limit: -1}, api.FilterData{Origin: "external"})

	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 1, len(collection.Data))
	if len(collection.Data) > 0 {
		assert.Equal(t, snap.RepositoryPath, collection.Data[0].RepositoryPath)
		assert.Equal(t, snap.ContentCounts, models.ContentCountsType(collection.Data[0].ContentCounts))
		assert.Equal(t, snap.AddedCounts, models.ContentCountsType(collection.Data[0].AddedCounts))
		assert.Equal(t, snap.RemovedCounts, models.ContentCountsType(collection.Data[0].RemovedCounts))
		assert.False(t, collection.Data[0].CreatedAt.IsZero())
		// Check that the repositoryConfig has the appropriate values
		assert.Equal(t, snap.UUID, repository.LastSnapshotUUID)
		assert.EqualValues(t, snap.AddedCounts, repository.LastSnapshot.AddedCounts)
		assert.EqualValues(t, snap.RemovedCounts, repository.LastSnapshot.RemovedCounts)
		// Check that the list repositoryConfig has the appropriate values
		assert.Equal(t, int64(1), repoCount)
		assert.Equal(t, snap.UUID, repositoryList.Data[0].LastSnapshotUUID)
		assert.EqualValues(t, snap.AddedCounts, repositoryList.Data[0].LastSnapshot.AddedCounts)
		assert.EqualValues(t, snap.RemovedCounts, repositoryList.Data[0].LastSnapshot.RemovedCounts)
	}
}

func (s *SnapshotsSuite) TestCreateAndListRedHatRepo() {
	t := s.T()
	tx := s.tx
	ctx := context.Background()

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	sDao := snapshotDaoImpl{db: tx, pulpClient: mockPulpClient}

	if config.Get().Features.Snapshots.Enabled {
		mockPulpClient.WithDomainMock().On("GetContentPath", ctx).Return(testContentPath, nil)
	} else {
		mockPulpClient.On("GetContentPath", ctx).Return(testContentPath, nil)
	}

	repoDao := repositoryConfigDaoImpl{db: tx, yumRepo: &yum.MockYumRepository{}, pulpClient: mockPulpClient}

	redhatRepositoryConfig := s.createRedhatRepository()
	redhatSnap := s.createSnapshot(redhatRepositoryConfig)

	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: "",
	}

	collection, total, err := sDao.List(context.Background(), "ShouldNotMatter", redhatRepositoryConfig.UUID, pageData, filterData)

	repository, _ := repoDao.fetchRepoConfig(context.Background(), "ShouldNotMatter", redhatRepositoryConfig.UUID, true)
	repositoryList, repoCount, _ := repoDao.List(context.Background(), "ShouldNotMatter", api.PaginationData{Limit: -1}, api.FilterData{Origin: "external"})

	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 1, len(collection.Data))
	if len(collection.Data) > 0 {
		assert.Equal(t, redhatSnap.RepositoryPath, collection.Data[0].RepositoryPath)
		assert.Equal(t, redhatSnap.ContentCounts, models.ContentCountsType(collection.Data[0].ContentCounts))
		assert.Equal(t, redhatSnap.AddedCounts, models.ContentCountsType(collection.Data[0].AddedCounts))
		assert.Equal(t, redhatSnap.RemovedCounts, models.ContentCountsType(collection.Data[0].RemovedCounts))
		assert.False(t, collection.Data[0].CreatedAt.IsZero())
		// Check that the repositoryConfig has the appropriate values
		assert.Equal(t, redhatSnap.UUID, repository.LastSnapshotUUID)
		assert.EqualValues(t, redhatSnap.AddedCounts, repository.LastSnapshot.AddedCounts)
		assert.EqualValues(t, redhatSnap.RemovedCounts, repository.LastSnapshot.RemovedCounts)
		// Check that the list repositoryConfig has the appropriate values
		assert.Equal(t, int64(1), repoCount)
		assert.Equal(t, redhatSnap.UUID, repositoryList.Data[0].LastSnapshotUUID)
		assert.EqualValues(t, redhatSnap.AddedCounts, repositoryList.Data[0].LastSnapshot.AddedCounts)
		assert.EqualValues(t, redhatSnap.RemovedCounts, repositoryList.Data[0].LastSnapshot.RemovedCounts)
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

	collection, total, err := sDao.List(context.Background(), rConfig.OrgID, rConfig.UUID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, 0, len(collection.Data))
}

func (s *SnapshotsSuite) TestListPageLimit() {
	t := s.T()
	tx := s.tx

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	sDaoImpl := snapshotDaoImpl{db: tx, pulpClient: mockPulpClient}

	mockPulpClient.On("GetContentPath", context.Background()).Return(testContentPath, nil)

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

	collection, total, err := sDaoImpl.List(context.Background(), rConfig.OrgID, rConfig.UUID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(11), total)
	assert.Equal(t, 10, len(collection.Data))
}

func (s *SnapshotsSuite) TestListNotFound() {
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

	s.createSnapshot(rConfig)

	collection, total, err := sDao.List(context.Background(), rConfig.OrgID, "bad-uuid", pageData, filterData)
	assert.Error(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, 0, len(collection.Data))
}

func (s *SnapshotsSuite) TestListNotFoundBadOrgId() {
	t := s.T()
	tx := s.tx

	sDao := snapshotDaoImpl{db: tx}

	testRepository := models.Repository{
		URL:                    "https://example.com",
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
	}
	err := tx.Create(&testRepository).Error
	assert.NoError(t, err)

	rConfig := models.RepositoryConfiguration{
		Name:           "toSnapshot",
		OrgID:          "not-banana-id",
		RepositoryUUID: testRepository.UUID,
	}

	err = tx.Create(&rConfig).Error
	assert.NoError(t, err)

	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
	}
	filterData := api.FilterData{
		Search:  "",
		Arch:    "",
		Version: "",
	}

	s.createSnapshot(rConfig)

	collection, total, err := sDao.List(context.Background(), "bad-banana-id", rConfig.UUID, pageData, filterData)
	assert.Error(t, err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(t, ok)
	assert.True(t, daoError.NotFound)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, 0, len(collection.Data))
	assert.ErrorContains(t, err, "Could not find repository with UUID "+rConfig.UUID)
}

func (s *SnapshotsSuite) TestFetchForRepoUUID() {
	t := s.T()
	tx := s.tx

	repoConfig := s.createRepository()
	s.createSnapshot(repoConfig)

	sDao := snapshotDaoImpl{db: tx}
	snaps, err := sDao.FetchForRepoConfigUUID(context.Background(), repoConfig.UUID)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(snaps))
	assert.Equal(t, snaps[0].RepositoryConfigurationUUID, repoConfig.UUID)
}

func (s *SnapshotsSuite) TestFetchLatestSnapshot() {
	t := s.T()
	tx := s.tx

	repoConfig := s.createRepository()
	s.createSnapshot(repoConfig)
	latestSnapshot := s.createSnapshot(repoConfig)

	sDao := GetSnapshotDao(tx)
	response, err := sDao.FetchLatestSnapshot(context.Background(), repoConfig.UUID)
	assert.NoError(t, err)
	// Need to truncate because PostgreSQL has microsecond precision
	assert.True(t, latestSnapshot.Base.CreatedAt.Truncate(time.Microsecond).Equal(response.CreatedAt))
	assert.Equal(t, latestSnapshot.RepositoryPath, response.RepositoryPath)
}

func (s *SnapshotsSuite) TestFetchSnapshotsByDateAndRepository() {
	t := s.T()
	tx := s.tx

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	sDao := snapshotDaoImpl{db: tx, pulpClient: mockPulpClient}
	mockPulpClient.On("GetContentPath", context.Background()).Return(testContentPath, nil)

	repoConfig := s.createRepository()
	baseTime := time.Now()
	s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(-time.Hour*30)) // Before Date
	second := s.createSnapshotAtSpecifiedTime(repoConfig, baseTime)          // Target Date
	s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(time.Hour*1))   // After Date

	request := api.ListSnapshotByDateRequest{}

	request.Date = second.Base.CreatedAt.Add(time.Minute * 31)

	request.RepositoryUUIDS = []string{repoConfig.UUID}

	response, err := sDao.FetchSnapshotsByDateAndRepository(context.Background(), repoConfig.OrgID, request)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, false, response.Data[0].IsAfter)
	assert.Equal(t, second.Base.UUID, response.Data[0].Match.UUID)
	assert.Equal(t, second.Base.CreatedAt.Day(), response.Data[0].Match.CreatedAt.Day())
	assert.NotEmpty(t, response.Data[0].Match.URL)
}

func (s *SnapshotsSuite) TestFetchSnapshotsModelByDateAndRepositoryNew() {
	t := s.T()
	tx := s.tx

	repoConfig := s.createRepository()
	baseTime := time.Now()
	first := s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(-time.Hour*30)) // Before Date
	second := s.createSnapshotAtSpecifiedTime(repoConfig, baseTime)                   // Target Date
	third := s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(time.Hour*1))   // After Date

	sDao := GetSnapshotDao(tx)
	// Exact match to second
	response, err := sDao.FetchSnapshotsModelByDateAndRepository(context.Background(), repoConfig.OrgID, api.ListSnapshotByDateRequest{
		RepositoryUUIDS: []string{repoConfig.UUID},
		Date:            second.Base.CreatedAt.Add(time.Second * 1),
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(response))
	assert.Equal(t, second.Base.UUID, response[0].UUID)

	// 31 minutes after should still use second
	response, err = sDao.FetchSnapshotsModelByDateAndRepository(context.Background(), repoConfig.OrgID, api.ListSnapshotByDateRequest{
		RepositoryUUIDS: []string{repoConfig.UUID},
		Date:            second.Base.CreatedAt.Add(time.Minute * 31),
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(response))
	assert.Equal(t, second.Base.UUID, response[0].UUID)

	// 31 minutes after should still use second, but specify EDT time
	tz, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	response, err = sDao.FetchSnapshotsModelByDateAndRepository(context.Background(), repoConfig.OrgID, api.ListSnapshotByDateRequest{
		RepositoryUUIDS: []string{repoConfig.UUID},
		Date:            second.Base.CreatedAt.Add(time.Minute * 31).In(tz),
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(response))
	assert.Equal(t, second.Base.UUID, response[0].UUID)

	// 1 minute before should use first
	response, err = sDao.FetchSnapshotsModelByDateAndRepository(context.Background(), repoConfig.OrgID, api.ListSnapshotByDateRequest{
		RepositoryUUIDS: []string{repoConfig.UUID},
		Date:            second.Base.CreatedAt.Add(time.Minute * -1),
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(response))
	assert.Equal(t, first.Base.UUID, response[0].UUID)

	// 2 hours after should use third
	response, err = sDao.FetchSnapshotsModelByDateAndRepository(context.Background(), repoConfig.OrgID, api.ListSnapshotByDateRequest{
		RepositoryUUIDS: []string{repoConfig.UUID},
		Date:            second.Base.CreatedAt.Add(time.Minute * 120),
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(response))
	assert.Equal(t, third.Base.UUID, response[0].UUID)
}

func (s *SnapshotsSuite) TestFetchSnapshotsByDateAndRepositoryMulti() {
	t := s.T()
	tx := s.tx

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	sDao := snapshotDaoImpl{db: tx, pulpClient: mockPulpClient}
	mockPulpClient.On("GetContentPath", context.Background()).Return(testContentPath, nil)

	repoConfig := s.createRepository()
	repoConfig2 := s.createRepository()
	redhatRepo := s.createRedhatRepository()

	baseTime := time.Now()
	s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(-time.Hour*24)) // Before Date
	target1 := s.createSnapshotAtSpecifiedTime(repoConfig, baseTime)         // Closest to Target Date
	s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(time.Hour*24))  // After Date

	target2 := s.createSnapshotAtSpecifiedTime(repoConfig2, baseTime.Add(time.Hour*30)) // Target Date with IsAfter = true
	s.createSnapshotAtSpecifiedTime(repoConfig2, baseTime.Add(time.Hour*70))            // After Date
	s.createSnapshotAtSpecifiedTime(repoConfig2, baseTime.Add(time.Hour*90))            // After Date

	s.createSnapshotAtSpecifiedTime(redhatRepo, baseTime.Add(-time.Hour*600))            // Before Date
	s.createSnapshotAtSpecifiedTime(redhatRepo, baseTime.Add(-time.Hour*200))            // Before Date
	target3 := s.createSnapshotAtSpecifiedTime(redhatRepo, baseTime.Add(-time.Hour*100)) // Closest to Target Date

	request := api.ListSnapshotByDateRequest{}
	request.Date = target1.Base.CreatedAt

	// Intentionally not found ID
	randomUUID, _ := uuid2.NewUUID()

	request.RepositoryUUIDS = []string{
		repoConfig.UUID,
		repoConfig2.UUID,
		redhatRepo.UUID,
		randomUUID.String(),
	}

	fullRepsonse, err := sDao.FetchSnapshotsByDateAndRepository(context.Background(), repoConfig.OrgID, request)
	response := fullRepsonse.Data
	assert.NoError(t, err)
	assert.Equal(t, 4, len(response))
	// target 1
	assert.Equal(t, false, response[0].IsAfter)
	assert.Equal(t, target1.Base.UUID, response[0].Match.UUID)
	// We have to round to the nearest second as go times are at a different precision than postgresql times and won't be exactly equal
	assert.Equal(t, target1.Base.CreatedAt.Round(time.Second), response[0].Match.CreatedAt.Round(time.Second))

	// target 2
	assert.Equal(t, true, response[1].IsAfter)
	assert.Equal(t, target2.Base.UUID, response[1].Match.UUID)
	assert.Equal(t, target2.Base.CreatedAt.Round(time.Second), response[1].Match.CreatedAt.Round(time.Second))

	// target 3 < RedHat repo before the expected date
	assert.Equal(t, false, response[2].IsAfter)
	assert.Equal(t, target3.Base.UUID, response[2].Match.UUID)
	assert.Equal(t, target3.Base.CreatedAt.Round(time.Second), response[2].Match.CreatedAt.Round(time.Second))

	// target 4 < RandomUUID Expect empty state
	assert.Equal(t, randomUUID.String(), response[3].RepositoryUUID)
	assert.Equal(t, false, response[3].IsAfter)
	assert.Empty(t, response[3].Match) // Expect empty struct
}

func (s *SnapshotsSuite) TestListByTemplate() {
	t := s.T()
	tx := s.tx

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	sDao := snapshotDaoImpl{db: tx, pulpClient: mockPulpClient}

	mockPulpClient.On("GetContentPath", context.Background()).Return(testContentPath, nil)

	repoConfig := s.createRepositoryWithPrefix("Last")
	repoConfig2 := s.createRepositoryWithPrefix("First")
	redhatRepo := s.createRedhatRepository()
	template := s.createTemplate(repoConfig.OrgID, repoConfig, repoConfig2, redhatRepo)
	template.RepositoryUUIDS = []string{repoConfig.UUID, repoConfig2.UUID, redhatRepo.UUID}

	baseTime := time.Now()
	t1b := s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(-time.Hour*30)) // Before Date
	t1 := s.createSnapshotAtSpecifiedTime(repoConfig, baseTime)                     // Closest to Target Date
	t1a := s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(time.Hour*30))  // After Date

	t2 := s.createSnapshotAtSpecifiedTime(repoConfig2, baseTime.Add(time.Hour*30))   // Target Date with IsAfter = true
	t2a := s.createSnapshotAtSpecifiedTime(repoConfig2, baseTime.Add(time.Hour*70))  // After Date
	t2aa := s.createSnapshotAtSpecifiedTime(repoConfig2, baseTime.Add(time.Hour*90)) // After Date

	s.createSnapshotAtSpecifiedTime(redhatRepo, baseTime.Add(-time.Hour*600))       // Before Date
	s.createSnapshotAtSpecifiedTime(redhatRepo, baseTime.Add(-time.Hour*200))       // Before Date
	t3 := s.createSnapshotAtSpecifiedTime(redhatRepo, baseTime.Add(-time.Hour*100)) // Closest to Target Date

	const NonRedHatRepoSearch = "to"
	pageData := api.PaginationData{
		Limit:  100,
		Offset: 0,
		SortBy: "repository_name:desc",
	}

	tDao := templateDaoImpl{db: tx}
	err := tDao.UpdateSnapshots(context.Background(), template.UUID, template.RepositoryUUIDS, []models.Snapshot{t1, t2, t3})
	assert.NoError(t, err)

	snapshots, totalSnapshots, err := sDao.ListByTemplate(context.Background(), repoConfig.OrgID, template, NonRedHatRepoSearch, pageData)

	assert.NoError(t, err)
	assert.Equal(t, 2, len(snapshots.Data))
	assert.Equal(t, int64(2), totalSnapshots)

	// target 1
	assert.True(t, snapshots.Data[0].CreatedAt.After(t1b.CreatedAt))
	assert.True(t, snapshots.Data[0].CreatedAt.Before(t1a.CreatedAt))
	assert.True(t, bytes.Contains([]byte(snapshots.Data[0].RepositoryName), []byte("Last")))
	assert.Equal(t, t1.Base.CreatedAt.Day(), snapshots.Data[0].CreatedAt.Day())
	assert.Equal(t, repoConfig.UUID, snapshots.Data[0].RepositoryUUID)

	// target 2
	assert.True(t, snapshots.Data[1].CreatedAt.Before(t2a.CreatedAt))
	assert.True(t, snapshots.Data[1].CreatedAt.Before(t2aa.CreatedAt))
	assert.True(t, bytes.Contains([]byte(snapshots.Data[1].RepositoryName), []byte("First")))
	assert.Equal(t, t2.Base.CreatedAt.Day(), snapshots.Data[1].CreatedAt.Day())
	assert.Equal(t, repoConfig2.UUID, snapshots.Data[1].RepositoryUUID)
}

func (s *SnapshotsSuite) TestListByTemplateWithPagination() {
	t := s.T()
	tx := s.tx

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	sDao := snapshotDaoImpl{db: tx, pulpClient: mockPulpClient}

	mockPulpClient.On("GetContentPath", context.Background()).Return(testContentPath, nil)

	repoConfig := s.createRepositoryWithPrefix("Last")
	repoConfig2 := s.createRepositoryWithPrefix("First")
	redhatRepo := s.createRedhatRepository()
	template := s.createTemplate(repoConfig.OrgID, repoConfig, repoConfig2, redhatRepo)
	template.RepositoryUUIDS = []string{repoConfig.UUID, repoConfig2.UUID, redhatRepo.UUID}

	baseTime := time.Now()
	t1 := s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(-time.Hour*30))
	t2 := s.createSnapshotAtSpecifiedTime(repoConfig2, baseTime)
	t3 := s.createSnapshotAtSpecifiedTime(redhatRepo, baseTime.Add(-time.Hour*100))

	// First call
	pageData := api.PaginationData{
		Limit:  1,
		Offset: 1,
		SortBy: "created_at:desc",
	}

	tDao := templateDaoImpl{db: tx}
	err := tDao.UpdateSnapshots(context.Background(), template.UUID, template.RepositoryUUIDS, []models.Snapshot{t1, t2, t3})
	assert.NoError(t, err)

	snapshots, totalSnapshots, err := sDao.ListByTemplate(context.Background(), repoConfig.OrgID, template, "", pageData)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(snapshots.Data))
	assert.Equal(t, int64(3), totalSnapshots)

	// target
	assert.True(t, bytes.Contains([]byte(snapshots.Data[0].RepositoryName), []byte("Last")))
	assert.Equal(t, t1.Base.CreatedAt.Day(), snapshots.Data[0].CreatedAt.Day())
	assert.Equal(t, repoConfig.UUID, snapshots.Data[0].RepositoryUUID)

	// Second call (test for no nil snapshot overflow)
	pageData = api.PaginationData{
		Limit:  5,
		Offset: 1,
		SortBy: "created_at:desc",
	}

	snapshots, totalSnapshots, err = sDao.ListByTemplate(context.Background(), repoConfig.OrgID, template, "", pageData)

	assert.NoError(t, err)
	assert.Equal(t, 2, len(snapshots.Data))
	assert.Equal(t, int64(3), totalSnapshots)
}

func (s *SnapshotsSuite) TestFetchLatestSnapshotNotFound() {
	t := s.T()
	tx := s.tx

	repoConfig := s.createRepository()

	sDao := GetSnapshotDao(tx)
	_, err := sDao.FetchLatestSnapshot(context.Background(), repoConfig.UUID)
	assert.Equal(t, err, gorm.ErrRecordNotFound)
}

func (s *SnapshotsSuite) TestGetRepositoryConfigurationFile() {
	t := s.T()
	tx := s.tx
	ctx := context.Background()

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	sDao := snapshotDaoImpl{db: tx, pulpClient: mockPulpClient}

	testRepository := models.Repository{
		URL:                    "https://example.com",
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
	}
	err := tx.Create(&testRepository).Error
	assert.NoError(t, err)

	repoConfig := models.RepositoryConfiguration{
		Name:           "!!my repo?test15()",
		OrgID:          "someOrg",
		RepositoryUUID: testRepository.UUID,
	}
	err = tx.Create(&repoConfig).Error
	assert.NoError(t, err)
	expectedRepoID := "[__my_repo_test15__]"

	snapshot := s.createSnapshot(repoConfig)

	// Test happy scenario
	mockPulpClient.On("GetContentPath", ctx).Return(testContentPath, nil).Once()
	repoConfigFile, err := sDao.GetRepositoryConfigurationFile(context.Background(), repoConfig.OrgID, snapshot.UUID, false)
	assert.NoError(t, err)
	assert.Contains(t, repoConfigFile, repoConfig.Name)
	assert.Contains(t, repoConfigFile, expectedRepoID)
	assert.Contains(t, repoConfigFile, testContentPath)
	assert.Contains(t, repoConfigFile, "module_hotfixes=0")

	// Test error from pulp call
	mockPulpClient.On("GetContentPath", ctx).Return("", fmt.Errorf("some error")).Once()
	repoConfigFile, err = sDao.GetRepositoryConfigurationFile(context.Background(), repoConfig.OrgID, snapshot.UUID, false)
	assert.Error(t, err)
	assert.Empty(t, repoConfigFile)

	// Test red hat repo gpg key path is correct
	repoConfigRh := models.RepositoryConfiguration{
		Name:           "rh repo",
		OrgID:          config.RedHatOrg,
		RepositoryUUID: testRepository.UUID,
		GpgKey:         "gpg key",
	}
	err = tx.Create(&repoConfigRh).Error
	assert.NoError(t, err)
	snapshot.RepositoryConfigurationUUID = repoConfigRh.UUID
	err = tx.Updates(&snapshot).Error
	assert.NoError(t, err)

	mockPulpClient.On("GetContentPath", ctx).Return(testContentPath, nil).Once()
	repoConfigFile, err = sDao.GetRepositoryConfigurationFile(context.Background(), repoConfigRh.OrgID, snapshot.UUID, false)
	assert.NoError(t, err)
	assert.Contains(t, repoConfigFile, repoConfigRh.Name)
	assert.Contains(t, repoConfigFile, config.RedHatGpgKeyPath)
}

func (s *SnapshotsSuite) TestGetRepositoryConfigurationFileNotFound() {
	t := s.T()
	tx := s.tx
	ctx := context.Background()

	mockPulpClient := pulp_client.MockPulpClient{}
	sDao := snapshotDaoImpl{db: tx, pulpClient: &mockPulpClient}
	repoConfig := s.createRepository()
	snapshot := s.createSnapshot(repoConfig)

	if config.Get().Features.Snapshots.Enabled {
		mockPulpClient.On("GetContentPath", ctx).Return(testContentPath, nil).Times(3)
	}

	// Test bad snapshot UUID
	mockPulpClient.On("GetContentPath").Return(testContentPath, nil).Once()
	repoConfigFile, err := sDao.GetRepositoryConfigurationFile(context.Background(), repoConfig.OrgID, uuid2.NewString(), false)
	assert.Error(t, err)
	if err != nil {
		daoError, ok := err.(*ce.DaoError)
		assert.True(t, ok)
		assert.True(t, daoError.NotFound)
		assert.Contains(t, daoError.Message, "Could not find snapshot")
	}
	assert.Empty(t, repoConfigFile)

	//  Test bad org ID
	mockPulpClient.On("GetContentPath", ctx).Return(testContentPath, nil).Once()
	repoConfigFile, err = sDao.GetRepositoryConfigurationFile(context.Background(), "bad orgID", snapshot.UUID, false)
	assert.Error(t, err)
	if err != nil {
		daoError, ok := err.(*ce.DaoError)
		assert.True(t, ok)
		assert.True(t, daoError.NotFound)
	}
	assert.Empty(t, repoConfigFile)
}

func (s *SnapshotsSuite) TestFetchSnapshotByVersionHref() {
	t := s.T()
	tx := s.tx

	sDao := snapshotDaoImpl{db: tx}
	repoConfig := s.createRepository()
	snapshot := s.createSnapshot(repoConfig)

	snap, err := sDao.FetchSnapshotByVersionHref(context.Background(), repoConfig.UUID, snapshot.VersionHref)
	require.NoError(t, err)
	assert.NotNil(t, snap)

	snap, err = sDao.FetchSnapshotByVersionHref(context.Background(), repoConfig.UUID, "Not areal href")
	require.NoError(t, err)
	assert.Nil(t, snap)
}
