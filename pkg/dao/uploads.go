package dao

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"gorm.io/gorm"
)

type uploadDaoImpl struct {
	db         *gorm.DB
	pulpClient pulp_client.PulpClient
}

func GetUploadDao(db *gorm.DB, pulpClient pulp_client.PulpClient) UploadDao {
	return &uploadDaoImpl{
		db:         db,
		pulpClient: pulpClient,
	}
}

func (t uploadDaoImpl) StoreFileUpload(ctx context.Context, orgID string, uploadUUID string, sha256 string, chunkSize int64) error {
	var upload models.Upload

	upload.OrgID = orgID
	upload.UploadUUID = uploadUUID
	upload.Sha256 = sha256
	upload.ChunkSize = chunkSize

	upload.ChunkList = []string{}

	db := t.db.Model(models.Upload{}).WithContext(ctx).Create(&upload)
	if db.Error != nil {
		return db.Error
	}

	return nil
}

func (t uploadDaoImpl) GetExistingUploadIDAndCompletedChunks(ctx context.Context, orgID string, sha256 string, chunkSize int64) (string, []string, error) {
	db := t.db.Model(models.Upload{}).WithContext(ctx)

	var result models.Upload

	db.Where("org_id = ?", orgID).Where("chunk_size = ?", chunkSize).Where("sha256 = ?", sha256).First(&result)

	if db.Error != nil {
		return "", []string{}, db.Error
	}

	return result.UploadUUID, result.ChunkList, nil
}

func (t uploadDaoImpl) StoreChunkUpload(ctx context.Context, orgID string, uploadUUID string, sha256 string) error {
	db := t.db.Model(models.Upload{}).
		WithContext(ctx).
		Where("org_id = ?", orgID).
		Where("upload_uuid = ?", uploadUUID).
		Where("? != all(chunk_list)", sha256).
		Update("chunk_list", gorm.Expr(`array_append(chunk_list, ?)`, sha256))

	if db.Error != nil {
		return db.Error
	}

	return nil
}