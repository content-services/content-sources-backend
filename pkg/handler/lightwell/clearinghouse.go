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

	uploadID := uuid.New()
	upload, err := h.Uploads.Create(c.Request().Context(), dao.CreateClearinghouseUploadParams{
		UUID:     uploadID,
		OrgID:    orgID,
		Username: username,
		Filename: prepared[0].name,
		Status:   api.ClearinghouseSubmissionStatusUploading,
	})
	if err != nil {
		return clearinghouseStoreError(err, "Error creating clearinghouse submission")
	}
	files, err := h.storeClearinghouseFiles(c, upload.UUID, prepared)
	if err != nil {
		_ = h.Uploads.Delete(c.Request().Context(), upload.UUID)
		return err
	}
	return c.JSON(http.StatusCreated, clearinghouseSubmissionFromStore(upload, files))
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

func (h *ClearinghouseHandler) storeClearinghouseFiles(c echo.Context, uploadID uuid.UUID, files []preparedClearinghouseFile) ([]dao.ClearinghouseUploadFile, error) {
	if h.S3 == nil {
		return nil, ce.NewErrorResponse(http.StatusInternalServerError, "Error uploading clearinghouse submission", "s3 not configured")
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
			return nil, err
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
		}
	}
	var uploaded []string
	stored, err := h.Uploads.CreateFiles(c.Request().Context(), params, func(rows []dao.ClearinghouseUploadFile) error {
		for i := range rows {
			if err := h.S3.Put(c.Request().Context(), rows[i].Filepath, opened[i].body, opened[i].size); err != nil {
				return err
			}
			uploaded = append(uploaded, rows[i].Filepath)
		}
		return nil
	})
	if err != nil {
		cleanupErr := h.removeClearinghouseObjects(c, uploaded)
		closeOpened()
		if cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
		if errors.Is(err, dao.ErrClearinghouseSubmissionClosed) {
			return nil, ce.NewErrorResponse(http.StatusConflict, "Cannot add files to clearinghouse submission", "Submission has been submitted and can no longer be changed")
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) || errors.Is(err, pgx.ErrNoRows) {
			return nil, clearinghouseStoreError(err, "Error creating clearinghouse submission file")
		}
		return nil, ce.NewErrorResponse(http.StatusInternalServerError, "Error uploading clearinghouse submission", err.Error())
	}
	closeOpened()
	return stored, nil
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
			UUID:     file.UUID.String(),
			Filename: file.Filename,
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
		ProcessedAt:        upload.ProcessedAt,
		ProcessedBy:        upload.ProcessedBy,
		VulnerabilityCount: count,
		Status:             upload.Status,
		ErrorMessage:       upload.ErrorMessage,
		Files:              outFiles,
	}
}
