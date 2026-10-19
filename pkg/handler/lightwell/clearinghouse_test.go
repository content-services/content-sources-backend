package lightwell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/s3_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestClearinghouseSubmissions(t *testing.T) {
	oldClearinghouse := config.Get().Features.LightwellClearinghouse
	oldRead := config.Get().Features.LightwellClearinghouseRead
	t.Cleanup(func() {
		config.Get().Features.LightwellClearinghouse = oldClearinghouse
		config.Get().Features.LightwellClearinghouseRead = oldRead
	})

	uploads := &memClearinghouseStore{}
	s3Client := s3_client.NewMockS3Client(t)
	s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil)
	s3Client.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(nil)

	config.Get().Features.LightwellClearinghouse = config.Feature{Enabled: false}
	config.Get().Features.LightwellClearinghouseRead = config.Feature{Enabled: false}

	code, body := serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
	}))
	assert.Equal(t, http.StatusBadRequest, code, string(body))

	config.Get().Features.LightwellClearinghouse = config.Feature{Enabled: true}
	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		strings.Repeat("a", 300) + ".json": "{}",
	}))
	assert.Equal(t, http.StatusBadRequest, code, string(body))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"one.json": "{}",
		"two.json": "{}",
	}))
	assert.Equal(t, http.StatusBadRequest, code, string(body))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
	}))
	require.Equal(t, http.StatusCreated, code, string(body))
	var created api.ClearinghouseSubmission
	require.NoError(t, json.Unmarshal(body, &created))
	assert.Equal(t, api.ClearinghouseSubmissionStatusUploading, created.Status)
	assert.Equal(t, test_handler.MockOrgId, created.OrgID)
	assert.Equal(t, "user", created.Username)
	assert.Equal(t, "vulnerabilities.json", created.Filename)
	require.Len(t, created.Files, 1)
	assert.Equal(t, "vulnerabilities.json", created.Files[0].Filename)

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"reproducer.py": "print('hi')",
	}))
	require.Equal(t, http.StatusOK, code, string(body))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"notes.txt": "more",
	}))
	require.Equal(t, http.StatusOK, code, string(body))
	var updated api.ClearinghouseSubmission
	require.NoError(t, json.Unmarshal(body, &updated))
	assert.Len(t, updated.Files, 3)
	assert.Equal(t, "user", updated.Username)

	otherUser := test_handler.MockIdentity
	user := *test_handler.MockIdentity.Identity.User
	user.Username = "other"
	otherUser.Identity.User = &user
	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequestAs(t, http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"other.txt": "nope",
	}, otherUser))
	assert.Equal(t, http.StatusForbidden, code, string(body))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"notes.txt": "again",
	}))
	assert.Equal(t, http.StatusConflict, code, string(body))
	s3Client.AssertNumberOfCalls(t, "Put", 3)

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"notes.txt": "again",
		"extra.txt": "new",
	}))
	assert.Equal(t, http.StatusConflict, code, string(body))
	s3Client.AssertNumberOfCalls(t, "Put", 3)
	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseRequest(t, http.MethodGet, "/clearinghouse/submissions/"+created.UUID))
	require.Equal(t, http.StatusOK, code, string(body))
	var fetched api.ClearinghouseSubmission
	require.NoError(t, json.Unmarshal(body, &fetched))
	assert.Len(t, fetched.Files, 3)

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"extra.txt": "new",
	}))
	require.Equal(t, http.StatusOK, code, string(body))
	s3Client.AssertNumberOfCalls(t, "Put", 4)

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequestAs(t, http.MethodDelete, "/clearinghouse/submissions/"+created.UUID+"/files/notes.txt", "files", map[string]string{}, otherUser))
	assert.Equal(t, http.StatusForbidden, code, string(body))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseRequest(t, http.MethodDelete, "/clearinghouse/submissions/"+created.UUID+"/files/notes.txt"))
	assert.Equal(t, http.StatusNoContent, code, string(body))
	s3Client.AssertNumberOfCalls(t, "Delete", 1)
	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseRequest(t, http.MethodGet, "/clearinghouse/submissions/"+created.UUID))
	require.Equal(t, http.StatusOK, code, string(body))
	require.NoError(t, json.Unmarshal(body, &fetched))
	assert.Len(t, fetched.Files, 3)

	otherID := uuid.New()
	uploads.uploads = append(uploads.uploads, dao.ClearinghouseUpload{
		UUID:     otherID,
		OrgID:    "other-org",
		Username: "someone",
		Status:   api.ClearinghouseSubmissionStatusSubmitted,
	})

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseRequest(t, http.MethodGet, "/clearinghouse/submissions"))
	require.Equal(t, http.StatusOK, code, string(body))
	var listed api.ClearinghouseSubmissionCollectionResponse
	require.NoError(t, json.Unmarshal(body, &listed))
	require.Len(t, listed.Data, 1)
	assert.Equal(t, created.UUID, listed.Data[0].UUID)
	assert.Equal(t, "user", listed.Data[0].Username)
	assert.Len(t, listed.Data[0].Files, 3)
}

