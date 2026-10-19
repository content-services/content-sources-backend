package lightwell

import (
	"archive/zip"
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
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type ClearinghouseSuite struct {
	LightwellSuite
	clearinghouseFeature config.Feature
	readFeature          config.Feature
}

func TestClearinghouseSuite(t *testing.T) {
	suite.Run(t, new(ClearinghouseSuite))
}

func (s *ClearinghouseSuite) SetupTest() {
	s.LightwellSuite.SetupTest()
	s.clearinghouseFeature = config.Get().Features.LightwellClearinghouse
	s.readFeature = config.Get().Features.LightwellClearinghouseRead
	config.Get().Features.LightwellClearinghouse = config.Feature{}
	config.Get().Features.LightwellClearinghouseRead = config.Feature{}
}

func (s *ClearinghouseSuite) TearDownTest() {
	config.Get().Features.LightwellClearinghouse = s.clearinghouseFeature
	config.Get().Features.LightwellClearinghouseRead = s.readFeature
	s.reg.ClearinghouseUpload.AssertExpectations(s.T())
}

func (s *ClearinghouseSuite) enableClearinghouse() {
	config.Get().Features.LightwellClearinghouse = config.Feature{Enabled: true}
}

func (s *ClearinghouseSuite) enableRead() {
	config.Get().Features.LightwellClearinghouseRead = config.Feature{Enabled: true}
}

func (s *ClearinghouseSuite) disableRead() {
	config.Get().Features.LightwellClearinghouseRead = config.Feature{}
}

func (s *ClearinghouseSuite) serve(req *http.Request) (int, []byte) {
	code, body, err := s.serveRouter(req)
	s.Require().NoError(err)
	return code, body
}

func (s *ClearinghouseSuite) allowPut() {
	s.s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil)
}

