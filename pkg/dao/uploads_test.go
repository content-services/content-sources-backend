package dao

import (
	"context"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UploadsSuite struct {
	*DaoSuite
	mockPulpClient *pulp_client.MockPulpClient
}

func TestUploadsSuite(t *testing.T) {
	m := DaoSuite{}
	r := UploadsSuite{DaoSuite: &m}
	suite.Run(t, &r)
}

func (s *UploadsSuite) uploadsDao() uploadDaoImpl {
	return uploadDaoImpl{
		db:         s.tx,
		pulpClient: s.mockPulpClient,
	}
}

func (s *UploadsSuite) SetupTest() {
	s.DaoSuite.SetupTest()
}

func (s *UploadsSuite) TestStoreFileUpload() {
	uploadDao := s.uploadsDao()
	ctx := context.Background()
	uploadUUID := "bananaUUID"
	chunkSize := int64(16000)
	uploadSize := int64(16000)
	uploadSha := "bananaHash256"
	chunkSha := "bananaChunkHash256"

	err := uploadDao.StoreFileUpload(ctx, orgIDTest, uploadUUID, uploadSha, chunkSize, uploadSize)

	assert.Equal(s.T(), err, nil)

	existingUploadUUID, chunkList, err := uploadDao.GetExistingUploadIDAndCompletedChunks(ctx, orgIDTest, uploadSha, chunkSize, uploadSize)

	assert.Equal(s.T(), nil, err)
	assert.Equal(s.T(), uploadUUID, existingUploadUUID)
	assert.Equal(s.T(), []string{}, chunkList)

	var upload models.Upload
	require.NoError(s.T(), s.tx.Where("upload_uuid = ?", uploadUUID).First(&upload).Error)
	require.NotNil(s.T(), upload.ChunkMetadata)
	assert.Empty(s.T(), upload.ChunkMetadata)

	existingChunk, err := uploadDao.GetCompletedUploadChunk(ctx, orgIDTest, uploadUUID, chunkSha, UploadChunkRange{Offset: 0, Size: 100})
	require.NoError(s.T(), err)
	assert.Nil(s.T(), existingChunk)

	err = uploadDao.StoreChunkUpload(ctx, orgIDTest, uploadUUID, chunkSha, UploadChunkRange{Offset: 0, Size: 100})

	assert.Equal(s.T(), nil, err)

	existingUploadUUID, chunkList, err = uploadDao.GetExistingUploadIDAndCompletedChunks(ctx, orgIDTest, uploadSha, chunkSize, uploadSize)

	assert.Equal(s.T(), nil, err)
	assert.Equal(s.T(), uploadUUID, existingUploadUUID)
	assert.Equal(s.T(), []string{chunkSha}, chunkList)

	require.NoError(s.T(), s.tx.Where("upload_uuid = ?", uploadUUID).First(&upload).Error)
	assert.Equal(s.T(), models.UploadChunkMetadata{
		"0": {Size: 100, Sha256: chunkSha},
	}, upload.ChunkMetadata)
}

func (s *UploadsSuite) TestGetCompletedUploadChunk() {
	uploadDao := s.uploadsDao()
	ctx := context.Background()
	uploadUUID := uuid.NewString()
	uploadSize := int64(1000)
	chunkSha := "chunk-sha256"
	chunkRange := UploadChunkRange{Offset: 100, Size: 100}

	require.NoError(s.T(), uploadDao.StoreFileUpload(ctx, orgIDTest, uploadUUID, "file-sha256", 100, uploadSize))
	require.NoError(s.T(), uploadDao.StoreChunkUpload(ctx, orgIDTest, uploadUUID, chunkSha, chunkRange))

	// An upload created before range tracking can have checksums without metadata.
	legacyUpload := models.Upload{
		UploadUUID: uuid.NewString(),
		OrgID:      orgIDTest,
		ChunkSize:  100,
		Size:       uploadSize,
		Sha256:     "legacy-file-sha256",
		ChunkList:  []string{chunkSha},
	}
	require.NoError(s.T(), s.tx.Create(&legacyUpload).Error)

	testCases := []struct {
		name       string
		orgID      string
		uploadUUID string
		sha256     string
		chunkRange UploadChunkRange
		found      bool
	}{
		{"exact match", orgIDTest, uploadUUID, chunkSha, chunkRange, true},
		{"different offset", orgIDTest, uploadUUID, chunkSha, UploadChunkRange{Offset: 200, Size: 100}, false},
		{"different size", orgIDTest, uploadUUID, chunkSha, UploadChunkRange{Offset: 100, Size: 99}, false},
		{"different checksum", orgIDTest, uploadUUID, "other-chunk-sha256", chunkRange, false},
		{"different organization", orgIDTest + "-other", uploadUUID, chunkSha, chunkRange, false},
		{"legacy checksum without metadata", orgIDTest, legacyUpload.UploadUUID, chunkSha, chunkRange, false},
		{"missing upload", orgIDTest, uuid.NewString(), chunkSha, chunkRange, false},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			upload, err := uploadDao.GetCompletedUploadChunk(ctx, tc.orgID, tc.uploadUUID, tc.sha256, tc.chunkRange)
			require.NoError(s.T(), err)
			if !tc.found {
				assert.Nil(s.T(), upload)
				return
			}

			require.NotNil(s.T(), upload)
			assert.Equal(s.T(), uploadUUID, upload.UploadUUID)
			assert.Equal(s.T(), uploadSize, upload.Size)
		})
	}
}