func TestClearinghouseCreatePutFailure(t *testing.T) {
	oldClearinghouse := config.Get().Features.LightwellClearinghouse
	t.Cleanup(func() {
		config.Get().Features.LightwellClearinghouse = oldClearinghouse
	})
	config.Get().Features.LightwellClearinghouse = config.Feature{Enabled: true}

	uploads := &memClearinghouseStore{}
	s3Client := s3_client.NewMockS3Client(t)
	s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(errors.New("put failed"))

	code, body := serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
	}))
	assert.Equal(t, http.StatusInternalServerError, code, string(body))
	assert.Contains(t, string(body), "put failed")
	assert.Empty(t, uploads.uploads)
	assert.Empty(t, uploads.files)
}

func TestClearinghouseAddFilesPutFailure(t *testing.T) {
	oldClearinghouse := config.Get().Features.LightwellClearinghouse
	t.Cleanup(func() {
		config.Get().Features.LightwellClearinghouse = oldClearinghouse
	})
	config.Get().Features.LightwellClearinghouse = config.Feature{Enabled: true}

	uploads := &memClearinghouseStore{}
	s3Client := s3_client.NewMockS3Client(t)
	s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil).Once()
	s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil).Once()
	s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(errors.New("put failed")).Once()
	s3Client.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(nil).Once()

	code, body := serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
	}))
	require.Equal(t, http.StatusCreated, code, string(body))
	var created api.ClearinghouseSubmission
	require.NoError(t, json.Unmarshal(body, &created))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"notes.txt": "more",
		"extra.txt": "new",
	}))
	assert.Equal(t, http.StatusInternalServerError, code, string(body))
	assert.Contains(t, string(body), "put failed")
	require.Len(t, uploads.files, 1)
	assert.Equal(t, "vulnerabilities.json", uploads.files[0].Filename)
	assert.Len(t, uploads.uploads, 1)

	uploads = &memClearinghouseStore{}
	s3Client = s3_client.NewMockS3Client(t)
	s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil).Once()
	s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil).Once()
	s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(errors.New("put failed")).Once()
	s3Client.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(errors.New("delete failed")).Once()

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
	}))
	require.Equal(t, http.StatusCreated, code, string(body))
	require.NoError(t, json.Unmarshal(body, &created))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"notes.txt": "more",
		"extra.txt": "new",
	}))
	assert.Equal(t, http.StatusInternalServerError, code, string(body))
	assert.Contains(t, string(body), "delete failed")
	require.Len(t, uploads.files, 1)
	assert.Equal(t, "vulnerabilities.json", uploads.files[0].Filename)
}

