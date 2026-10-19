package lightwell

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/s3_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
)

const (
	maxClearinghouseFilesPerRequest = 20
	maxClearinghouseFileBytes       = 50 * 1024 * 1024
)

type ClearinghouseHandler struct {
	Uploads dao.ClearinghouseUploadDao
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

func RegisterClearinghouseRoutes(engine *echo.Group, uploads dao.ClearinghouseUploadDao, s3Client s3_client.S3Client) {
	h := ClearinghouseHandler{Uploads: uploads, S3: s3Client}
	clearinghouse := checkLightwellClearinghouseAccessible()
	addLightwellRoute(engine, http.MethodPost, "/clearinghouse/submissions/create", h.createSubmission, rbac.RbacVerbRead, clearinghouse)
	addLightwellRoute(engine, http.MethodPost, "/clearinghouse/submissions/:uuid/files", h.addSubmissionFiles, rbac.RbacVerbRead, clearinghouse)
	addLightwellRoute(engine, http.MethodDelete, "/clearinghouse/submissions/:uuid/files/:filename", h.deleteSubmissionFile, rbac.RbacVerbRead, clearinghouse)
	addLightwellRoute(engine, http.MethodGet, "/clearinghouse/submissions", h.listSubmissions, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodDelete, "/clearinghouse/submissions/:uuid", h.deleteSubmission, rbac.RbacVerbRead, clearinghouse)
}

// createSubmission godoc
// @Summary      Create a clearinghouse submission
// @ID           createClearinghouseSubmission
// @Description  Upload one file, up to 50 MiB, as a new clearinghouse submission. The original file name is stored and returned as filename. Requires the Lightwell clearinghouse feature.
// @Tags         lightwell
// @Accept       multipart/form-data
// @Produce      json
// @Param        file formData file true "File to store with the submission. Maximum size is 50 MiB."
// @Success      201 {object} api.ClearinghouseSubmission
// @Failure      400 {object} ce.ErrorResponse
// @Failure      409 {object} ce.ErrorResponse
// @Failure      413 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /clearinghouse/submissions/create [post]
func (h *ClearinghouseHandler) createSubmission(c echo.Context) error {
	_, orgID := handler.GetAccountIdOrgId(c)
	if orgID == "" {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot upload a clearinghouse submission", "Org ID is required")
	}
	username, err := clearinghouseUsername(c)
	if err != nil {
		return err
	}
	header, err := clearinghouseFormFile(c)
	if err != nil {
		return err
	}
	prepared, err := prepareClearinghouseFiles([]*multipart.FileHeader{header})
	if err != nil {
		return err
	}

	upload, files, err := h.storeNewClearinghouseSubmission(c, dao.CreateClearinghouseUploadParams{
		UUID:     uuid.New(),
		OrgID:    orgID,
		Username: username,
		Filename: prepared[0].name,
		Status:   api.ClearinghouseSubmissionStatusPending,
	}, prepared)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, clearinghouseSubmissionFromStore(upload, files))
}

