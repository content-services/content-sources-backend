package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrClearinghouseSubmissionClosed = errors.New("clearinghouse submission can no longer be changed")

type ClearinghouseUpload struct {
	UUID               uuid.UUID
	CreatedAt          time.Time
	OrgID              string
	Username           string
	Name               string
	Filename           string
	VulnerabilityCount *int32
	Status             string
	ErrorMessage       *string
	SubmittedAt        *time.Time
	SubmittedBy        *string
	ProcessedAt        *time.Time
	ProcessedBy        *string
	PackageSummary     *api.ClearinghousePackageSummary
}

type ClearinghouseUploadFile struct {
	UUID       uuid.UUID
	UploadUUID uuid.UUID
	Filename   string
	Filepath   string
	Username   string
	Submission bool
	CreatedAt  time.Time
}

type CreateClearinghouseUploadParams struct {
	UUID               uuid.UUID
	OrgID              string
	Username           string
	Name               string
	Filename           string
	VulnerabilityCount *int32
	Status             string
	// Packages are stored in order with the upload. PackageSummary is stored on the upload.
	Packages       []CreateClearinghouseUploadPackageParams
	PackageSummary *api.ClearinghousePackageSummary
}

// CreateClearinghouseUploadPackageParams is one unique package parsed from a submission.
// ValidPurl is false when Packageurl does not parse (Ecosystem is Unknown and
// Namespace, Name and Version are empty) or has no version.
type CreateClearinghouseUploadPackageParams struct {
	Packageurl string
	Ecosystem  string
	Namespace  string
	Name       string
	Version    string
	ValidPurl  bool
	// InvalidReason is an api.ClearinghouseInvalidReason* code and InvalidDetail
	// explains it. Both are empty when ValidPurl is true.
	InvalidReason string
	InvalidDetail string
}

type CreateClearinghouseUploadFileParams struct {
	UUID       uuid.UUID
	UploadUUID uuid.UUID
	Filename   string
	Filepath   string
	Username   string
	Submission bool
}

// ClearinghouseUploadPackage is one unique package parsed from a submission.
// ValidPurl is false when Packageurl does not parse (Ecosystem is Unknown and
// Namespace, Name and Version are empty) or has no version.
type ClearinghouseUploadPackage struct {
	Packageurl string
	Ecosystem  string
	Namespace  string
	Name       string
	Version    string
	ValidPurl  bool
	// InvalidReason is an api.ClearinghouseInvalidReason* code and InvalidDetail
	// explains it. Both are empty when ValidPurl is true.
	InvalidReason string
	InvalidDetail string
}

// ListClearinghouseUploadPackagesParams filters the packages of one upload.
// Nil or empty filters match every package.
type ListClearinghouseUploadPackagesParams struct {
	Ecosystems []string
	ValidPurl  *bool
	Limit      int32
	Offset     int32
}

type ListClearinghouseUploadsParams struct {
	OrgID          *string
	OrgIDs         []string
	RestrictOrgIDs bool
	Limit          int32
	Offset         int32
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
		Name:     params.Name,
		Filename: params.Filename,
		Status:   params.Status,
	})
	if err != nil {
		return ClearinghouseUpload{}, err
	}
	return clearinghouseUploadFromStore(row)
}