func TestClearinghouseDeleteSubmissionJSON(t *testing.T) {
	oldClearinghouse := config.Get().Features.LightwellClearinghouse
	t.Cleanup(func() {
		config.Get().Features.LightwellClearinghouse = oldClearinghouse
	})
	config.Get().Features.LightwellClearinghouse = config.Feature{Enabled: true}

	uploads := &memClearinghouseStore{}
	s3Client := s3_client.NewMockS3Client(t)
	s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil)
	s3Client.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(nil)

	code, body := serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
	}))
	require.Equal(t, http.StatusCreated, code, string(body))
	var created api.ClearinghouseSubmission
	require.NoError(t, json.Unmarshal(body, &created))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"reproducer.py": "print('hi')",
	}))
	require.Equal(t, http.StatusOK, code, string(body))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseRequest(t, http.MethodDelete, "/clearinghouse/submissions/"+created.UUID+"/files/vulnerabilities.json"))
	assert.Equal(t, http.StatusNoContent, code, string(body))
	s3Client.AssertNumberOfCalls(t, "Delete", 2)
	assert.Empty(t, uploads.uploads)
	assert.Empty(t, uploads.files)

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"late.txt": "nope",
	}))
	assert.Equal(t, http.StatusNotFound, code, string(body))
}

func TestClearinghouseDeleteSubmission(t *testing.T) {
	oldClearinghouse := config.Get().Features.LightwellClearinghouse
	t.Cleanup(func() {
		config.Get().Features.LightwellClearinghouse = oldClearinghouse
	})
	config.Get().Features.LightwellClearinghouse = config.Feature{Enabled: true}

	uploads := &memClearinghouseStore{}
	s3Client := s3_client.NewMockS3Client(t)
	s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil)
	s3Client.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(nil)

	code, body := serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
	}))
	require.Equal(t, http.StatusCreated, code, string(body))
	var created api.ClearinghouseSubmission
	require.NoError(t, json.Unmarshal(body, &created))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseUploadRequest(t, http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"reproducer.py": "print('hi')",
	}))
	require.Equal(t, http.StatusOK, code, string(body))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseRequest(t, http.MethodDelete, "/clearinghouse/submissions/"+created.UUID+"/files/missing.txt"))
	assert.Equal(t, http.StatusNotFound, code, string(body))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseRequest(t, http.MethodDelete, "/clearinghouse/submissions/"+created.UUID+"/files/reproducer.py"))
	assert.Equal(t, http.StatusNoContent, code, string(body))

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseRequest(t, http.MethodDelete, "/clearinghouse/submissions/"+created.UUID))
	assert.Equal(t, http.StatusNoContent, code, string(body))
	s3Client.AssertNumberOfCalls(t, "Delete", 2)

	code, body = serveClearinghouse(t, uploads, s3Client, clearinghouseRequest(t, http.MethodGet, "/clearinghouse/submissions/"+created.UUID))
	assert.Equal(t, http.StatusNotFound, code, string(body))
}

type memClearinghouseStore struct {
	uploads []dao.ClearinghouseUpload
	files   []dao.ClearinghouseUploadFile
}

func (m *memClearinghouseStore) Create(_ context.Context, params dao.CreateClearinghouseUploadParams) (dao.ClearinghouseUpload, error) {
	row := dao.ClearinghouseUpload{
		UUID:     params.UUID,
		OrgID:    params.OrgID,
		Username: params.Username,
		Filename: params.Filename,
		Status:   params.Status,
	}
	m.uploads = append(m.uploads, row)
	return row, nil
}

func (m *memClearinghouseStore) Get(_ context.Context, uploadID uuid.UUID) (dao.ClearinghouseUpload, error) {
	for _, upload := range m.uploads {
		if upload.UUID == uploadID {
			return upload, nil
		}
	}
	return dao.ClearinghouseUpload{}, pgx.ErrNoRows
}

func (m *memClearinghouseStore) Delete(_ context.Context, uploadID uuid.UUID) error {
	keptUploads := make([]dao.ClearinghouseUpload, 0, len(m.uploads))
	for _, upload := range m.uploads {
		if upload.UUID != uploadID {
			keptUploads = append(keptUploads, upload)
		}
	}
	m.uploads = keptUploads
	keptFiles := make([]dao.ClearinghouseUploadFile, 0, len(m.files))
	for _, file := range m.files {
		if file.UploadUUID != uploadID {
			keptFiles = append(keptFiles, file)
		}
	}
	m.files = keptFiles
	return nil
}

