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
	addLightwellRoute(engine, http.MethodPost, "/clearinghouse/submissions/:uuid/submit", h.submitSubmission, rbac.RbacVerbRead, clearinghouse)
	addLightwellRoute(engine, http.MethodGet, "/clearinghouse/submissions", h.listSubmissions, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodGet, "/clearinghouse/submissions/:uuid", h.getSubmission, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodDelete, "/clearinghouse/submissions/:uuid", h.deleteSubmission, rbac.RbacVerbRead, clearinghouse)
	addLightwellRoute(engine, http.MethodGet, "/clearinghouse/submissions/:uuid/submission", h.downloadSubmission, rbac.RbacVerbRead)
	addLightwellRoute(engine, http.MethodGet, "/clearinghouse/submissions/:uuid/files/:filename", h.downloadSubmissionFile, rbac.RbacVerbRead)
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

// addSubmissionFiles godoc
// @Summary      Add files to a clearinghouse submission
// @ID           addClearinghouseSubmissionFiles
// @Description  Upload more files onto an existing clearinghouse submission while it is uploading. Only the user who created the submission can add files. Requires the Lightwell clearinghouse feature.
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
	upload, err := h.Uploads.Get(c.Request().Context(), uploadID)
	if err != nil {
		return clearinghouseStoreError(err, "Error fetching clearinghouse submission")
	}
	_, orgID := handler.GetAccountIdOrgId(c)
	if upload.OrgID != orgID {
		return ce.NewErrorResponse(http.StatusNotFound, "Error fetching clearinghouse submission", "Submission not found")
	}
	username, err := clearinghouseUsername(c)
	if err != nil {
		return err
	}
	if upload.Username != username {
		return ce.NewErrorResponse(http.StatusForbidden, "Cannot add files to clearinghouse submission", "Only the user who created the submission can add files")
	}
	if upload.Status != api.ClearinghouseSubmissionStatusUploading {
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
// @Description  Delete one file from a clearinghouse submission while it is uploading. Deleting the file that was uploaded with the submission removes the submission and every file stored with it. Only the user who created the submission can delete files. Requires the Lightwell clearinghouse feature.
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
// @Description  Delete a clearinghouse submission and its files while it is uploading. Only the user who created the submission can delete it. Requires the Lightwell clearinghouse feature.
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
	if upload.Status != api.ClearinghouseSubmissionStatusUploading {
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

// submitSubmission godoc
// @Summary      Submit a clearinghouse submission
// @ID           submitClearinghouseSubmission
// @Description  Conclude a clearinghouse submission. Sets its status from uploading to submitted. Only the user who created the submission can submit it. Requires the Lightwell clearinghouse feature.
// @Tags         lightwell
// @Produce      json
// @Param        uuid path string true "Clearinghouse submission UUID"
// @Success      200 {object} api.ClearinghouseSubmission
// @Failure      400 {object} ce.ErrorResponse
// @Failure      403 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      409 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /clearinghouse/submissions/{uuid}/submit [post]
func (h *ClearinghouseHandler) submitSubmission(c echo.Context) error {
	uploadID, err := parseClearinghouseUUID(c.Param("uuid"))
	if err != nil {
		return err
	}
	upload, err := h.Uploads.Get(c.Request().Context(), uploadID)
	if err != nil {
		return clearinghouseStoreError(err, "Error submitting clearinghouse submission")
	}
	_, orgID := handler.GetAccountIdOrgId(c)
	if upload.OrgID != orgID {
		return ce.NewErrorResponse(http.StatusNotFound, "Error submitting clearinghouse submission", "Submission not found")
	}
	username, err := clearinghouseUsername(c)
	if err != nil {
		return err
	}
	if upload.Username != username {
		return ce.NewErrorResponse(http.StatusForbidden, "Cannot submit clearinghouse submission", "Only the user who created the submission can submit it")
	}
	updated, err := h.Uploads.Submit(c.Request().Context(), upload.UUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ce.NewErrorResponse(http.StatusConflict, "Cannot submit clearinghouse submission", "Submission has been submitted and can no longer be changed")
		}
		return clearinghouseStoreError(err, "Error submitting clearinghouse submission")
	}
	files, err := h.Uploads.ListFiles(c.Request().Context(), upload.UUID)
	if err != nil {
		return clearinghouseStoreError(err, "Error listing clearinghouse submission files")
	}
	return c.JSON(http.StatusOK, clearinghouseSubmissionFromStore(updated, files))
}

// listSubmissions godoc
// @Summary      List clearinghouse submissions
// @ID           listClearinghouseSubmissions
// @Description  Customers with Lightwell clearinghouse see every submission in their org, including submissions that are still uploading. Callers with Lightwell clearinghouse read see every submission. File contents are not included.
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
	files, err := h.Uploads.ListFiles(c.Request().Context(), upload.UUID)
	if err != nil {
		return clearinghouseStoreError(err, "Error listing clearinghouse submission files")
	}
	return c.JSON(http.StatusOK, clearinghouseSubmissionFromStore(upload, files))
}

