package dao

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	mockExt "github.com/content-services/content-sources-backend/pkg/test/mocks/mock_external"
	zest "github.com/content-services/zest/release/v2023"
	"github.com/google/uuid"
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
	err := sDao.Create(&snap)
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
	err := sDao.Create(&snap)
	assert.NoError(t, err)
	return snap
}

func (s *SnapshotsSuite) TestCreateAndList() {
	t := s.T()
	tx := s.tx

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	sDaoImpl := snapshotDaoImpl{db: tx, pulpClient: mockPulpClient}
	sDao := sDaoImpl.WithContext(context.Background())

	if config.Get().Features.Snapshots.Enabled {
		mockPulpClient.WithContextMock().WithDomainMock().On("GetContentPath").Return(testContentPath, nil)
	} else {
		mockPulpClient.WithContextMock().On("GetContentPath").Return(testContentPath, nil)
	}

	repoDaoImpl := repositoryConfigDaoImpl{db: tx, yumRepo: &mockExt.YumRepositoryMock{}, pulpClient: mockPulpClient}
	repoDao := repoDaoImpl.WithContext(context.Background())
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

	collection, total, err := sDao.List(rConfig.OrgID, rConfig.UUID, pageData, filterData)

	repository, _ := repoDaoImpl.fetchRepoConfig(rConfig.OrgID, rConfig.UUID, false)
	repositoryList, repoCount, _ := repoDao.List(rConfig.OrgID, api.PaginationData{Limit: -1}, api.FilterData{})

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

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	sDaoImpl := snapshotDaoImpl{db: tx, pulpClient: mockPulpClient}
	sDao := sDaoImpl.WithContext(context.Background())

	if config.Get().Features.Snapshots.Enabled {
		mockPulpClient.WithContextMock().WithDomainMock().On("GetContentPath").Return(testContentPath, nil)
	} else {
		mockPulpClient.WithContextMock().On("GetContentPath").Return(testContentPath, nil)
	}

	repoDaoImpl := repositoryConfigDaoImpl{db: tx, yumRepo: &mockExt.YumRepositoryMock{}, pulpClient: mockPulpClient}
	repoDao := repoDaoImpl.WithContext(context.Background())

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

	collection, total, err := sDao.List("ShouldNotMatter", redhatRepositoryConfig.UUID, pageData, filterData)

	repository, _ := repoDaoImpl.fetchRepoConfig("ShouldNotMatter", redhatRepositoryConfig.UUID, true)
	repositoryList, repoCount, _ := repoDao.List("ShouldNotMatter", api.PaginationData{Limit: -1}, api.FilterData{})

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

	collection, total, err := sDao.List(rConfig.OrgID, rConfig.UUID, pageData, filterData)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, 0, len(collection.Data))
}

func (s *SnapshotsSuite) TestListPageLimit() {
	t := s.T()
	tx := s.tx

	mockPulpClient := pulp_client.NewMockPulpClient(t)
	sDaoImpl := snapshotDaoImpl{db: tx, pulpClient: mockPulpClient}
	sDao := sDaoImpl.WithContext(context.Background())

	mockPulpClient.WithContextMock().On("GetContentPath").Return(testContentPath, nil)

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

	collection, total, err := sDao.List(rConfig.OrgID, rConfig.UUID, pageData, filterData)
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

	collection, total, err := sDao.List(rConfig.OrgID, "bad-uuid", pageData, filterData)
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

	collection, total, err := sDao.List("bad-banana-id", rConfig.UUID, pageData, filterData)
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
	snaps, err := sDao.FetchForRepoConfigUUID(repoConfig.UUID)
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
	response, err := sDao.FetchLatestSnapshot(repoConfig.UUID)
	assert.NoError(t, err)
	// Need to truncate because PostgreSQL has microsecond precision
	assert.Equal(t, latestSnapshot.Base.CreatedAt.Truncate(time.Microsecond), response.CreatedAt)
	assert.Equal(t, latestSnapshot.RepositoryPath, response.RepositoryPath)
}

