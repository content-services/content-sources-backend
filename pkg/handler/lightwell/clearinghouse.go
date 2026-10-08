package lightwell

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/s3_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
)

const (
	maxClearinghouseFilesPerRequest = 20
	maxClearinghouseFileBytes       = 50 * 1024 * 1024
)

type clearinghouseUploadStore interface {
	CreateClearinghouseUpload(ctx context.Context, arg store.CreateClearinghouseUploadParams) (store.ClearinghouseUpload, error)
	GetClearinghouseUpload(ctx context.Context, uploadID uuid.UUID) (store.ClearinghouseUpload, error)
	CreateClearinghouseUploadFile(ctx context.Context, arg store.CreateClearinghouseUploadFileParams) (store.ClearinghouseUploadFile, error)
	ListClearinghouseUploadFiles(ctx context.Context, clearinghouseUploadUuid uuid.UUID) ([]store.ClearinghouseUploadFile, error)
	ListClearinghouseUploads(ctx context.Context, arg store.ListClearinghouseUploadsParams) ([]store.ClearinghouseUpload, error)
	CountClearinghouseUploads(ctx context.Context, orgID *string) (int64, error)
	ListClearinghouseUploadFilesByUploads(ctx context.Context, uploadUuids []uuid.UUID) ([]store.ClearinghouseUploadFile, error)
	GetClearinghouseUploadFileByName(ctx context.Context, arg store.GetClearinghouseUploadFileByNameParams) (store.ClearinghouseUploadFile, error)
}

var _ clearinghouseUploadStore = (*store.Queries)(nil)

type ClearinghouseHandler struct {
	Uploads clearinghouseUploadStore
	S3      s3_client.S3Client
}

func checkLightwellClearinghouseAccessible() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if err := handler.CheckLightwellClearinghouseAccessible(c.Request().Context()); err != nil {
				return err
			}
			return next(c)
		}
	}
}

func RegisterClearinghouseRoutes(engine *echo.Group, uploads clearinghouseUploadStore, s3Client s3_client.S3Client) {
	h := ClearinghouseHandler{Uploads: uploads, S3: s3Client}
	clearinghouse := checkLightwellClearinghouseAccessible()
	addLightwellRoute(engine, http.MethodPost, "/clearinghouse/submissions", h.createSubmission, rbac.RbacVerbRead, clearinghouse)
	addLightwellRoute(engine, http.MethodPost, "/clearinghouse/submissions/:uuid/files", h.addSubmissionFiles, rbac.RbacVerbRead, clearinghouse)
	addLightwellRoute(engine, http.MethodGet, "/clearinghouse/submissions", h.listSubmissions, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodGet, "/clearinghouse/submissions/:uuid", h.getSubmission, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodGet, "/clearinghouse/submissions/:uuid/files/:filename", h.downloadSubmissionFile, rbac.RbacVerbRead)
}

// createSubmission godoc
// @Summary      Create a clearinghouse submission
// @ID           createClearinghouseSubmission
// @Description  Upload one file, up to 50 MiB, as a new clearinghouse submission. Requires the Lightwell clearinghouse feature.
// @Tags         lightwell
// @Accept       multipart/form-data
// @Produce      json
// @Param        file formData file true "File to store with the submission. Maximum size is 50 MiB."
// @Success      201 {object} api.ClearinghouseSubmission
// @Failure      400 {object} ce.ErrorResponse
// @Failure      409 {object} ce.ErrorResponse
// @Failure      413 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /clearinghouse/submissions [post]
func (h *ClearinghouseHandler) createSubmission(c echo.Context) error {
	accountID, orgID := handler.GetAccountIdOrgId(c)
	if orgID == "" {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot upload a clearinghouse submission", "Org ID is required")
	}
	header, err := clearinghouseFormFile(c)
	if err != nil {
		return err
	}
	prepared, err := prepareClearinghouseFiles([]*multipart.FileHeader{header})
	if err != nil {
		return err
	}

	uploadID := uuid.New()
	if err = h.putClearinghouseFiles(c, uploadID, prepared); err != nil {
		return err
	}

	var account *string
	if accountID != "" {
		account = &accountID
	}
	upload, err := h.Uploads.CreateClearinghouseUpload(c.Request().Context(), store.CreateClearinghouseUploadParams{
		Uuid:      uploadID,
		OrgID:     orgID,
		AccountID: account,
		Status:    api.ClearinghouseSubmissionStatusValidating,
	})
	if err != nil {
		return clearinghouseStoreError(err, "Error creating clearinghouse submission")
	}
	files, err := h.insertClearinghouseFiles(c, upload.Uuid, prepared)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, clearinghouseSubmissionFromStore(upload, files))
}