func (s *ClearinghouseSuite) expectCreate() {
	s.reg.ClearinghouseUpload.On("CreateWithFiles", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(func(_ context.Context, upload dao.CreateClearinghouseUploadParams, params []dao.CreateClearinghouseUploadFileParams, before func([]dao.ClearinghouseUploadFile) error) (dao.ClearinghouseUpload, []dao.ClearinghouseUploadFile, error) {
			rows := clearinghouseRows(params)
			if before != nil {
				if err := before(rows); err != nil {
					return dao.ClearinghouseUpload{}, nil, err
				}
			}
			return dao.ClearinghouseUpload{
				UUID:      upload.UUID,
				OrgID:     upload.OrgID,
				Username:  upload.Username,
				Filename:  upload.Filename,
				Status:    upload.Status,
				CreatedAt: time.Now().UTC(),
			}, rows, nil
		}).Once()
}

func (s *ClearinghouseSuite) expectCreateFiles(check func([]dao.CreateClearinghouseUploadFileParams)) {
	s.reg.ClearinghouseUpload.On("CreateFiles", mock.Anything, mock.Anything, mock.Anything).
		Return(func(_ context.Context, params []dao.CreateClearinghouseUploadFileParams, before func([]dao.ClearinghouseUploadFile) error) ([]dao.ClearinghouseUploadFile, error) {
			if check != nil {
				check(params)
			}
			rows := clearinghouseRows(params)
			if before != nil {
				if err := before(rows); err != nil {
					return nil, err
				}
			}
			return rows, nil
		}).Once()
}

func (s *ClearinghouseSuite) TestCreateValidation() {
	code, body := s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
	}))
	s.Equal(http.StatusBadRequest, code, string(body))

	s.enableClearinghouse()
	code, body = s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		strings.Repeat("a", 300) + ".json": "{}",
	}))
	s.Equal(http.StatusBadRequest, code, string(body))

	code, body = s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"one.json": "{}",
		"two.json": "{}",
	}))
	s.Equal(http.StatusBadRequest, code, string(body))
}
func (s *ClearinghouseSuite) TestCreate() {
	s.enableClearinghouse()
	s.allowPut()
	s.expectCreate()

	code, body := s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
	}))
	s.Require().Equal(http.StatusCreated, code, string(body))
	var created api.ClearinghouseSubmission
	s.Require().NoError(json.Unmarshal(body, &created))
	s.Equal(api.ClearinghouseSubmissionStatusPending, created.Status)
	s.Equal(test_handler.MockOrgId, created.OrgID)
	s.Equal("user", created.Username)
	s.Equal("vulnerabilities.json", created.Filename)
	s.Require().Len(created.Files, 1)
	s.Equal("vulnerabilities.json", created.Files[0].Filename)
	s.Equal("user", created.Files[0].Username)
	s.False(created.Files[0].CreatedAt.IsZero())
	s.s3Client.AssertNumberOfCalls(s.T(), "Put", 1)
}
func (s *ClearinghouseSuite) TestCreatePutFailure() {
	s.enableClearinghouse()
	s.expectCreate()
	s.s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(errors.New("put failed")).Once()

	code, body := s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
	}))
	s.Equal(http.StatusInternalServerError, code, string(body))
	s.Contains(string(body), "put failed")
}
func (s *ClearinghouseSuite) TestAddFiles() {
	s.enableClearinghouse()
	s.allowPut()
	id := uuid.New()
	upload := clearinghouseUpload(id, api.ClearinghouseSubmissionStatusPending)
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(upload, nil)

	s.expectCreateFiles(nil)
	s.expectCreateFiles(nil)
	s.expectCreateFiles(func(params []dao.CreateClearinghouseUploadFileParams) {
		s.Require().Len(params, 1)
		s.Equal("other.txt", params[0].Filename)
		s.Equal("other", params[0].Username)
	})
	s.reg.ClearinghouseUpload.On("CreateFiles", mock.Anything, mock.Anything, mock.Anything).Return(nil, &pgconn.PgError{Code: "23505"}).Once()
	s.reg.ClearinghouseUpload.On("CreateFiles", mock.Anything, mock.Anything, mock.Anything).Return(nil, &pgconn.PgError{Code: "23505"}).Once()
	s.expectCreateFiles(nil)

	now := time.Now().UTC()
	file := func(name, username string) dao.ClearinghouseUploadFile {
		return clearinghouseFile(id, name, username, now)
	}
	vuln := file("vulnerabilities.json", "user")
	reproducer := file("reproducer.py", "user")
	notes := file("notes.txt", "user")
	other := file("other.txt", "other")
	extra := file("extra.txt", "user")
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, id).Return([]dao.ClearinghouseUploadFile{vuln, reproducer}, nil).Once()
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, id).Return([]dao.ClearinghouseUploadFile{vuln, reproducer, notes}, nil).Once()
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, id).Return([]dao.ClearinghouseUploadFile{vuln, reproducer, notes, other}, nil).Once()
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, id).Return([]dao.ClearinghouseUploadFile{vuln, reproducer, notes, other}, nil).Once()
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, id).Return([]dao.ClearinghouseUploadFile{vuln, reproducer, notes, other, extra}, nil).Once()

	path := "/clearinghouse/submissions/" + id.String() + "/files"
	code, body := s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, path, "files", map[string]string{
		"reproducer.py": "print('hi')",
	}))
	s.Require().Equal(http.StatusOK, code, string(body))

	code, body = s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, path, "files", map[string]string{
		"notes.txt": "more",
	}))
	s.Require().Equal(http.StatusOK, code, string(body))
	var updated api.ClearinghouseSubmission
	s.Require().NoError(json.Unmarshal(body, &updated))
	s.Len(updated.Files, 3)
	s.Equal("user", updated.Username)

	code, body = s.serve(clearinghouseUploadRequestAs(s.T(), http.MethodPost, path, "files", map[string]string{
		"other.txt": "nope",
	}, clearinghouseOtherUser()))
	s.Require().Equal(http.StatusOK, code, string(body))
	s.Require().NoError(json.Unmarshal(body, &updated))
	var added api.ClearinghouseSubmissionFile
	foundOther := false
	for _, submissionFile := range updated.Files {
		if submissionFile.Filename == "other.txt" {
			added = submissionFile
			foundOther = true
		}
	}
	s.Require().True(foundOther)
	s.Equal("other", added.Username)

	code, body = s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, path, "files", map[string]string{
		"notes.txt": "again",
	}))
	s.Equal(http.StatusConflict, code, string(body))
	s.s3Client.AssertNumberOfCalls(s.T(), "Put", 3)

	code, body = s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, path, "files", map[string]string{
		"notes.txt": "again",
		"extra.txt": "new",
	}))
	s.Equal(http.StatusConflict, code, string(body))
	s.s3Client.AssertNumberOfCalls(s.T(), "Put", 3)

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions/"+id.String()))
	s.Require().Equal(http.StatusOK, code, string(body))
	var fetched api.ClearinghouseSubmission
	s.Require().NoError(json.Unmarshal(body, &fetched))
	s.Len(fetched.Files, 4)
	for _, submissionFile := range fetched.Files {
		s.NotEqual("extra.txt", submissionFile.Filename)
	}

	code, body = s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, path, "files", map[string]string{
		"extra.txt": "new",
	}))
	s.Require().Equal(http.StatusOK, code, string(body))
	s.s3Client.AssertNumberOfCalls(s.T(), "Put", 4)
}
func (s *ClearinghouseSuite) TestAddFilesPutFailure() {
	s.enableClearinghouse()
	s.s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil).Once()
	s.s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil).Once()
	s.s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(errors.New("put failed")).Once()
	s.s3Client.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(nil).Once()
	s.expectCreate()
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, mock.Anything).Return(func(_ context.Context, uploadID uuid.UUID) (dao.ClearinghouseUpload, error) {
		return clearinghouseUpload(uploadID, api.ClearinghouseSubmissionStatusPending), nil
	}).Once()
	s.expectCreateFiles(nil)

	code, body := s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
	}))
	s.Require().Equal(http.StatusCreated, code, string(body))
	var created api.ClearinghouseSubmission
	s.Require().NoError(json.Unmarshal(body, &created))

	code, body = s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"notes.txt": "more",
		"extra.txt": "new",
	}))
	s.Equal(http.StatusInternalServerError, code, string(body))
	s.Contains(string(body), "put failed")
}
func (s *ClearinghouseSuite) TestAddFilesCleanupFailure() {
	s.enableClearinghouse()
	s.s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil).Once()
	s.s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil).Once()
	s.s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(errors.New("put failed")).Once()
	s.s3Client.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(errors.New("delete failed")).Once()
	s.expectCreate()
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, mock.Anything).Return(func(_ context.Context, uploadID uuid.UUID) (dao.ClearinghouseUpload, error) {
		return clearinghouseUpload(uploadID, api.ClearinghouseSubmissionStatusPending), nil
	}).Once()
	s.expectCreateFiles(nil)

	code, body := s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", "file", map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
	}))
	s.Require().Equal(http.StatusCreated, code, string(body))
	var created api.ClearinghouseSubmission
	s.Require().NoError(json.Unmarshal(body, &created))

	code, body = s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+created.UUID+"/files", "files", map[string]string{
		"notes.txt": "more",
		"extra.txt": "new",
	}))
	s.Equal(http.StatusInternalServerError, code, string(body))
	s.Contains(string(body), "delete failed")
}
func (s *ClearinghouseSuite) TestDeleteFile() {
	s.enableClearinghouse()
	id := uuid.New()
	upload := clearinghouseUpload(id, api.ClearinghouseSubmissionStatusPending)
	now := time.Now().UTC()
	notes := clearinghouseFile(id, "notes.txt", "user", now)
	remaining := []dao.ClearinghouseUploadFile{
		clearinghouseFile(id, "vulnerabilities.json", "user", now),
		clearinghouseFile(id, "reproducer.py", "user", now),
		clearinghouseFile(id, "extra.txt", "user", now),
	}
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(upload, nil).Twice()
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(upload, nil).Once()
	s.reg.ClearinghouseUpload.On("DeleteFileIfUploading", mock.Anything, id, "notes.txt", mock.Anything).
		Return(func(_ context.Context, _ uuid.UUID, _ string, after func(dao.ClearinghouseUploadFile) error) error {
			return after(notes)
		}).Once()
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, id).Return(remaining, nil).Once()
	s.s3Client.On("Delete", mock.Anything, notes.Filepath).Return(nil).Once()

	path := "/clearinghouse/submissions/" + id.String() + "/files/notes.txt"
	code, body := s.serve(clearinghouseRequestAs(s.T(), http.MethodDelete, path, clearinghouseOtherUser()))
	s.Equal(http.StatusForbidden, code, string(body))

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodDelete, path))
	s.Equal(http.StatusNoContent, code, string(body))
	s.s3Client.AssertNumberOfCalls(s.T(), "Delete", 1)

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions/"+id.String()))
	s.Require().Equal(http.StatusOK, code, string(body))
	var fetched api.ClearinghouseSubmission
	s.Require().NoError(json.Unmarshal(body, &fetched))
	s.Len(fetched.Files, 3)
	for _, submissionFile := range fetched.Files {
		s.Equal("user", submissionFile.Username)
		s.False(submissionFile.CreatedAt.IsZero())
	}
}
func (s *ClearinghouseSuite) TestDeleteOriginalFile() {
	s.enableClearinghouse()
	id := uuid.New()
	upload := clearinghouseUpload(id, api.ClearinghouseSubmissionStatusPending)
	now := upload.CreatedAt
	files := []dao.ClearinghouseUploadFile{
		clearinghouseFile(id, "vulnerabilities.json", "user", now),
		clearinghouseFile(id, "reproducer.py", "user", now),
	}
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(upload, nil).Once()
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(dao.ClearinghouseUpload{}, pgx.ErrNoRows).Once()
	s.reg.ClearinghouseUpload.On("DeleteIfUploading", mock.Anything, id, mock.Anything).
		Return(func(_ context.Context, _ uuid.UUID, after func([]dao.ClearinghouseUploadFile) error) error {
			return after(files)
		}).Once()
	s.s3Client.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(nil)

	code, body := s.serve(clearinghouseRequest(s.T(), http.MethodDelete, "/clearinghouse/submissions/"+id.String()+"/files/vulnerabilities.json"))
	s.Equal(http.StatusNoContent, code, string(body))
	s.s3Client.AssertNumberOfCalls(s.T(), "Delete", 2)

	code, body = s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+id.String()+"/files", "files", map[string]string{
		"late.txt": "nope",
	}))
	s.Equal(http.StatusNotFound, code, string(body))
}
func (s *ClearinghouseSuite) TestDeleteSubmission() {
	s.enableClearinghouse()
	id := uuid.New()
	upload := clearinghouseUpload(id, api.ClearinghouseSubmissionStatusPending)
	reproducer := clearinghouseFile(id, "reproducer.py", "user", upload.CreatedAt)
	original := clearinghouseFile(id, "vulnerabilities.json", "user", upload.CreatedAt)
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(upload, nil).Times(3)
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(dao.ClearinghouseUpload{}, pgx.ErrNoRows).Once()
	s.reg.ClearinghouseUpload.On("DeleteFileIfUploading", mock.Anything, id, "missing.txt", mock.Anything).Return(pgx.ErrNoRows).Once()
	s.reg.ClearinghouseUpload.On("DeleteFileIfUploading", mock.Anything, id, "reproducer.py", mock.Anything).
		Return(func(_ context.Context, _ uuid.UUID, _ string, after func(dao.ClearinghouseUploadFile) error) error {
			return after(reproducer)
		}).Once()
	s.reg.ClearinghouseUpload.On("DeleteIfUploading", mock.Anything, id, mock.Anything).
		Return(func(_ context.Context, _ uuid.UUID, after func([]dao.ClearinghouseUploadFile) error) error {
			return after([]dao.ClearinghouseUploadFile{original})
		}).Once()
	s.s3Client.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(nil)

	code, body := s.serve(clearinghouseRequest(s.T(), http.MethodDelete, "/clearinghouse/submissions/"+id.String()+"/files/missing.txt"))
	s.Equal(http.StatusNotFound, code, string(body))

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodDelete, "/clearinghouse/submissions/"+id.String()+"/files/reproducer.py"))
	s.Equal(http.StatusNoContent, code, string(body))

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodDelete, "/clearinghouse/submissions/"+id.String()))
	s.Equal(http.StatusNoContent, code, string(body))
	s.s3Client.AssertNumberOfCalls(s.T(), "Delete", 2)

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions/"+id.String()))
	s.Equal(http.StatusNotFound, code, string(body))
}
func (s *ClearinghouseSuite) TestListAndGet() {
	s.enableClearinghouse()
	id := uuid.New()
	otherID := uuid.New()
	now := time.Now().UTC()
	mine := clearinghouseUpload(id, api.ClearinghouseSubmissionStatusPending)
	other := dao.ClearinghouseUpload{
		UUID:      otherID,
		OrgID:     "other-org",
		Username:  "someone",
		Filename:  "vulnerabilities.json",
		Status:    api.ClearinghouseSubmissionStatusSubmitted,
		CreatedAt: now,
	}
	files := []dao.ClearinghouseUploadFile{
		clearinghouseFile(id, "vulnerabilities.json", "user", now),
		clearinghouseFile(id, "reproducer.py", "user", now),
		clearinghouseFile(id, "extra.txt", "user", now),
	}
	s.reg.ClearinghouseUpload.On("List", mock.Anything, mock.MatchedBy(clearinghouseOrgParams(test_handler.MockOrgId))).
		Return([]dao.ClearinghouseUpload{mine}, nil).Once()
	s.reg.ClearinghouseUpload.On("Count", mock.Anything, mock.MatchedBy(func(orgID *string) bool {
		return orgID != nil && *orgID == test_handler.MockOrgId
	})).Return(int64(1), nil).Once()
	s.reg.ClearinghouseUpload.On("ListFilesByUploads", mock.Anything, []uuid.UUID{id}).Return(files, nil).Once()
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, otherID).Return(other, nil)
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(mine, nil).Once()
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, id).Return(files, nil).Once()

	code, body := s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions"))
	s.Require().Equal(http.StatusOK, code, string(body))
	var listed api.ClearinghouseSubmissionCollectionResponse
	s.Require().NoError(json.Unmarshal(body, &listed))
	s.Require().Len(listed.Data, 1)
	s.Equal(id.String(), listed.Data[0].UUID)
	s.Equal("user", listed.Data[0].Username)
	s.False(listed.Data[0].CreatedAt.IsZero())
	s.Nil(listed.Data[0].SubmittedAt)
	s.Nil(listed.Data[0].SubmittedBy)
	s.Require().Len(listed.Data[0].Files, 3)
	for _, submissionFile := range listed.Data[0].Files {
		s.Equal("user", submissionFile.Username)
		s.False(submissionFile.CreatedAt.IsZero())
	}

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions/"+id.String()))
	s.Require().Equal(http.StatusOK, code, string(body))
	var fetched api.ClearinghouseSubmission
	s.Require().NoError(json.Unmarshal(body, &fetched))
	s.Equal(id.String(), fetched.UUID)
	s.Equal(api.ClearinghouseSubmissionStatusPending, fetched.Status)
	s.False(fetched.CreatedAt.IsZero())
	s.Require().Len(fetched.Files, 3)

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions/"+otherID.String()))
	s.Equal(http.StatusNotFound, code, string(body))

	s.enableRead()
	s.reg.ClearinghouseUpload.On("List", mock.Anything, mock.MatchedBy(func(params dao.ListClearinghouseUploadsParams) bool {
		return params.OrgID == nil && params.Limit == 100 && params.Offset == 0
	})).Return([]dao.ClearinghouseUpload{mine, other}, nil).Once()
	s.reg.ClearinghouseUpload.On("Count", mock.Anything, mock.MatchedBy(func(orgID *string) bool {
		return orgID == nil
	})).Return(int64(2), nil).Once()
	s.reg.ClearinghouseUpload.On("ListFilesByUploads", mock.Anything, []uuid.UUID{id, otherID}).Return(files, nil).Once()
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, otherID).Return([]dao.ClearinghouseUploadFile{}, nil).Once()

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions"))
	s.Require().Equal(http.StatusOK, code, string(body))
	s.Require().NoError(json.Unmarshal(body, &listed))
	s.Len(listed.Data, 2)

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions/"+otherID.String()))
	s.Require().Equal(http.StatusOK, code, string(body))
	s.Require().NoError(json.Unmarshal(body, &fetched))
	s.Equal(otherID.String(), fetched.UUID)
	s.Equal("someone", fetched.Username)
	s.Equal(api.ClearinghouseSubmissionStatusSubmitted, fetched.Status)
}

