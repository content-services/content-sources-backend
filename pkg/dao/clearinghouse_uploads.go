package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrClearinghouseSubmissionClosed = errors.New("clearinghouse submission can no longer be changed")

type ClearinghouseUpload struct {
	UUID               uuid.UUID
	CreatedAt          time.Time
	OrgID              string
	Username           string
	Filename           string
	VulnerabilityCount *int32
	Status             string
	ErrorMessage       *string
	ProcessedAt        *time.Time
	ProcessedBy        *string
}

type ClearinghouseUploadFile struct {
	UUID       uuid.UUID
	UploadUUID uuid.UUID
	Filename   string
	Filepath   string
}

type CreateClearinghouseUploadParams struct {
	UUID     uuid.UUID
	OrgID    string
	Username string
	Filename string
	Status   string
}

type CreateClearinghouseUploadFileParams struct {
	UUID       uuid.UUID
	UploadUUID uuid.UUID
	Filename   string
	Filepath   string
}

type ListClearinghouseUploadsParams struct {
	OrgID  *string
	Limit  int32
	Offset int32
}

type clearinghouseTxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type clearinghouseUploadDaoImpl struct {
	pool    clearinghouseTxBeginner
	querier store.Querier
}

func newClearinghouseUploadDao(pool clearinghouseTxBeginner, q store.Querier) ClearinghouseUploadDao {
	return clearinghouseUploadDaoImpl{pool: pool, querier: q}
}

var _ ClearinghouseUploadDao = clearinghouseUploadDaoImpl{}

func (d clearinghouseUploadDaoImpl) requireQuerier() error {
	if d.querier == nil {
		return fmt.Errorf("lightwell querier is not initialized")
	}
	return nil
}

func (d clearinghouseUploadDaoImpl) Create(ctx context.Context, params CreateClearinghouseUploadParams) (ClearinghouseUpload, error) {
	if err := d.requireQuerier(); err != nil {
		return ClearinghouseUpload{}, err
	}
	row, err := d.querier.CreateClearinghouseUpload(ctx, store.CreateClearinghouseUploadParams{
		Uuid:     params.UUID,
		OrgID:    params.OrgID,
		Username: params.Username,
		Filename: params.Filename,
		Status:   params.Status,
	})
	if err != nil {
		return ClearinghouseUpload{}, err
	}
	return clearinghouseUploadFromStore(row), nil
}

func (d clearinghouseUploadDaoImpl) Get(ctx context.Context, uploadID uuid.UUID) (ClearinghouseUpload, error) {
	if err := d.requireQuerier(); err != nil {
		return ClearinghouseUpload{}, err
	}
	row, err := d.querier.GetClearinghouseUpload(ctx, uploadID)
	if err != nil {
		return ClearinghouseUpload{}, err
	}
	return clearinghouseUploadFromStore(row), nil
}

func (d clearinghouseUploadDaoImpl) Delete(ctx context.Context, uploadID uuid.UUID) error {
	if err := d.requireQuerier(); err != nil {
		return err
	}
	return d.querier.DeleteClearinghouseUpload(ctx, uploadID)
}

