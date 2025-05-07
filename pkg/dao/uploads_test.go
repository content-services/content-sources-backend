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

	err = uploadDao.StoreChunkUpload(ctx, orgIDTest, uploadUUID, chunkSha)

	assert.Equal(s.T(), nil, err)

	existingUploadUUID, chunkList, err = uploadDao.GetExistingUploadIDAndCompletedChunks(ctx, orgIDTest, uploadSha, chunkSize, uploadSize)

	assert.Equal(s.T(), nil, err)
	assert.Equal(s.T(), uploadUUID, existingUploadUUID)
	assert.Equal(s.T(), []string{chunkSha}, chunkList)
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