func clearinghouseRows(params []dao.CreateClearinghouseUploadFileParams) []dao.ClearinghouseUploadFile {
	now := time.Now().UTC()
	rows := make([]dao.ClearinghouseUploadFile, len(params))
	for i, param := range params {
		rows[i] = dao.ClearinghouseUploadFile{
			UUID:       param.UUID,
			UploadUUID: param.UploadUUID,
			Filename:   param.Filename,
			Filepath:   param.Filepath,
			Username:   param.Username,
			CreatedAt:  now,
		}
	}
	return rows
}

func clearinghouseUpload(id uuid.UUID, status string) dao.ClearinghouseUpload {
	return dao.ClearinghouseUpload{
		UUID:      id,
		OrgID:     test_handler.MockOrgId,
		Username:  "user",
		Filename:  "vulnerabilities.json",
		Status:    status,
		CreatedAt: time.Now().UTC(),
	}
}

func clearinghouseFile(uploadID uuid.UUID, name, username string, createdAt time.Time) dao.ClearinghouseUploadFile {
	return dao.ClearinghouseUploadFile{
		UUID:       uuid.New(),
		UploadUUID: uploadID,
		Filename:   name,
		Filepath:   "uploads/" + uploadID.String() + "/" + name,
		Username:   username,
		CreatedAt:  createdAt,
	}
}

func clearinghouseOrgParams(orgID string) func(dao.ListClearinghouseUploadsParams) bool {
	return func(params dao.ListClearinghouseUploadsParams) bool {
		return params.Limit == 100 && params.Offset == 0 && params.OrgID != nil && *params.OrgID == orgID
	}
}

func clearinghouseOtherUser() identity.XRHID {
	other := test_handler.MockIdentity
	user := *test_handler.MockIdentity.Identity.User
	user.Username = "other"
	other.Identity.User = &user
	return other
}

func readZipEntry(t *testing.T, file *zip.File) string {
	t.Helper()
	reader, err := file.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

type failingReadCloser struct{}

func (failingReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func (failingReadCloser) Close() error {
	return nil
}

func clearinghouseRequest(t *testing.T, method, path string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, LightwellAPIPath+path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	return req
}

func clearinghouseRequestAs(t *testing.T, method, path string, id identity.XRHID) *http.Request {
	t.Helper()
	req := clearinghouseRequest(t, method, path)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, id))
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
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, LightwellAPIPath+path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, id))
	return req
}