// addSubmissionFiles godoc
// @Summary      Add files to a clearinghouse submission
// @ID           addClearinghouseSubmissionFiles
// @Description  Upload more files onto an existing clearinghouse submission. Only the user who created the submission can add files. Requires the Lightwell clearinghouse feature.
// @Tags         lightwell
// @Accept       multipart/form-data
// @Produce      json
// @Param        uuid  path   string true "Clearinghouse submission UUID"
// @Param        files formData file true "Files to add. Repeat the field to upload multiple files."
// @Success      200 {object} api.ClearinghouseSubmission
// @Failure      400 {object} ce.ErrorResponse
// @Failure      403 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      409 {object} ce.ErrorResponse
// @Failure      413 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /clearinghouse/submissions/{uuid}/files [post]
func (h *ClearinghouseHandler) addSubmissionFiles(c echo.Context) error {
	uploadID, err := parseClearinghouseUUID(c.Param("uuid"))
	if err != nil {
		return err
	}
	upload, err := h.Uploads.GetClearinghouseUpload(c.Request().Context(), uploadID)
	if err != nil {
		return clearinghouseStoreError(err, "Error fetching clearinghouse submission")
	}
	accountID, orgID := handler.GetAccountIdOrgId(c)
	if upload.OrgID != orgID {
		return ce.NewErrorResponse(http.StatusNotFound, "Error fetching clearinghouse submission", "Submission not found")
	}
	if upload.AccountID == nil || *upload.AccountID != accountID {
		return ce.NewErrorResponse(http.StatusForbidden, "Cannot add files to clearinghouse submission", "Only the user who created the submission can add files")
	}

	headers, err := clearinghouseFormFiles(c)
	if err != nil {
		return err
	}
	prepared, err := prepareClearinghouseFiles(headers)
	if err != nil {
		return err
	}
	if err = h.putClearinghouseFiles(c, upload.Uuid, prepared); err != nil {
		return err
	}
	if _, err = h.insertClearinghouseFiles(c, upload.Uuid, prepared); err != nil {
		return err
	}
	files, err := h.Uploads.ListClearinghouseUploadFiles(c.Request().Context(), upload.Uuid)
	if err != nil {
		return clearinghouseStoreError(err, "Error listing clearinghouse submission files")
	}
	return c.JSON(http.StatusOK, clearinghouseSubmissionFromStore(upload, files))
}