func (s *UploadsSuite) TestStoreChunkUploadMetadata() {
	uploadDao := s.uploadsDao()
	ctx := context.Background()
	uploadUUID := uuid.NewString()
	chunkSha := "chunk-sha256"
	firstRange := UploadChunkRange{Offset: 0, Size: 100}
	secondRange := UploadChunkRange{Offset: 100, Size: 100}

	require.NoError(s.T(), uploadDao.StoreFileUpload(ctx, orgIDTest, uploadUUID, "file-sha256", 100, 1000))
	require.NoError(s.T(), uploadDao.StoreChunkUpload(ctx, orgIDTest, uploadUUID, chunkSha, firstRange))
	require.NoError(s.T(), uploadDao.StoreChunkUpload(ctx, orgIDTest, uploadUUID, chunkSha, secondRange))

	var upload models.Upload
	require.NoError(s.T(), s.tx.Where("upload_uuid = ?", uploadUUID).First(&upload).Error)
	assert.Equal(s.T(), models.UploadChunkMetadata{
		"0":   {Size: 100, Sha256: chunkSha},
		"100": {Size: 100, Sha256: chunkSha},
	}, upload.ChunkMetadata)

	for _, chunkRange := range []UploadChunkRange{firstRange, secondRange} {
		existing, err := uploadDao.GetCompletedUploadChunk(ctx, orgIDTest, uploadUUID, chunkSha, chunkRange)
		require.NoError(s.T(), err)
		require.NotNil(s.T(), existing)
		assert.Equal(s.T(), uploadUUID, existing.UploadUUID)
	}

	replacementSha := "replacement-sha256"
	replacementRange := UploadChunkRange{Offset: 0, Size: 50}
	require.NoError(s.T(), uploadDao.StoreChunkUpload(ctx, orgIDTest, uploadUUID, replacementSha, replacementRange))

	require.NoError(s.T(), s.tx.Where("upload_uuid = ?", uploadUUID).First(&upload).Error)
	assert.Equal(s.T(), models.UploadChunkMetadata{
		"0":   {Size: 50, Sha256: replacementSha},
		"100": {Size: 100, Sha256: chunkSha},
	}, upload.ChunkMetadata)
	assert.Contains(s.T(), upload.ChunkList, replacementSha)

	existing, err := uploadDao.GetCompletedUploadChunk(ctx, orgIDTest, uploadUUID, chunkSha, firstRange)
	require.NoError(s.T(), err)
	assert.Nil(s.T(), existing)

	existing, err = uploadDao.GetCompletedUploadChunk(ctx, orgIDTest, uploadUUID, replacementSha, replacementRange)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), existing)
	assert.Equal(s.T(), uploadUUID, existing.UploadUUID)
}