func (d clearinghouseUploadDaoImpl) CreateWithFiles(ctx context.Context, upload CreateClearinghouseUploadParams, files []CreateClearinghouseUploadFileParams, afterCommit func([]ClearinghouseUploadFile) error) (ClearinghouseUpload, []ClearinghouseUploadFile, error) {
	if err := d.requireQuerier(); err != nil {
		return ClearinghouseUpload{}, nil, err
	}
	if d.pool == nil {
		return ClearinghouseUpload{}, nil, fmt.Errorf("lightwell pool is not initialized")
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return ClearinghouseUpload{}, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var packageSummary []byte
	if upload.PackageSummary != nil {
		if packageSummary, err = json.Marshal(upload.PackageSummary); err != nil {
			return ClearinghouseUpload{}, nil, err
		}
	}
	q := store.New(tx)
	row, err := q.CreateClearinghouseUpload(ctx, store.CreateClearinghouseUploadParams{
		Uuid:               upload.UUID,
		OrgID:              upload.OrgID,
		Username:           upload.Username,
		Name:               upload.Name,
		Filename:           upload.Filename,
		VulnerabilityCount: upload.VulnerabilityCount,
		Status:             upload.Status,
		PackageSummary:     packageSummary,
	})
	if err != nil {
		return ClearinghouseUpload{}, nil, err
	}
	created, err := clearinghouseUploadFromStore(row)
	if err != nil {
		return ClearinghouseUpload{}, nil, err
	}
	if len(upload.Packages) > 0 {
		if _, err := q.CreateClearinghouseUploadPackages(ctx, clearinghouseUploadPackageRows(upload.UUID, upload.Packages)); err != nil {
			return ClearinghouseUpload{}, nil, err
		}
	}
	out := make([]ClearinghouseUploadFile, 0, len(files))
	for _, file := range files {
		fileRow, err := q.CreateClearinghouseUploadFile(ctx, store.CreateClearinghouseUploadFileParams{
			Uuid:                    file.UUID,
			ClearinghouseUploadUuid: file.UploadUUID,
			Filename:                file.Filename,
			Filepath:                file.Filepath,
			Username:                file.Username,
			Submission:              file.Submission,
		})
		if err != nil {
			return ClearinghouseUpload{}, nil, err
		}
		out = append(out, clearinghouseUploadFileFromStore(fileRow))
	}
	if err := tx.Commit(ctx); err != nil {
		return ClearinghouseUpload{}, nil, err
	}
	if afterCommit != nil {
		if err := afterCommit(out); err != nil {
			if delErr := d.querier.DeleteClearinghouseUpload(ctx, upload.UUID); delErr != nil {
				return ClearinghouseUpload{}, nil, errors.Join(err, delErr)
			}
			return ClearinghouseUpload{}, nil, err
		}
	}
	return created, out, nil
}

func (d clearinghouseUploadDaoImpl) Get(ctx context.Context, uploadID uuid.UUID) (ClearinghouseUpload, error) {
	if err := d.requireQuerier(); err != nil {
		return ClearinghouseUpload{}, err
	}
	row, err := d.querier.GetClearinghouseUpload(ctx, uploadID)
	if err != nil {
		return ClearinghouseUpload{}, err
	}
	return clearinghouseUploadFromStore(row)
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
		OrgID:          params.OrgID,
		RestrictOrgIds: params.RestrictOrgIDs,
		OrgIds:         clearinghouseOrgIDs(params.OrgIDs),
		PageLimit:      params.Limit,
		PageOffset:     params.Offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ClearinghouseUpload, 0, len(rows))
	for _, row := range rows {
		upload, err := clearinghouseUploadFromStore(row)
		if err != nil {
			return nil, err
		}
		out = append(out, upload)
	}
	return out, nil
}

func (d clearinghouseUploadDaoImpl) Count(ctx context.Context, params ListClearinghouseUploadsParams) (int64, error) {
	if err := d.requireQuerier(); err != nil {
		return 0, err
	}
	return d.querier.CountClearinghouseUploads(ctx, store.CountClearinghouseUploadsParams{
		OrgID:          params.OrgID,
		RestrictOrgIds: params.RestrictOrgIDs,
		OrgIds:         clearinghouseOrgIDs(params.OrgIDs),
	})
}

func clearinghouseOrgIDs(orgIDs []string) []string {
	if orgIDs == nil {
		return []string{}
	}
	return orgIDs
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
		Username:                params.Username,
		Submission:              params.Submission,
	})
	if err != nil {
		return ClearinghouseUploadFile{}, err
	}
	return clearinghouseUploadFileFromStore(row), nil
}

func (d clearinghouseUploadDaoImpl) CreateFiles(ctx context.Context, params []CreateClearinghouseUploadFileParams, afterCommit func([]ClearinghouseUploadFile) error) ([]ClearinghouseUploadFile, error) {
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
		if upload.Status != api.ClearinghouseSubmissionStatusPending {
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
			Username:                file.Username,
			Submission:              file.Submission,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, clearinghouseUploadFileFromStore(row))
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if afterCommit != nil {
		if err := afterCommit(out); err != nil {
			var delErrs []error
			for _, file := range out {
				if delErr := d.querier.DeleteClearinghouseUploadFile(ctx, file.UUID); delErr != nil {
					delErrs = append(delErrs, delErr)
				}
			}
			if len(delErrs) > 0 {
				return nil, errors.Join(append([]error{err}, delErrs...)...)
			}
			return nil, err
		}
	}
	return out, nil
}

func (d clearinghouseUploadDaoImpl) DeleteIfUploading(ctx context.Context, uploadID uuid.UUID, afterCommit func([]ClearinghouseUploadFile) error) error {
	var files []ClearinghouseUploadFile
	err := d.withUploadingUpload(ctx, uploadID, func(q *store.Queries) error {
		rows, err := q.ListClearinghouseUploadFiles(ctx, uploadID)
		if err != nil {
			return err
		}
		files = clearinghouseUploadFilesFromStore(rows)
		return q.DeleteClearinghouseUpload(ctx, uploadID)
	})
	if err != nil {
		return err
	}
	if afterCommit != nil {
		return afterCommit(files)
	}
	return nil
}