func (s *SnapshotsSuite) TestFetchSnapshotsByDateAndRepository() {
	t := s.T()
	tx := s.tx

	repoConfig := s.createRepository()
	baseTime := time.Now()
	s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(-time.Hour*30)) // Before Date
	second := s.createSnapshotAtSpecifiedTime(repoConfig, baseTime)          // Target Date
	s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(time.Hour*30))  // After Date

	sDao := GetSnapshotDao(tx)

	request := api.ListSnapshotByDateRequest{}

	request.Date = strings.Split(second.Base.CreatedAt.String(), " ")[0]

	request.RepositoryUUIDS = []string{repoConfig.UUID}

	response, err := sDao.FetchSnapshotsByDateAndRepository(repoConfig.OrgID, request)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(response))
	assert.Equal(t, false, response[0].IsAfter)
	assert.Equal(t, second.Base.UUID, response[0].Match.UUID)
	assert.Equal(t, second.Base.CreatedAt.Day(), response[0].Match.CreatedAt.Day())
}

func (s *SnapshotsSuite) TestFetchSnapshotsByDateAndRepositoryMulti() {
	t := s.T()
	tx := s.tx

	repoConfig := s.createRepository()
	repoConfig2 := s.createRepository()
	redhatRepo := s.createRedhatRepository()

	baseTime := time.Now()
	s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(-time.Hour*30)) // Before Date
	target1 := s.createSnapshotAtSpecifiedTime(repoConfig, baseTime)         // Closest to Target Date
	s.createSnapshotAtSpecifiedTime(repoConfig, baseTime.Add(time.Hour*30))  // After Date

	target2 := s.createSnapshotAtSpecifiedTime(repoConfig2, baseTime.Add(time.Hour*30)) // Target Date with IsAfter = true
	s.createSnapshotAtSpecifiedTime(repoConfig2, baseTime.Add(time.Hour*70))            // After Date
	s.createSnapshotAtSpecifiedTime(repoConfig2, baseTime.Add(time.Hour*90))            // After Date

	s.createSnapshotAtSpecifiedTime(redhatRepo, baseTime.Add(-time.Hour*600))            // Before Date
	s.createSnapshotAtSpecifiedTime(redhatRepo, baseTime.Add(-time.Hour*200))            // Before Date
	target3 := s.createSnapshotAtSpecifiedTime(redhatRepo, baseTime.Add(-time.Hour*100)) // Closest to Target Date

	request := api.ListSnapshotByDateRequest{}
	request.Date = strings.Split(target1.Base.CreatedAt.String(), " ")[0]

	// Intentionally not found ID
	randomUUID, _ := uuid.NewUUID()

	request.RepositoryUUIDS = []string{
		repoConfig.UUID,
		repoConfig2.UUID,
		redhatRepo.UUID,
		randomUUID.String(),
	}

	sDao := GetSnapshotDao(tx)

	response, err := sDao.FetchSnapshotsByDateAndRepository(repoConfig.OrgID, request)

	assert.NoError(t, err)
	assert.Equal(t, 4, len(response))
	// target 1
	assert.Equal(t, false, response[0].IsAfter)
	assert.Equal(t, target1.Base.UUID, response[0].Match.UUID)
	assert.Equal(t, target1.Base.CreatedAt.Day(), response[0].Match.CreatedAt.Day())

	// target 2
	assert.Equal(t, true, response[1].IsAfter)
	assert.Equal(t, target2.Base.UUID, response[1].Match.UUID)
	assert.Equal(t, target2.Base.CreatedAt.Day(), response[1].Match.CreatedAt.Day())

	// target 3 < RedHat repo before the expected date
	assert.Equal(t, false, response[2].IsAfter)
	assert.Equal(t, target3.Base.UUID, response[2].Match.UUID)
	assert.Equal(t, target3.Base.CreatedAt.Day(), response[2].Match.CreatedAt.Day())

	// target 4 < RandomUUID Expect empty state
	assert.Equal(t, randomUUID.String(), response[3].RepositoryUUID)
	assert.Equal(t, false, response[3].IsAfter)
	assert.Empty(t, response[3].Match) // Expect empty struct
}

func (s *SnapshotsSuite) TestFetchLatestSnapshotNotFound() {
	t := s.T()
	tx := s.tx

	repoConfig := s.createRepository()

	sDao := GetSnapshotDao(tx)
	_, err := sDao.FetchLatestSnapshot(repoConfig.UUID)
	assert.Equal(t, err, gorm.ErrRecordNotFound)
}