// addSubmissionFiles godoc
// @Summary      Add files to a clearinghouse submission
// @ID           addClearinghouseSubmissionFiles
// @Description  Upload more files onto an existing clearinghouse submission while it is pending. Anyone in the same org can add files. Requires the Lightwell clearinghouse feature.
// @Tags         lightwell
// @Accept       multipart/form-data
// @Produce      json
// @Param        uuid  path   string true "Clearinghouse submission UUID"
// @Param        files formData file true "Files to add. Repeat the field to upload multiple files."
// @Success      200 {object} api.ClearinghouseSubmission
// @Failure      400 {object} ce.ErrorResponse
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
	upload, err := h.Uploads.Get(c.Request().Context(), uploadID)
	if err != nil {
		return clearinghouseStoreError(err, "Error fetching clearinghouse submission")
	}
	_, orgID := handler.GetAccountIdOrgId(c)
	if upload.OrgID != orgID {
		return ce.NewErrorResponse(http.StatusNotFound, "Error fetching clearinghouse submission", "Submission not found")
	}
	if upload.Status != api.ClearinghouseSubmissionStatusPending {
		return ce.NewErrorResponse(http.StatusConflict, "Cannot add files to clearinghouse submission", "Submission has been submitted and can no longer be changed")
	}

	headers, err := clearinghouseFormFiles(c)
	if err != nil {
		return err
	}
	prepared, err := prepareClearinghouseFiles(headers)
	if err != nil {
		return err
	}
	if _, err = h.storeClearinghouseFiles(c, upload.UUID, prepared); err != nil {
		return err
	}
	files, err := h.Uploads.ListFiles(c.Request().Context(), upload.UUID)
	if err != nil {
		return clearinghouseStoreError(err, "Error listing clearinghouse submission files")
	}
	return c.JSON(http.StatusOK, clearinghouseSubmissionFromStore(upload, files))
}

// deleteSubmissionFile godoc
// @Summary      Delete a clearinghouse submission file
// @ID           deleteClearinghouseSubmissionFile
// @Description  Delete one file from a clearinghouse submission while it is pending. Deleting the file that was uploaded with the submission removes the submission and every file stored with it. Only the user who created the submission can delete files. Requires the Lightwell clearinghouse feature.
// @Tags         lightwell
// @Param        uuid     path string true "Clearinghouse submission UUID"
// @Param        filename path string true "File name"
// @Success      204 "Deleted"
// @Failure      400 {object} ce.ErrorResponse
// @Failure      403 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      409 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /clearinghouse/submissions/{uuid}/files/{filename} [delete]
func (h *ClearinghouseHandler) deleteSubmissionFile(c echo.Context) error {
	filename, err := cleanClearinghouseFilename(c.Param("filename"))
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error deleting clearinghouse submission file", err.Error())
	}
	upload, err := h.authorUploadingSubmission(c, "Cannot delete clearinghouse submission file", "Only the user who created the submission can delete files")
	if err != nil {
		return err
	}
	if filename == upload.Filename {
		err = h.Uploads.DeleteIfUploading(c.Request().Context(), upload.UUID, func(files []dao.ClearinghouseUploadFile) error {
			for _, file := range files {
				if err := h.deleteClearinghouseObject(c, file.Filepath); err != nil {
					return err
				}
			}
			return nil
		})
	} else {
		err = h.Uploads.DeleteFileIfUploading(c.Request().Context(), upload.UUID, filename, func(file dao.ClearinghouseUploadFile) error {
			return h.deleteClearinghouseObject(c, file.Filepath)
		})
	}
	if err != nil {
		return clearinghouseDeleteError(err, "Error deleting clearinghouse submission file", "File not found")
	}
	return c.NoContent(http.StatusNoContent)
}

