package dao

import (
	"context"
	"errors"
	"strconv"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

type uploadDaoImpl struct {
	db         *gorm.DB
	pulpClient pulp_client.PulpClient
}

type UploadChunkRange struct {
	Offset int64
	Size   int64
}

func GetUploadDao(db *gorm.DB, pulpClient pulp_client.PulpClient) UploadDao {
	return &uploadDaoImpl{
		db:         db,
		pulpClient: pulpClient,
	}
}

func (t uploadDaoImpl) StoreFileUpload(ctx context.Context, orgID string, uploadUUID string, sha256 string, chunkSize int64, uploadSize int64) error {
	var upload models.Upload

	upload.OrgID = orgID
	upload.UploadUUID = uploadUUID
	upload.Sha256 = sha256
	upload.ChunkSize = chunkSize
	upload.Size = uploadSize
	upload.ChunkList = []string{}
	upload.ChunkMetadata = models.UploadChunkMetadata{}

	db := t.db.Model(models.Upload{}).WithContext(ctx).Create(&upload)
	if db.Error != nil {
		return db.Error
	}

	return nil
}

func (t uploadDaoImpl) GetExistingUploadIDAndCompletedChunks(ctx context.Context, orgID string, sha256 string, chunkSize int64, uploadSize int64) (string, []string, error) {
	db := t.db.Model(models.Upload{}).WithContext(ctx)

	var result models.Upload

	db.Where("org_id = ?", orgID).
		Where("chunk_size = ?", chunkSize).
		Where("sha256 = ?", sha256).
		Where("size = ?", uploadSize).
		Order("created_at DESC").
		First(&result)

	if db.Error != nil {
		return "", []string{}, db.Error
	}

	return result.UploadUUID, result.ChunkList, nil
}

func (t uploadDaoImpl) GetCompletedUploadChunk(ctx context.Context, orgID string, uploadUUID string, sha256 string, chunkRange UploadChunkRange) (*models.Upload, error) {
	offsetKey := strconv.FormatInt(chunkRange.Offset, 10)

	var upload models.Upload
	err := t.db.WithContext(ctx).
		Model(&models.Upload{}).
		Select("upload_uuid", "size").
		Where("org_id = ? AND upload_uuid = ?", orgID, uploadUUID).
		Where(
			`(chunk_metadata -> ?::text)
				@> jsonb_build_object(
					'size', ?::bigint,
					'sha256', ?::text
				)`,
			offsetKey,
			chunkRange.Size,
			sha256,
		).
		Take(&upload).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &upload, nil
}

func (t uploadDaoImpl) StoreChunkUpload(ctx context.Context, orgID string, uploadUUID string, sha256 string, chunkRange UploadChunkRange) error {
	offsetKey := strconv.FormatInt(chunkRange.Offset, 10)

	db := t.db.Model(models.Upload{}).
		WithContext(ctx).
		Where("org_id = ?", orgID).
		Where("upload_uuid = ?", uploadUUID).
		Updates(map[string]any{
			"chunk_list": gorm.Expr(`array_append(chunk_list, ?)`, sha256),
			"chunk_metadata": gorm.Expr(
				`jsonb_set(
					chunk_metadata,
					ARRAY[?::text],
					jsonb_build_object('size', ?::bigint, 'sha256', ?::text),
					true
				)`,
				offsetKey,
				chunkRange.Size,
				sha256,
			),
		})

	if db.Error != nil {
		return db.Error
	}

	return nil
}

func (t uploadDaoImpl) DeleteUpload(ctx context.Context, uploadUUID string) error {
	err := t.db.WithContext(ctx).
		Where("upload_uuid = ?", UuidifyString(uploadUUID)).
		Delete(models.Upload{}).Error

	if err != nil {
		return err
	}
	return nil
}

func (t uploadDaoImpl) ListUploadsForCleanup(ctx context.Context) ([]models.Upload, error) {
	var uploads []models.Upload
	err := t.db.WithContext(ctx).
		Where("created_at < current_date - INTERVAL '1' day").
		Find(&uploads).Error
	if err != nil {
		return nil, err
	}
	return uploads, nil
}