func (m *memClearinghouseStore) List(_ context.Context, params dao.ListClearinghouseUploadsParams) ([]dao.ClearinghouseUpload, error) {
	out := []dao.ClearinghouseUpload{}
	for _, upload := range m.uploads {
		if params.OrgID == nil || upload.OrgID == *params.OrgID {
			out = append(out, upload)
		}
	}
	return out, nil
}

func (m *memClearinghouseStore) Count(_ context.Context, orgID *string) (int64, error) {
	rows, err := m.List(context.Background(), dao.ListClearinghouseUploadsParams{OrgID: orgID})
	return int64(len(rows)), err
}

func (m *memClearinghouseStore) CreateFile(ctx context.Context, params dao.CreateClearinghouseUploadFileParams) (dao.ClearinghouseUploadFile, error) {
	rows, err := m.CreateFiles(ctx, []dao.CreateClearinghouseUploadFileParams{params}, nil)
	if err != nil {
		return dao.ClearinghouseUploadFile{}, err
	}
	return rows[0], nil
}

func (m *memClearinghouseStore) CreateFiles(_ context.Context, params []dao.CreateClearinghouseUploadFileParams, beforeCommit func([]dao.ClearinghouseUploadFile) error) ([]dao.ClearinghouseUploadFile, error) {
	for _, file := range params {
		found := false
		for _, upload := range m.uploads {
			if upload.UUID != file.UploadUUID {
				continue
			}
			found = true
			if upload.Status != api.ClearinghouseSubmissionStatusUploading {
				return nil, dao.ErrClearinghouseSubmissionClosed
			}
		}
		if !found {
			return nil, pgx.ErrNoRows
		}
	}
	seen := map[string]struct{}{}
	for _, file := range params {
		key := file.UploadUUID.String() + "\x00" + file.Filename
		if _, ok := seen[key]; ok {
			return nil, &pgconn.PgError{Code: "23505"}
		}
		seen[key] = struct{}{}
		for _, existing := range m.files {
			if existing.UploadUUID == file.UploadUUID && existing.Filename == file.Filename {
				return nil, &pgconn.PgError{Code: "23505"}
			}
		}
	}
	out := make([]dao.ClearinghouseUploadFile, 0, len(params))
	for _, file := range params {
		out = append(out, dao.ClearinghouseUploadFile(file))
	}
	if beforeCommit != nil {
		if err := beforeCommit(out); err != nil {
			return nil, err
		}
	}
	m.files = append(m.files, out...)
	return out, nil
}

func (m *memClearinghouseStore) DeleteFile(_ context.Context, fileID uuid.UUID) error {
	kept := make([]dao.ClearinghouseUploadFile, 0, len(m.files))
	for _, file := range m.files {
		if file.UUID != fileID {
			kept = append(kept, file)
		}
	}
	m.files = kept
	return nil
}

func (m *memClearinghouseStore) ListFiles(ctx context.Context, uploadID uuid.UUID) ([]dao.ClearinghouseUploadFile, error) {
	return m.ListFilesByUploads(ctx, []uuid.UUID{uploadID})
}

func (m *memClearinghouseStore) ListFilesByUploads(_ context.Context, uploadIDs []uuid.UUID) ([]dao.ClearinghouseUploadFile, error) {
	wanted := map[uuid.UUID]struct{}{}
	for _, id := range uploadIDs {
		wanted[id] = struct{}{}
	}
	out := []dao.ClearinghouseUploadFile{}
	for _, file := range m.files {
		if _, ok := wanted[file.UploadUUID]; ok {
			out = append(out, file)
		}
	}
	return out, nil
}

func (m *memClearinghouseStore) GetFileByName(_ context.Context, uploadID uuid.UUID, filename string) (dao.ClearinghouseUploadFile, error) {
	for _, file := range m.files {
		if file.UploadUUID == uploadID && file.Filename == filename {
			return file, nil
		}
	}
	return dao.ClearinghouseUploadFile{}, pgx.ErrNoRows
}