func (d clearinghouseUploadDaoImpl) DeleteFileIfUploading(ctx context.Context, uploadID uuid.UUID, filename string, afterCommit func(ClearinghouseUploadFile) error) error {
	var file ClearinghouseUploadFile
	err := d.withUploadingUpload(ctx, uploadID, func(q *store.Queries) error {
		row, err := q.GetClearinghouseUploadFileByName(ctx, store.GetClearinghouseUploadFileByNameParams{
			ClearinghouseUploadUuid: uploadID,
			Filename:                filename,
		})
		if err != nil {
			return err
		}
		file = clearinghouseUploadFileFromStore(row)
		return q.DeleteClearinghouseUploadFile(ctx, file.UUID)
	})
	if err != nil {
		return err
	}
	if afterCommit != nil {
		return afterCommit(file)
	}
	return nil
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
	if upload.Status != api.ClearinghouseSubmissionStatusPending {
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

func (d clearinghouseUploadDaoImpl) ListPackages(ctx context.Context, uploadID uuid.UUID, params ListClearinghouseUploadPackagesParams) ([]ClearinghouseUploadPackage, int64, error) {
	if err := d.requireQuerier(); err != nil {
		return nil, 0, err
	}
	var ecosystems []string
	if len(params.Ecosystems) > 0 {
		ecosystems = params.Ecosystems
	}
	var validPurl pgtype.Bool
	if params.ValidPurl != nil {
		validPurl = pgtype.Bool{Bool: *params.ValidPurl, Valid: true}
	}
	rows, err := d.querier.ListClearinghouseUploadPackages(ctx, store.ListClearinghouseUploadPackagesParams{
		ClearinghouseUploadUuid: uploadID,
		Ecosystems:              ecosystems,
		ValidPurl:               validPurl,
		PageLimit:               params.Limit,
		PageOffset:              params.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := d.querier.CountClearinghouseUploadPackages(ctx, store.CountClearinghouseUploadPackagesParams{
		ClearinghouseUploadUuid: uploadID,
		Ecosystems:              ecosystems,
		ValidPurl:               validPurl,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]ClearinghouseUploadPackage, 0, len(rows))
	for _, row := range rows {
		pkg := ClearinghouseUploadPackage{
			Packageurl: row.Packageurl,
			Ecosystem:  row.Ecosystem,
			Name:       row.Name,
			Version:    row.Version,
			ValidPurl:  row.ValidPurl,
		}
		if row.Namespace != nil {
			pkg.Namespace = *row.Namespace
		}
		if row.InvalidReason != nil {
			pkg.InvalidReason = *row.InvalidReason
		}
		if row.InvalidDetail != nil {
			pkg.InvalidDetail = *row.InvalidDetail
		}
		out = append(out, pkg)
	}
	return out, total, nil
}

func clearinghouseUploadPackageRows(uploadID uuid.UUID, packages []CreateClearinghouseUploadPackageParams) []store.CreateClearinghouseUploadPackagesParams {
	rows := make([]store.CreateClearinghouseUploadPackagesParams, len(packages))
	for i, pkg := range packages {
		rows[i] = store.CreateClearinghouseUploadPackagesParams{
			Uuid:                    uuid.New(),
			ClearinghouseUploadUuid: uploadID,
			Position:                int32(i), //nolint:gosec // package count is bounded by the 50 MiB submission limit
			Packageurl:              pkg.Packageurl,
			Ecosystem:               pkg.Ecosystem,
			Namespace:               optionalString(pkg.Namespace),
			Name:                    pkg.Name,
			Version:                 pkg.Version,
			ValidPurl:               pkg.ValidPurl,
			InvalidReason:           optionalString(pkg.InvalidReason),
			InvalidDetail:           optionalString(pkg.InvalidDetail),
		}
	}
	return rows
}

func (d clearinghouseUploadDaoImpl) Submit(ctx context.Context, uploadID uuid.UUID, username string) (ClearinghouseUpload, error) {
	if err := d.requireQuerier(); err != nil {
		return ClearinghouseUpload{}, err
	}
	row, err := d.querier.SubmitClearinghouseUpload(ctx, store.SubmitClearinghouseUploadParams{
		SubmittedBy: &username,
		Uuid:        uploadID,
	})
	if err != nil {
		return ClearinghouseUpload{}, err
	}
	return clearinghouseUploadFromStore(row)
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
	return clearinghouseUploadFromStore(row)
}

func clearinghouseUploadFromStore(row store.ClearinghouseUpload) (ClearinghouseUpload, error) {
	var packageSummary *api.ClearinghousePackageSummary
	if len(row.PackageSummary) > 0 {
		packageSummary = &api.ClearinghousePackageSummary{}
		if err := json.Unmarshal(row.PackageSummary, packageSummary); err != nil {
			return ClearinghouseUpload{}, fmt.Errorf("decoding package summary of clearinghouse upload %s: %w", row.Uuid, err)
		}
	}
	return ClearinghouseUpload{
		UUID:               row.Uuid,
		CreatedAt:          row.CreatedAt,
		OrgID:              row.OrgID,
		Username:           row.Username,
		Name:               row.Name,
		Filename:           row.Filename,
		VulnerabilityCount: row.VulnerabilityCount,
		Status:             row.Status,
		ErrorMessage:       row.ErrorMessage,
		SubmittedAt:        row.SubmittedAt,
		SubmittedBy:        row.SubmittedBy,
		ProcessedAt:        row.ProcessedAt,
		ProcessedBy:        row.ProcessedBy,
		PackageSummary:     packageSummary,
	}, nil
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
		Username:   row.Username,
		Submission: row.Submission,
		CreatedAt:  row.CreatedAt,
	}
}