func (d clearinghouseUploadDaoImpl) List(ctx context.Context, params ListClearinghouseUploadsParams) ([]ClearinghouseUpload, error) {
	if err := d.requireQuerier(); err != nil {
		return nil, err
	}
	rows, err := d.querier.ListClearinghouseUploads(ctx, store.ListClearinghouseUploadsParams{
		OrgID:      params.OrgID,
		PageLimit:  params.Limit,
		PageOffset: params.Offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ClearinghouseUpload, 0, len(rows))
	for _, row := range rows {
		out = append(out, clearinghouseUploadFromStore(row))
	}
	return out, nil
}

func (d clearinghouseUploadDaoImpl) Count(ctx context.Context, orgID *string) (int64, error) {
	if err := d.requireQuerier(); err != nil {
		return 0, err
	}
	return d.querier.CountClearinghouseUploads(ctx, orgID)
}

func (d clearinghouseUploadDaoImpl) CreateFile(ctx context.Context, params CreateClearinghouseUploadFileParams) (ClearinghouseUploadFile, error) {
	if err := d.requireQuerier(); err != nil {
		return ClearinghouseUploadFile{}, err
	}
	row, err := d.querier.CreateClearinghouseUploadFile(ctx, store.CreateClearinghouseUploadFileParams{
		Uuid:                    params.UUID,
		ClearinghouseUploadUuid: params.UploadUUID,
		Filename:                params.Filename,
		Filepath:                params.Filepath,
	})
	if err != nil {
		return ClearinghouseUploadFile{}, err
	}
	return clearinghouseUploadFileFromStore(row), nil
}

func (d clearinghouseUploadDaoImpl) CreateFiles(ctx context.Context, params []CreateClearinghouseUploadFileParams) ([]ClearinghouseUploadFile, error) {
	if err := d.requireQuerier(); err != nil {
		return nil, err
	}
	if d.pool == nil {
		return nil, fmt.Errorf("lightwell pool is not initialized")
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := store.New(tx)
	locked := map[uuid.UUID]struct{}{}
	for _, file := range params {
		if _, ok := locked[file.UploadUUID]; ok {
			continue
		}
		upload, err := q.GetClearinghouseUploadForUpdate(ctx, file.UploadUUID)
		if err != nil {
			return nil, err
		}
		if upload.Status != api.ClearinghouseSubmissionStatusUploading {
			return nil, ErrClearinghouseSubmissionClosed
		}
		locked[file.UploadUUID] = struct{}{}
	}
	out := make([]ClearinghouseUploadFile, 0, len(params))
	for _, file := range params {
		row, err := q.CreateClearinghouseUploadFile(ctx, store.CreateClearinghouseUploadFileParams{
			Uuid:                    file.UUID,
			ClearinghouseUploadUuid: file.UploadUUID,
			Filename:                file.Filename,
			Filepath:                file.Filepath,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, clearinghouseUploadFileFromStore(row))
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (d clearinghouseUploadDaoImpl) DeleteIfUploading(ctx context.Context, uploadID uuid.UUID, beforeCommit func([]ClearinghouseUploadFile) error) error {
	return d.withUploadingUpload(ctx, uploadID, func(q *store.Queries) error {
		rows, err := q.ListClearinghouseUploadFiles(ctx, uploadID)
		if err != nil {
			return err
		}
		files := clearinghouseUploadFilesFromStore(rows)
		if beforeCommit != nil {
			if err := beforeCommit(files); err != nil {
				return err
			}
		}
		return q.DeleteClearinghouseUpload(ctx, uploadID)
	})
}

func (d clearinghouseUploadDaoImpl) DeleteFileIfUploading(ctx context.Context, uploadID uuid.UUID, filename string, beforeCommit func(ClearinghouseUploadFile) error) error {
	return d.withUploadingUpload(ctx, uploadID, func(q *store.Queries) error {
		row, err := q.GetClearinghouseUploadFileByName(ctx, store.GetClearinghouseUploadFileByNameParams{
			ClearinghouseUploadUuid: uploadID,
			Filename:                filename,
		})
		if err != nil {
			return err
		}
		file := clearinghouseUploadFileFromStore(row)
		if beforeCommit != nil {
			if err := beforeCommit(file); err != nil {
				return err
			}
		}
		return q.DeleteClearinghouseUploadFile(ctx, file.UUID)
	})
}

func (d clearinghouseUploadDaoImpl) withUploadingUpload(ctx context.Context, uploadID uuid.UUID, fn func(q *store.Queries) error) error {
	if err := d.requireQuerier(); err != nil {
		return err
	}
	if d.pool == nil {
		return fmt.Errorf("lightwell pool is not initialized")
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := store.New(tx)
	upload, err := q.GetClearinghouseUploadForUpdate(ctx, uploadID)
	if err != nil {
		return err
	}
	if upload.Status != api.ClearinghouseSubmissionStatusUploading {
		return ErrClearinghouseSubmissionClosed
	}
	if err := fn(q); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d clearinghouseUploadDaoImpl) DeleteFile(ctx context.Context, fileID uuid.UUID) error {
	if err := d.requireQuerier(); err != nil {
		return err
	}
	return d.querier.DeleteClearinghouseUploadFile(ctx, fileID)
}

func (d clearinghouseUploadDaoImpl) ListFiles(ctx context.Context, uploadID uuid.UUID) ([]ClearinghouseUploadFile, error) {
	if err := d.requireQuerier(); err != nil {
		return nil, err
	}
	rows, err := d.querier.ListClearinghouseUploadFiles(ctx, uploadID)
	if err != nil {
		return nil, err
	}
	return clearinghouseUploadFilesFromStore(rows), nil
}

func (d clearinghouseUploadDaoImpl) ListFilesByUploads(ctx context.Context, uploadIDs []uuid.UUID) ([]ClearinghouseUploadFile, error) {
	if err := d.requireQuerier(); err != nil {
		return nil, err
	}
	rows, err := d.querier.ListClearinghouseUploadFilesByUploads(ctx, uploadIDs)
	if err != nil {
		return nil, err
	}
	return clearinghouseUploadFilesFromStore(rows), nil
}

func (d clearinghouseUploadDaoImpl) GetFileByName(ctx context.Context, uploadID uuid.UUID, filename string) (ClearinghouseUploadFile, error) {
	if err := d.requireQuerier(); err != nil {
		return ClearinghouseUploadFile{}, err
	}
	row, err := d.querier.GetClearinghouseUploadFileByName(ctx, store.GetClearinghouseUploadFileByNameParams{
		ClearinghouseUploadUuid: uploadID,
		Filename:                filename,
	})
	if err != nil {
		return ClearinghouseUploadFile{}, err
	}
	return clearinghouseUploadFileFromStore(row), nil
}

func (d clearinghouseUploadDaoImpl) Submit(ctx context.Context, uploadID uuid.UUID) (ClearinghouseUpload, error) {
	if err := d.requireQuerier(); err != nil {
		return ClearinghouseUpload{}, err
	}
	row, err := d.querier.SubmitClearinghouseUpload(ctx, uploadID)
	if err != nil {
		return ClearinghouseUpload{}, err
	}
	return clearinghouseUploadFromStore(row), nil
}

func (d clearinghouseUploadDaoImpl) Process(ctx context.Context, uploadID uuid.UUID, username string) (ClearinghouseUpload, error) {
	if err := d.requireQuerier(); err != nil {
		return ClearinghouseUpload{}, err
	}
	row, err := d.querier.ProcessClearinghouseUpload(ctx, store.ProcessClearinghouseUploadParams{
		ProcessedBy: &username,
		Uuid:        uploadID,
	})
	if err != nil {
		return ClearinghouseUpload{}, err
	}
	return clearinghouseUploadFromStore(row), nil
}

func clearinghouseUploadFromStore(row store.ClearinghouseUpload) ClearinghouseUpload {
	return ClearinghouseUpload{
		UUID:               row.Uuid,
		CreatedAt:          row.CreatedAt,
		OrgID:              row.OrgID,
		Username:           row.Username,
		Filename:           row.Filename,
		VulnerabilityCount: row.VulnerabilityCount,
		Status:             row.Status,
		ErrorMessage:       row.ErrorMessage,
		ProcessedAt:        row.ProcessedAt,
		ProcessedBy:        row.ProcessedBy,
	}
}

func clearinghouseUploadFilesFromStore(rows []store.ClearinghouseUploadFile) []ClearinghouseUploadFile {
	out := make([]ClearinghouseUploadFile, 0, len(rows))
	for _, row := range rows {
		out = append(out, clearinghouseUploadFileFromStore(row))
	}
	return out
}

func clearinghouseUploadFileFromStore(row store.ClearinghouseUploadFile) ClearinghouseUploadFile {
	return ClearinghouseUploadFile{
		UUID:       row.Uuid,
		UploadUUID: row.ClearinghouseUploadUuid,
		Filename:   row.Filename,
		Filepath:   row.Filepath,
	}
}