func (m *memClearinghouseStore) Submit(_ context.Context, uploadID uuid.UUID) (dao.ClearinghouseUpload, error) {
	for i := range m.uploads {
		if m.uploads[i].UUID != uploadID {
			continue
		}
		if m.uploads[i].Status != api.ClearinghouseSubmissionStatusUploading {
			return dao.ClearinghouseUpload{}, pgx.ErrNoRows
		}
		m.uploads[i].Status = api.ClearinghouseSubmissionStatusSubmitted
		return m.uploads[i], nil
	}
	return dao.ClearinghouseUpload{}, pgx.ErrNoRows
}

func (m *memClearinghouseStore) Process(_ context.Context, uploadID uuid.UUID, username string) (dao.ClearinghouseUpload, error) {
	now := time.Now().UTC()
	for i := range m.uploads {
		if m.uploads[i].UUID != uploadID {
			continue
		}
		if m.uploads[i].Status != api.ClearinghouseSubmissionStatusSubmitted {
			return dao.ClearinghouseUpload{}, pgx.ErrNoRows
		}
		m.uploads[i].Status = api.ClearinghouseSubmissionStatusProcessed
		m.uploads[i].ProcessedAt = &now
		m.uploads[i].ProcessedBy = &username
		return m.uploads[i], nil
	}
	return dao.ClearinghouseUpload{}, pgx.ErrNoRows
}

func (m *memClearinghouseStore) DeleteIfUploading(ctx context.Context, uploadID uuid.UUID, beforeCommit func([]dao.ClearinghouseUploadFile) error) error {
	upload, err := m.uploadingUpload(uploadID)
	if err != nil {
		return err
	}
	files, err := m.ListFiles(ctx, upload.UUID)
	if err != nil {
		return err
	}
	if beforeCommit != nil {
		if err := beforeCommit(files); err != nil {
			return err
		}
	}
	return m.Delete(ctx, uploadID)
}

func (m *memClearinghouseStore) DeleteFileIfUploading(_ context.Context, uploadID uuid.UUID, filename string, beforeCommit func(dao.ClearinghouseUploadFile) error) error {
	if _, err := m.uploadingUpload(uploadID); err != nil {
		return err
	}
	var file dao.ClearinghouseUploadFile
	found := false
	for _, candidate := range m.files {
		if candidate.UploadUUID == uploadID && candidate.Filename == filename {
			file = candidate
			found = true
			break
		}
	}
	if !found {
		return pgx.ErrNoRows
	}
	if beforeCommit != nil {
		if err := beforeCommit(file); err != nil {
			return err
		}
	}
	return m.DeleteFile(context.Background(), file.UUID)
}

func (m *memClearinghouseStore) uploadingUpload(uploadID uuid.UUID) (dao.ClearinghouseUpload, error) {
	for _, upload := range m.uploads {
		if upload.UUID != uploadID {
			continue
		}
		if upload.Status != api.ClearinghouseSubmissionStatusUploading {
			return dao.ClearinghouseUpload{}, dao.ErrClearinghouseSubmissionClosed
		}
		return upload, nil
	}
	return dao.ClearinghouseUpload{}, pgx.ErrNoRows
}

func serveClearinghouse(t *testing.T, uploads dao.ClearinghouseUploadDao, s3Client s3_client.S3Client, req *http.Request) (int, []byte) {
	t.Helper()
	router := echo.New()
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	RegisterClearinghouseRoutes(router.Group(LightwellAPIPath), uploads, s3Client)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	body, err := io.ReadAll(rr.Result().Body)
	require.NoError(t, err)
	return rr.Code, body
}

func clearinghouseRequest(t *testing.T, method, path string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, LightwellAPIPath+path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	return req
}

func clearinghouseUploadRequest(t *testing.T, method, path, field string, files map[string]string) *http.Request {
	t.Helper()
	return clearinghouseUploadRequestAs(t, method, path, field, files, test_handler.MockIdentity)
}

func clearinghouseUploadRequestAs(t *testing.T, method, path, field string, files map[string]string, id identity.XRHID) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for name, content := range files {
		part, err := writer.CreateFormFile(field, name)
		require.NoError(t, err)
		_, err = part.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(method, LightwellAPIPath+path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, id))
	return req
}