// deleteSubmission godoc
// @Summary      Delete a clearinghouse submission
// @ID           deleteClearinghouseSubmission
// @Description  Delete a clearinghouse submission and its files while it is pending. Only the user who created the submission can delete it. Requires the Lightwell clearinghouse feature.
// @Tags         lightwell
// @Param        uuid path string true "Clearinghouse submission UUID"
// @Success      204 "Deleted"
// @Failure      400 {object} ce.ErrorResponse
// @Failure      403 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      409 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /clearinghouse/submissions/{uuid} [delete]
func (h *ClearinghouseHandler) deleteSubmission(c echo.Context) error {
	upload, err := h.authorUploadingSubmission(c, "Cannot delete clearinghouse submission", "Only the user who created the submission can delete it")
	if err != nil {
		return err
	}
	err = h.Uploads.DeleteIfUploading(c.Request().Context(), upload.UUID, func(files []dao.ClearinghouseUploadFile) error {
		for _, file := range files {
			if err := h.deleteClearinghouseObject(c, file.Filepath); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return clearinghouseDeleteError(err, "Error deleting clearinghouse submission", "Submission not found")
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *ClearinghouseHandler) authorUploadingSubmission(c echo.Context, title, forbidden string) (dao.ClearinghouseUpload, error) {
	uploadID, err := parseClearinghouseUUID(c.Param("uuid"))
	if err != nil {
		return dao.ClearinghouseUpload{}, err
	}
	upload, err := h.Uploads.Get(c.Request().Context(), uploadID)
	if err != nil {
		return dao.ClearinghouseUpload{}, clearinghouseStoreError(err, title)
	}
	_, orgID := handler.GetAccountIdOrgId(c)
	if upload.OrgID != orgID {
		return dao.ClearinghouseUpload{}, ce.NewErrorResponse(http.StatusNotFound, title, "Submission not found")
	}
	username, err := clearinghouseUsername(c)
	if err != nil {
		return dao.ClearinghouseUpload{}, err
	}
	if upload.Username != username {
		return dao.ClearinghouseUpload{}, ce.NewErrorResponse(http.StatusForbidden, title, forbidden)
	}
	if upload.Status != api.ClearinghouseSubmissionStatusPending {
		return dao.ClearinghouseUpload{}, ce.NewErrorResponse(http.StatusConflict, title, "Submission has been submitted and can no longer be changed")
	}
	return upload, nil
}

func (h *ClearinghouseHandler) deleteClearinghouseObject(c echo.Context, storageKey string) error {
	if h.S3 == nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error deleting clearinghouse submission", "s3 not configured")
	}
	if err := h.S3.Delete(c.Request().Context(), storageKey); err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error deleting clearinghouse submission", err.Error())
	}
	return nil
}

func clearinghouseDeleteError(err error, title, missing string) error {
	var response ce.ErrorResponse
	if errors.As(err, &response) {
		return response
	}
	if errors.Is(err, dao.ErrClearinghouseSubmissionClosed) {
		return ce.NewErrorResponse(http.StatusConflict, title, "Submission has been submitted and can no longer be changed")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ce.NewErrorResponse(http.StatusNotFound, title, missing)
	}
	return clearinghouseStoreError(err, title)
}

// listSubmissions godoc
// @Summary      List clearinghouse submissions
// @ID           listClearinghouseSubmissions
// @Description  Customers with Lightwell clearinghouse see every submission in their org, including submissions that are still pending. Callers with Lightwell clearinghouse read see every submission. File contents are not included.
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
	uploads, err := h.Uploads.List(c.Request().Context(), dao.ListClearinghouseUploadsParams{
		OrgID:  orgFilter,
		Limit:  page.limit,
		Offset: page.offset,
	})
	if err != nil {
		return clearinghouseStoreError(err, "Error listing clearinghouse submissions")
	}
	total, err := h.Uploads.Count(c.Request().Context(), orgFilter)
	if err != nil {
		return clearinghouseStoreError(err, "Error listing clearinghouse submissions")
	}

	filesByUpload := map[uuid.UUID][]dao.ClearinghouseUploadFile{}
	if len(uploads) > 0 {
		ids := make([]uuid.UUID, len(uploads))
		for i, upload := range uploads {
			ids[i] = upload.UUID
		}
		files, filesErr := h.Uploads.ListFilesByUploads(c.Request().Context(), ids)
		if filesErr != nil {
			return clearinghouseStoreError(filesErr, "Error listing clearinghouse submission files")
		}
		for _, file := range files {
			filesByUpload[file.UploadUUID] = append(filesByUpload[file.UploadUUID], file)
		}
	}

	response := api.ClearinghouseSubmissionCollectionResponse{Data: make([]api.ClearinghouseSubmission, 0, len(uploads))}
	for _, upload := range uploads {
		response.Data = append(response.Data, clearinghouseSubmissionFromStore(upload, filesByUpload[upload.UUID]))
	}
	return c.JSON(http.StatusOK, handler.SetCollectionResponseMetadata(&response, c, total))
}