func (s *UploadsSuite) TestStoreChunkUploadScope() {
	uploadDao := s.uploadsDao()
	ctx := context.Background()
	uploadUUID := uuid.NewString()
	chunkRange := UploadChunkRange{Offset: 0, Size: 100}

	require.NoError(s.T(), uploadDao.StoreFileUpload(ctx, orgIDTest, uploadUUID, "file-sha256", 100, 1000))
	require.NoError(s.T(), uploadDao.StoreChunkUpload(ctx, orgIDTest, uploadUUID, "chunk-sha256", chunkRange))
	var before models.Upload
	require.NoError(s.T(), s.tx.Where("upload_uuid = ?", uploadUUID).First(&before).Error)

	testCases := []struct {
		name       string
		orgID      string
		uploadUUID string
	}{
		{"different organization", orgIDTest + "-other", uploadUUID},
		{"missing upload", orgIDTest, uuid.NewString()},
	}
	for _, tc := range testCases {
		s.Run(tc.name, func() {
			err := uploadDao.StoreChunkUpload(ctx, tc.orgID, tc.uploadUUID, "replacement-sha256", chunkRange)
			require.NoError(s.T(), err)

			var after models.Upload
			require.NoError(s.T(), s.tx.Where("upload_uuid = ?", uploadUUID).First(&after).Error)
			assert.Equal(s.T(), before, after)
		})
	}
}

func (s *UploadsSuite) TestChunkUploadCanceledContext() {
	uploadDao := s.uploadsDao()
	ctx := context.Background()
	uploadUUID := uuid.NewString()
	chunkSha := "chunk-sha256"
	chunkRange := UploadChunkRange{Offset: 0, Size: 100}

	require.NoError(s.T(), uploadDao.StoreFileUpload(ctx, orgIDTest, uploadUUID, "file-sha256", 100, 1000))
	require.NoError(s.T(), uploadDao.StoreChunkUpload(ctx, orgIDTest, uploadUUID, chunkSha, chunkRange))
	var before models.Upload
	require.NoError(s.T(), s.tx.Where("upload_uuid = ?", uploadUUID).First(&before).Error)

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	upload, err := uploadDao.GetCompletedUploadChunk(canceledCtx, orgIDTest, uploadUUID, chunkSha, chunkRange)
	require.ErrorIs(s.T(), err, context.Canceled)
	assert.Nil(s.T(), upload)

	err = uploadDao.StoreChunkUpload(canceledCtx, orgIDTest, uploadUUID, "replacement-sha256", chunkRange)
	require.ErrorIs(s.T(), err, context.Canceled)

	var after models.Upload
	require.NoError(s.T(), s.tx.Where("upload_uuid = ?", uploadUUID).First(&after).Error)
	assert.Equal(s.T(), before, after)
}

func (s *UploadsSuite) TestDeleteUpload() {
	uploadDao := s.uploadsDao()
	ctx := context.Background()
	uploadUUID := uuid.New()
	var found models.Upload

	err := uploadDao.StoreFileUpload(ctx, orgIDTest, uploadUUID.String(), "test-sha", 500, 500)
	require.NoError(s.T(), err)

	err = uploadDao.DeleteUpload(ctx, uploadUUID.String())
	require.NoError(s.T(), err)

	err = s.tx.
		First(&found, "upload_uuid = ?", uploadUUID).
		Error
	require.Error(s.T(), err)
	assert.Equal(s.T(), "record not found", err.Error())
}

func (s *UploadsSuite) TestListUploadsForCleanup() {
	uploadDao := s.uploadsDao()
	ctx := context.Background()

	s.tx.Exec("DELETE FROM uploads")

	// Insert an upload with a timestamp older than 1 day
	oldUpload := models.Upload{
		CreatedAt:  time.Now().AddDate(0, 0, -2), // 2 days ago
		UploadUUID: uuid.NewString(),
		OrgID:      orgIDTest,
		ChunkSize:  int64(1),
		Size:       int64(1),
		Sha256:     uuid.NewString(),
		ChunkList:  []string{uuid.NewString()},
	}
	err := s.tx.Create(&oldUpload).Error
	require.NoError(s.T(), err)

	// Insert an upload with a recent timestamp
	recentUpload := models.Upload{
		CreatedAt:  time.Now(),
		UploadUUID: uuid.NewString(),
		OrgID:      orgIDTest,
		ChunkSize:  int64(1),
		Size:       int64(1),
		Sha256:     uuid.NewString(),
		ChunkList:  []string{uuid.NewString()},
	}
	err = s.tx.Create(&recentUpload).Error
	require.NoError(s.T(), err)

	uploads, err := uploadDao.ListUploadsForCleanup(ctx)
	require.NoError(s.T(), err)

	// Assert that only the old upload was found
	assert.Equal(s.T(), 1, len(uploads))
	assert.Equal(s.T(), oldUpload.UploadUUID, uploads[0].UploadUUID)
}
