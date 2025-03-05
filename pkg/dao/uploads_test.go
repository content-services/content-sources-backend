package dao

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/stretchr/testify/assert"
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

	err := uploadDao.StoreFileUpload(ctx, "bananaOrg", "bananaUUID", "bananaHash256", 16000)

	assert.Equal(s.T(), err, nil)

	uploadUUID, chunkList, err := uploadDao.GetExistingUploadIDAndCompletedChunks(ctx, "bananaOrg", "bananaHash256", 16000)

	assert.Equal(s.T(), nil, err)
	assert.Equal(s.T(), "bananaUUID", uploadUUID)
	assert.Equal(s.T(), []string{}, chunkList)

	err = uploadDao.StoreChunkUpload(ctx, "bananaOrg", "bananaUUID", "bananaChunkHash256")

	assert.Equal(s.T(), nil, err)

	uploadUUID, chunkList, err = uploadDao.GetExistingUploadIDAndCompletedChunks(ctx, "bananaOrg", "bananaHash256", 16000)

	assert.Equal(s.T(), nil, err)
	assert.Equal(s.T(), "bananaUUID", uploadUUID)
	assert.Equal(s.T(), []string{"bananaChunkHash256"}, chunkList)
}