type preparedClearinghouseFile struct {
	name   string
	header *multipart.FileHeader
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
		prepared = append(prepared, preparedClearinghouseFile{name: name, header: header})
	}
	return prepared, nil
}

type openedClearinghouseFile struct {
	body multipart.File
	size int64
}

func (h *ClearinghouseHandler) storeNewClearinghouseSubmission(c echo.Context, upload dao.CreateClearinghouseUploadParams, files []preparedClearinghouseFile) (dao.ClearinghouseUpload, []dao.ClearinghouseUploadFile, error) {
	opened, params, closeOpened, err := h.prepareClearinghouseStore(c, upload.UUID, files)
	if err != nil {
		return dao.ClearinghouseUpload{}, nil, err
	}
	defer closeOpened()

	var uploaded []string
	storedUpload, storedFiles, err := h.Uploads.CreateWithFiles(c.Request().Context(), upload, params, func(rows []dao.ClearinghouseUploadFile) error {
		return h.putClearinghouseObjects(c, opened, rows, &uploaded)
	})
	if err != nil {
		return dao.ClearinghouseUpload{}, nil, h.failClearinghouseStore(c, uploaded, err)
	}
	return storedUpload, storedFiles, nil
}

func (h *ClearinghouseHandler) storeClearinghouseFiles(c echo.Context, uploadID uuid.UUID, files []preparedClearinghouseFile) ([]dao.ClearinghouseUploadFile, error) {
	opened, params, closeOpened, err := h.prepareClearinghouseStore(c, uploadID, files)
	if err != nil {
		return nil, err
	}
	defer closeOpened()

	var uploaded []string
	stored, err := h.Uploads.CreateFiles(c.Request().Context(), params, func(rows []dao.ClearinghouseUploadFile) error {
		return h.putClearinghouseObjects(c, opened, rows, &uploaded)
	})
	if err != nil {
		return nil, h.failClearinghouseStore(c, uploaded, err)
	}
	return stored, nil
}

func (h *ClearinghouseHandler) prepareClearinghouseStore(c echo.Context, uploadID uuid.UUID, files []preparedClearinghouseFile) ([]openedClearinghouseFile, []dao.CreateClearinghouseUploadFileParams, func(), error) {
	username, err := clearinghouseUsername(c)
	if err != nil {
		return nil, nil, nil, err
	}
	if h.S3 == nil {
		return nil, nil, nil, ce.NewErrorResponse(http.StatusInternalServerError, "Error uploading clearinghouse submission", "s3 not configured")
	}
	opened := make([]openedClearinghouseFile, 0, len(files))
	closeOpened := func() {
		for i := range opened {
			opened[i].body.Close()
		}
	}
	for i := range files {
		body, size, err := openClearinghouseFile(files[i])
		if err != nil {
			closeOpened()
			return nil, nil, nil, err
		}
		opened = append(opened, openedClearinghouseFile{body: body, size: size})
	}
	params := make([]dao.CreateClearinghouseUploadFileParams, len(files))
	for i := range files {
		params[i] = dao.CreateClearinghouseUploadFileParams{
			UUID:       uuid.New(),
			UploadUUID: uploadID,
			Filename:   files[i].name,
			Filepath:   clearinghouseStorageKey(uploadID, files[i].name),
			Username:   username,
		}
	}
	return opened, params, closeOpened, nil
}

func (h *ClearinghouseHandler) putClearinghouseObjects(c echo.Context, opened []openedClearinghouseFile, rows []dao.ClearinghouseUploadFile, uploaded *[]string) error {
	for i := range rows {
		if err := h.S3.Put(c.Request().Context(), rows[i].Filepath, opened[i].body, opened[i].size); err != nil {
			return err
		}
		*uploaded = append(*uploaded, rows[i].Filepath)
	}
	return nil
}