func (s *SnapshotsSuite) TestGetRepositoryConfigurationFile() {
	t := s.T()
	tx := s.tx

	host := "example.com:9000/"
	mockPulpClient := pulp_client.NewMockPulpClient(t)
	sDaoImpl := snapshotDaoImpl{db: tx, pulpClient: mockPulpClient}
	sDao := sDaoImpl.WithContext(context.Background())

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
	mockPulpClient.WithContextMock().On("GetContentPath").Return(testContentPath, nil).Once()
	repoConfigFile, err := sDao.GetRepositoryConfigurationFile(repoConfig.OrgID, snapshot.UUID, repoConfig.UUID, host)
	assert.NoError(t, err)
	assert.Contains(t, repoConfigFile, repoConfig.Name)
	assert.Contains(t, repoConfigFile, expectedRepoID)
	assert.Contains(t, repoConfigFile, testContentPath)
	assert.Contains(t, repoConfigFile, "module_hotfixes=0")

	// Test error from pulp call
	mockPulpClient.WithContextMock().On("GetContentPath").Return("", fmt.Errorf("some error")).Once()
	repoConfigFile, err = sDao.GetRepositoryConfigurationFile(repoConfig.OrgID, snapshot.UUID, repoConfig.UUID, host)
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

	mockPulpClient.WithContextMock().On("GetContentPath").Return(testContentPath, nil).Once()
	repoConfigFile, err = sDao.GetRepositoryConfigurationFile(repoConfigRh.OrgID, snapshot.UUID, repoConfigRh.UUID, host)
	assert.NoError(t, err)
	assert.Contains(t, repoConfigFile, repoConfigRh.Name)
	assert.Contains(t, repoConfigFile, config.RedHatGpgKeyPath)
}

func (s *SnapshotsSuite) TestGetRepositoryConfigurationFileNotFound() {
	t := s.T()
	tx := s.tx

	host := "example.com:9000/"
	mockPulpClient := pulp_client.MockPulpClient{}
	sDao := snapshotDaoImpl{db: tx, pulpClient: &mockPulpClient}
	repoConfig := s.createRepository()
	snapshot := s.createSnapshot(repoConfig)

	if config.Get().Features.Snapshots.Enabled {
		mockPulpClient.WithContextMock().On("GetContentPath").Return(testContentPath, nil).Times(3)
	}

	// Test bad repo UUID
	mockPulpClient.On("GetContentPath").Return(testContentPath, nil).Once()
	repoConfigFile, err := sDao.WithContext(context.Background()).GetRepositoryConfigurationFile(repoConfig.OrgID, snapshot.UUID, uuid2.NewString(), host)
	assert.Error(t, err)
	if err != nil {
		daoError, ok := err.(*ce.DaoError)
		assert.True(t, ok)
		assert.True(t, daoError.NotFound)
		assert.Contains(t, daoError.Message, "Could not find repository")
	}
	assert.Empty(t, repoConfigFile)

	// Test bad snapshot UUID
	mockPulpClient.On("GetContentPath").Return(testContentPath, nil).Once()
	repoConfigFile, err = sDao.WithContext(context.Background()).GetRepositoryConfigurationFile(repoConfig.OrgID, uuid2.NewString(), repoConfig.UUID, host)
	assert.Error(t, err)
	if err != nil {
		daoError, ok := err.(*ce.DaoError)
		assert.True(t, ok)
		assert.True(t, daoError.NotFound)
		assert.Contains(t, daoError.Message, "Could not find snapshot")
	}
	assert.Empty(t, repoConfigFile)

	//  Test bad org ID
	mockPulpClient.On("GetContentPath").Return(testContentPath, nil).Once()
	repoConfigFile, err = sDao.WithContext(context.Background()).GetRepositoryConfigurationFile("bad orgID", snapshot.UUID, repoConfig.UUID, host)
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

	snap, err := sDao.FetchSnapshotByVersionHref(repoConfig.UUID, snapshot.VersionHref)
	require.NoError(t, err)
	assert.NotNil(t, snap)

	snap, err = sDao.FetchSnapshotByVersionHref(repoConfig.UUID, "Not areal href")
	require.NoError(t, err)
	assert.Nil(t, snap)
}