// listSubmissions godoc
// @Summary      List clearinghouse submissions
// @ID           listClearinghouseSubmissions
// @Description  Customers with Lightwell clearinghouse see their own submissions. Callers with Lightwell clearinghouse read see every submission. File contents are not included.
// @Tags         lightwell
// @Produce      json
// @Param        limit  query int false "Number of items to include in response. Default value: 100."
// @Param        offset query int false "Starting point for retrieving a subset of results. Default value: 0."
// @Success      200 {object} api.ClearinghouseSubmissionCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /clearinghouse/submissions [get]
func (h *ClearinghouseHandler) listSubmissions(c echo.Context) error {
	orgFilter, err := clearinghouseOrgFilter(c)
	if err != nil {
		return err
	}
	page := clearinghousePage(c)
	uploads, err := h.Uploads.ListClearinghouseUploads(c.Request().Context(), store.ListClearinghouseUploadsParams{
		OrgID:      orgFilter,
		PageLimit:  page.limit,
		PageOffset: page.offset,
	})
	if err != nil {
		return clearinghouseStoreError(err, "Error listing clearinghouse submissions")
	}
	total, err := h.Uploads.CountClearinghouseUploads(c.Request().Context(), orgFilter)
	if err != nil {
		return clearinghouseStoreError(err, "Error listing clearinghouse submissions")
	}

	filesByUpload := map[uuid.UUID][]store.ClearinghouseUploadFile{}
	if len(uploads) > 0 {
		ids := make([]uuid.UUID, len(uploads))
		for i, upload := range uploads {
			ids[i] = upload.Uuid
		}
		files, filesErr := h.Uploads.ListClearinghouseUploadFilesByUploads(c.Request().Context(), ids)
		if filesErr != nil {
			return clearinghouseStoreError(filesErr, "Error listing clearinghouse submission files")
		}
		for _, file := range files {
			filesByUpload[file.ClearinghouseUploadUuid] = append(filesByUpload[file.ClearinghouseUploadUuid], file)
		}
	}

	response := api.ClearinghouseSubmissionCollectionResponse{Data: make([]api.ClearinghouseSubmission, 0, len(uploads))}
	for _, upload := range uploads {
		response.Data = append(response.Data, clearinghouseSubmissionFromStore(upload, filesByUpload[upload.Uuid]))
	}
	return c.JSON(http.StatusOK, handler.SetCollectionResponseMetadata(&response, c, total))
}

// getSubmission godoc
// @Summary      Get a clearinghouse submission
// @ID           getClearinghouseSubmission
// @Description  Customers with Lightwell clearinghouse can fetch their own submission. Callers with Lightwell clearinghouse read can fetch any submission. File contents are not included.
// @Tags         lightwell
// @Produce      json
// @Param        uuid path string true "Clearinghouse submission UUID"
// @Success      200 {object} api.ClearinghouseSubmission
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /clearinghouse/submissions/{uuid} [get]
func (h *ClearinghouseHandler) getSubmission(c echo.Context) error {
	upload, err := h.visibleUpload(c)
	if err != nil {
		return err
	}
	files, err := h.Uploads.ListClearinghouseUploadFiles(c.Request().Context(), upload.Uuid)
	if err != nil {
		return clearinghouseStoreError(err, "Error listing clearinghouse submission files")
	}
	return c.JSON(http.StatusOK, clearinghouseSubmissionFromStore(upload, files))
}

// downloadSubmissionFile godoc
// @Summary      Download a clearinghouse submission file
// @ID           downloadClearinghouseSubmissionFile
// @Description  Download a file stored with a clearinghouse submission. Requires the Lightwell clearinghouse read feature.
// @Tags         lightwell
// @Produce      application/octet-stream
// @Param        uuid     path string true "Clearinghouse submission UUID"
// @Param        filename path string true "File name"
// @Success      200 {file} file
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /clearinghouse/submissions/{uuid}/files/{filename} [get]
func (h *ClearinghouseHandler) downloadSubmissionFile(c echo.Context) error {
	if err := handler.CheckLightwellClearinghouseReadAccessible(c.Request().Context()); err != nil {
		return err
	}
	uploadID, err := parseClearinghouseUUID(c.Param("uuid"))
	if err != nil {
		return err
	}
	filename, err := cleanClearinghouseFilename(c.Param("filename"))
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error downloading clearinghouse submission file", err.Error())
	}
	if _, err = h.Uploads.GetClearinghouseUpload(c.Request().Context(), uploadID); err != nil {
		return clearinghouseStoreError(err, "Error fetching clearinghouse submission")
	}
	file, err := h.Uploads.GetClearinghouseUploadFileByName(c.Request().Context(), store.GetClearinghouseUploadFileByNameParams{
		ClearinghouseUploadUuid: uploadID,
		Filename:                filename,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ce.NewErrorResponse(http.StatusNotFound, "Error downloading clearinghouse submission file", "File not found")
		}
		return clearinghouseStoreError(err, "Error downloading clearinghouse submission file")
	}
	if h.S3 == nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error downloading clearinghouse submission file", "s3 not configured")
	}
	body, err := h.S3.Get(c.Request().Context(), file.Filepath)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error downloading clearinghouse submission file", err.Error())
	}
	defer body.Close()
	c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; filename=\""+filename+"\"")
	return c.Stream(http.StatusOK, "application/octet-stream", body)
}