func (h *ClearinghouseHandler) failClearinghouseStore(c echo.Context, uploaded []string, err error) error {
	cleanupErr := h.removeClearinghouseObjects(c, uploaded)
	if cleanupErr != nil {
		err = errors.Join(err, cleanupErr)
	}
	if errors.Is(err, dao.ErrClearinghouseSubmissionClosed) {
		return ce.NewErrorResponse(http.StatusConflict, "Cannot add files to clearinghouse submission", "Submission has been submitted and can no longer be changed")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) || errors.Is(err, pgx.ErrNoRows) {
		return clearinghouseStoreError(err, "Error creating clearinghouse submission file")
	}
	return ce.NewErrorResponse(http.StatusInternalServerError, "Error uploading clearinghouse submission", err.Error())
}

func (h *ClearinghouseHandler) removeClearinghouseObjects(c echo.Context, keys []string) error {
	var errs []error
	for _, key := range keys {
		if err := h.S3.Delete(c.Request().Context(), key); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func openClearinghouseFile(file preparedClearinghouseFile) (multipart.File, int64, error) {
	body, err := file.header.Open()
	if err != nil {
		return nil, 0, ce.NewErrorResponse(http.StatusBadRequest, "Error opening upload", err.Error())
	}
	n, err := io.Copy(io.Discard, io.LimitReader(body, maxClearinghouseFileBytes+1))
	if err != nil {
		body.Close()
		return nil, 0, ce.NewErrorResponse(http.StatusBadRequest, "Error reading upload", err.Error())
	}
	if n > maxClearinghouseFileBytes {
		body.Close()
		return nil, 0, ce.NewErrorResponse(http.StatusRequestEntityTooLarge, "Error reading upload", "File exceeds maximum upload size")
	}
	if _, err = body.Seek(0, io.SeekStart); err != nil {
		body.Close()
		return nil, 0, ce.NewErrorResponse(http.StatusInternalServerError, "Error uploading clearinghouse submission", err.Error())
	}
	return body, n, nil
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

func clearinghouseUsername(c echo.Context) (string, error) {
	id := identity.GetIdentity(c.Request().Context())
	if id.Identity.User != nil && id.Identity.User.Username != "" {
		return id.Identity.User.Username, nil
	}
	if id.Identity.ServiceAccount != nil && id.Identity.ServiceAccount.Username != "" {
		return id.Identity.ServiceAccount.Username, nil
	}
	return "", ce.NewErrorResponse(http.StatusBadRequest, "Missing user identity", "Username is required")
}

func clearinghouseSubmissionFromStore(upload dao.ClearinghouseUpload, files []dao.ClearinghouseUploadFile) api.ClearinghouseSubmission {
	outFiles := make([]api.ClearinghouseSubmissionFile, 0, len(files))
	for _, file := range files {
		outFiles = append(outFiles, api.ClearinghouseSubmissionFile{
			UUID:      file.UUID.String(),
			Filename:  file.Filename,
			Username:  file.Username,
			CreatedAt: file.CreatedAt,
		})
	}
	var count *int
	if upload.VulnerabilityCount != nil {
		n := int(*upload.VulnerabilityCount)
		count = &n
	}
	return api.ClearinghouseSubmission{
		UUID:               upload.UUID.String(),
		OrgID:              upload.OrgID,
		Username:           upload.Username,
		Filename:           upload.Filename,
		CreatedAt:          upload.CreatedAt,
		SubmittedAt:        upload.SubmittedAt,
		SubmittedBy:        upload.SubmittedBy,
		ProcessedAt:        upload.ProcessedAt,
		ProcessedBy:        upload.ProcessedBy,
		VulnerabilityCount: count,
		Status:             upload.Status,
		ErrorMessage:       upload.ErrorMessage,
		Files:              outFiles,
	}
}