// downloadSubmission godoc
// @Summary      Download a clearinghouse submission
// @ID           downloadClearinghouseSubmission
// @Description  Download the file that was uploaded when the clearinghouse submission was created. The author can download it while it is uploading or submitted. Callers with the Lightwell clearinghouse read feature can download it once it is submitted.
// @Tags         lightwell
// @Produce      application/octet-stream
// @Param        uuid path string true "Clearinghouse submission UUID"
// @Success      200 {file} file
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      409 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /clearinghouse/submissions/{uuid}/submission [get]
func (h *ClearinghouseHandler) downloadSubmission(c echo.Context) error {
	uploadID, err := parseClearinghouseUUID(c.Param("uuid"))
	if err != nil {
		return err
	}
	upload, err := h.Uploads.Get(c.Request().Context(), uploadID)
	if err != nil {
		return clearinghouseStoreError(err, "Error fetching clearinghouse submission")
	}
	return h.downloadClearinghouseFile(c, upload.Filename)
}

// downloadSubmissionFile godoc
// @Summary      Download a clearinghouse submission file
// @ID           downloadClearinghouseSubmissionFile
// @Description  Download a file stored with a clearinghouse submission. The author can download it while it is uploading or submitted. Callers with the Lightwell clearinghouse read feature can download it once it is submitted.
// @Tags         lightwell
// @Produce      application/octet-stream
// @Param        uuid     path string true "Clearinghouse submission UUID"
// @Param        filename path string true "File name"
// @Success      200 {file} file
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      409 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /clearinghouse/submissions/{uuid}/files/{filename} [get]
func (h *ClearinghouseHandler) downloadSubmissionFile(c echo.Context) error {
	filename, err := cleanClearinghouseFilename(c.Param("filename"))
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error downloading clearinghouse submission file", err.Error())
	}
	return h.downloadClearinghouseFile(c, filename)
}

func (h *ClearinghouseHandler) downloadClearinghouseFile(c echo.Context, filename string) error {
	uploadID, err := parseClearinghouseUUID(c.Param("uuid"))
	if err != nil {
		return err
	}
	upload, err := h.Uploads.Get(c.Request().Context(), uploadID)
	if err != nil {
		return clearinghouseStoreError(err, "Error fetching clearinghouse submission")
	}
	if err := authorizeClearinghouseDownload(c, upload); err != nil {
		return err
	}
	file, err := h.Uploads.GetFileByName(c.Request().Context(), uploadID, filename)
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

func authorizeClearinghouseDownload(c echo.Context, upload dao.ClearinghouseUpload) error {
	readErr := handler.CheckLightwellClearinghouseReadAccessible(c.Request().Context())
	authorErr := handler.CheckLightwellClearinghouseAccessible(c.Request().Context())
	if readErr != nil && authorErr != nil {
		return readErr
	}
	username, err := clearinghouseUsername(c)
	if err != nil {
		return err
	}
	_, orgID := handler.GetAccountIdOrgId(c)
	isAuthor := authorErr == nil && upload.Username == username && upload.OrgID == orgID
	hasRead := readErr == nil
	allowed := false
	switch upload.Status {
	case api.ClearinghouseSubmissionStatusUploading:
		allowed = isAuthor
	case api.ClearinghouseSubmissionStatusSubmitted:
		allowed = isAuthor || hasRead
	case api.ClearinghouseSubmissionStatusProcessed:
		allowed = hasRead
	}
	if allowed {
		return nil
	}
	if isAuthor || hasRead {
		return ce.NewErrorResponse(http.StatusConflict, "Cannot download clearinghouse submission", clearinghouseDownloadConflict(upload.Status))
	}
	if readErr != nil {
		return readErr
	}
	return ce.NewErrorResponse(http.StatusNotFound, "Error fetching clearinghouse submission", "Submission not found")
}

func clearinghouseDownloadConflict(status string) string {
	if status == api.ClearinghouseSubmissionStatusUploading {
		return "Submission must be submitted before it can be downloaded"
	}
	return "Submission cannot be downloaded"
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

func (h *ClearinghouseHandler) visibleUpload(c echo.Context) (dao.ClearinghouseUpload, error) {
	orgFilter, err := clearinghouseOrgFilter(c)
	if err != nil {
		return dao.ClearinghouseUpload{}, err
	}
	uploadID, err := parseClearinghouseUUID(c.Param("uuid"))
	if err != nil {
		return dao.ClearinghouseUpload{}, err
	}
	upload, err := h.Uploads.Get(c.Request().Context(), uploadID)
	if err != nil {
		return dao.ClearinghouseUpload{}, clearinghouseStoreError(err, "Error fetching clearinghouse submission")
	}
	if orgFilter != nil && upload.OrgID != *orgFilter {
		return dao.ClearinghouseUpload{}, ce.NewErrorResponse(http.StatusNotFound, "Error fetching clearinghouse submission", "Submission not found")
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