type preparedClearinghouseFile struct {
	name   string
	header *multipart.FileHeader
	size   int64
}

func clearinghouseFormFile(c echo.Context) (*multipart.FileHeader, error) {
	form, err := c.MultipartForm()
	if err != nil {
		return nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading upload", "File is required")
	}
	files := form.File["file"]
	if len(files) != 1 {
		return nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading upload", "Exactly one file is required")
	}
	return files[0], nil
}

func clearinghouseFormFiles(c echo.Context) ([]*multipart.FileHeader, error) {
	form, err := c.MultipartForm()
	if err != nil {
		return nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading upload", "Files are required")
	}
	files := form.File["files"]
	if len(files) == 0 {
		return nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading upload", "At least one file is required")
	}
	if len(files) > maxClearinghouseFilesPerRequest {
		return nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading upload", "Too many files")
	}
	return files, nil
}

func prepareClearinghouseFiles(headers []*multipart.FileHeader) ([]preparedClearinghouseFile, error) {
	prepared := make([]preparedClearinghouseFile, 0, len(headers))
	seen := map[string]struct{}{}
	for _, header := range headers {
		name, err := cleanClearinghouseFilename(header.Filename)
		if err != nil {
			return nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading upload", err.Error())
		}
		if _, ok := seen[name]; ok {
			return nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading upload", "Duplicate filename "+name)
		}
		seen[name] = struct{}{}
		if header.Size <= 0 {
			return nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading upload", "Size must be greater than 0")
		}
		if header.Size > maxClearinghouseFileBytes {
			return nil, ce.NewErrorResponse(http.StatusRequestEntityTooLarge, "Error reading upload", "File exceeds maximum upload size")
		}
		prepared = append(prepared, preparedClearinghouseFile{name: name, header: header, size: header.Size})
	}
	return prepared, nil
}

func (h *ClearinghouseHandler) putClearinghouseFiles(c echo.Context, uploadID uuid.UUID, files []preparedClearinghouseFile) error {
	if h.S3 == nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error uploading clearinghouse submission", "s3 not configured")
	}
	for i := range files {
		file, err := files[i].header.Open()
		if err != nil {
			return ce.NewErrorResponse(http.StatusBadRequest, "Error opening upload", err.Error())
		}
		n, err := io.Copy(io.Discard, io.LimitReader(file, maxClearinghouseFileBytes+1))
		if err != nil {
			file.Close()
			return ce.NewErrorResponse(http.StatusBadRequest, "Error reading upload", err.Error())
		}
		if n > maxClearinghouseFileBytes {
			file.Close()
			return ce.NewErrorResponse(http.StatusRequestEntityTooLarge, "Error reading upload", "File exceeds maximum upload size")
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			file.Close()
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error uploading clearinghouse submission", err.Error())
		}
		err = h.S3.Put(c.Request().Context(), clearinghouseStorageKey(uploadID, files[i].name), file, n)
		file.Close()
		if err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error uploading clearinghouse submission", err.Error())
		}
		files[i].size = n
	}
	return nil
}

func (h *ClearinghouseHandler) insertClearinghouseFiles(c echo.Context, uploadID uuid.UUID, files []preparedClearinghouseFile) ([]store.ClearinghouseUploadFile, error) {
	stored := make([]store.ClearinghouseUploadFile, 0, len(files))
	for _, file := range files {
		row, err := h.Uploads.CreateClearinghouseUploadFile(c.Request().Context(), store.CreateClearinghouseUploadFileParams{
			Uuid:                    uuid.New(),
			ClearinghouseUploadUuid: uploadID,
			Filename:                file.name,
			Filepath:                clearinghouseStorageKey(uploadID, file.name),
		})
		if err != nil {
			return nil, clearinghouseStoreError(err, "Error creating clearinghouse submission file")
		}
		stored = append(stored, row)
	}
	return stored, nil
}

func (h *ClearinghouseHandler) visibleUpload(c echo.Context) (store.ClearinghouseUpload, error) {
	orgFilter, err := clearinghouseOrgFilter(c)
	if err != nil {
		return store.ClearinghouseUpload{}, err
	}
	uploadID, err := parseClearinghouseUUID(c.Param("uuid"))
	if err != nil {
		return store.ClearinghouseUpload{}, err
	}
	upload, err := h.Uploads.GetClearinghouseUpload(c.Request().Context(), uploadID)
	if err != nil {
		return store.ClearinghouseUpload{}, clearinghouseStoreError(err, "Error fetching clearinghouse submission")
	}
	if orgFilter != nil && upload.OrgID != *orgFilter {
		return store.ClearinghouseUpload{}, ce.NewErrorResponse(http.StatusNotFound, "Error fetching clearinghouse submission", "Submission not found")
	}
	return upload, nil
}

func clearinghouseOrgFilter(c echo.Context) (*string, error) {
	ctx := c.Request().Context()
	if config.FeatureAccessible(ctx, config.Get().Features.LightwellClearinghouseRead) {
		return nil, nil
	}
	if !config.FeatureAccessible(ctx, config.Get().Features.LightwellClearinghouse) {
		return nil, ce.NewErrorResponse(http.StatusBadRequest, "Cannot access clearinghouse submissions",
			"Neither the user nor account is allowed.")
	}
	_, orgID := handler.GetAccountIdOrgId(c)
	return &orgID, nil
}

type clearinghousePageParams struct {
	limit  int32
	offset int32
}

func clearinghousePage(c echo.Context) clearinghousePageParams {
	page := handler.ParsePagination(c)
	if page.Offset < 0 {
		page.Offset = 0
	}
	return clearinghousePageParams{limit: int32(page.Limit), offset: int32(page.Offset)} //nolint:gosec // pagination limit is capped at 200
}

func parseClearinghouseUUID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.UUID{}, ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", "Invalid submission UUID")
	}
	return id, nil
}

func cleanClearinghouseFilename(name string) (string, error) {
	if name == "" || path.Base(name) != name || name == "." || name == ".." || strings.ContainsAny(name, "/\\\"\r\n") {
		return "", errors.New("invalid filename")
	}
	if len(name) > 255 {
		return "", errors.New("filename is too long")
	}
	return name, nil
}

func clearinghouseStorageKey(uploadID uuid.UUID, filename string) string {
	return "uploads/" + uploadID.String() + "/" + filename
}

func clearinghouseStoreError(err error, title string) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ce.NewErrorResponse(http.StatusConflict, title, "A file with that name already exists")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ce.NewErrorResponse(http.StatusNotFound, title, "Submission not found")
	}
	return ce.NewErrorResponse(http.StatusInternalServerError, title, err.Error())
}

func clearinghouseSubmissionFromStore(upload store.ClearinghouseUpload, files []store.ClearinghouseUploadFile) api.ClearinghouseSubmission {
	outFiles := make([]api.ClearinghouseSubmissionFile, 0, len(files))
	for _, file := range files {
		outFiles = append(outFiles, api.ClearinghouseSubmissionFile{
			UUID:     file.Uuid.String(),
			Filename: file.Filename,
		})
	}
	var count *int
	if upload.VulnerabilityCount != nil {
		n := int(*upload.VulnerabilityCount)
		count = &n
	}
	return api.ClearinghouseSubmission{
		UUID:               upload.Uuid.String(),
		OrgID:              upload.OrgID,
		AccountID:          upload.AccountID,
		CreatedAt:          upload.CreatedAt,
		VulnerabilityCount: count,
		Status:             upload.Status,
		ErrorMessage:       upload.ErrorMessage,
		Files:              outFiles,
	}
}
