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
				UUID:               upload.UUID,
				OrgID:              upload.OrgID,
				Username:           upload.Username,
				Name:               upload.Name,
				Filename:           upload.Filename,
				VulnerabilityCount: upload.VulnerabilityCount,
				Status:             upload.Status,
				CreatedAt:          time.Now().UTC(),
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
	// Feature not enabled.
	code, body := s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", clearinghouseSubmissionJSON("a.json")))
	s.Equal(http.StatusBadRequest, code, string(body))

	s.enableClearinghouse()

	// Malformed JSON.
	code, body = s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", `not json`))
	s.Equal(http.StatusBadRequest, code, string(body))

	// Missing vulnerabilities array.
	code, body = s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", `{"name":"a.json"}`))
	s.Equal(http.StatusBadRequest, code, string(body))

	// Empty vulnerabilities array.
	code, body = s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", `{"name":"a.json","vulnerabilities":[]}`))
	s.Equal(http.StatusBadRequest, code, string(body))

	// Missing name.
	code, body = s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", `{"vulnerabilities":[`+validVulnerability+`]}`))
	s.Equal(http.StatusBadRequest, code, string(body))

	// Blank name.
	code, body = s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", `{"name":"   ","vulnerabilities":[`+validVulnerability+`]}`))
	s.Equal(http.StatusBadRequest, code, string(body))

	// Vulnerability missing a required field (description).
	missingField := `{"vulnerability_id":"CVE-1","packageurl":"pkg:golang/example","title":"t"}`
	code, body = s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", clearinghouseSubmissionJSON("a.json", missingField)))
	s.Equal(http.StatusBadRequest, code, string(body))

	// Vulnerability with an empty required field (vulnerability_id).
	emptyField := `{"vulnerability_id":"","packageurl":"pkg:golang/example","title":"t","description":"d"}`
	code, body = s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", clearinghouseSubmissionJSON("a.json", emptyField)))
	s.Equal(http.StatusBadRequest, code, string(body))

	// A submission with the second of two vulnerabilities invalid is rejected.
	code, body = s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", clearinghouseSubmissionJSON("a.json", validVulnerability, missingField)))
	s.Equal(http.StatusBadRequest, code, string(body))
}
func (s *ClearinghouseSuite) TestCreate() {
	s.enableClearinghouse()
	var stored []byte
	s.s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).
		Return(func(_ context.Context, _ string, body io.Reader, _ int64) error {
			stored, _ = io.ReadAll(body)
			return nil
		}).Once()
	s.expectCreate()

	submission := clearinghouseSubmissionJSON("My Intake Batch", validVulnerability, validVulnerability)
	code, body := s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", submission))
	s.Require().Equal(http.StatusCreated, code, string(body))
	var created api.ClearinghouseSubmission
	s.Require().NoError(json.Unmarshal(body, &created))
	s.Equal(api.ClearinghouseSubmissionStatusPending, created.Status)
	s.Equal(test_handler.MockOrgId, created.OrgID)
	s.Equal("user", created.Username)
	s.Equal("My Intake Batch", created.Name)
	s.Require().NotNil(created.VulnerabilityCount)
	s.Equal(2, *created.VulnerabilityCount)
	s.Require().Len(created.Files, 1)
	s.Equal("submission.json", created.Files[0].Filename)
	s.True(created.Files[0].Submission)
	s.Equal("user", created.Files[0].Username)
	s.False(created.Files[0].CreatedAt.IsZero())
	s.s3Client.AssertNumberOfCalls(s.T(), "Put", 1)
	s.Equal(submission, string(stored))
}
func (s *ClearinghouseSuite) TestCreatePutFailure() {
	s.enableClearinghouse()
	s.expectCreate()
	s.s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(errors.New("put failed")).Once()

	code, body := s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", clearinghouseSubmissionJSON("a.json")))
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
		s.False(params[0].Submission)
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

	code, body := s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", clearinghouseSubmissionJSON("a.json")))
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

	code, body := s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/create", clearinghouseSubmissionJSON("a.json")))
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
func (s *ClearinghouseSuite) TestSubmit() {
	s.enableClearinghouse()
	id := uuid.New()
	current := clearinghouseUpload(id, api.ClearinghouseSubmissionStatusPending)
	files := []dao.ClearinghouseUploadFile{
		clearinghouseFile(id, "vulnerabilities.json", "user", current.CreatedAt),
		clearinghouseFile(id, "reproducer.py", "user", current.CreatedAt),
		clearinghouseFile(id, "extra.txt", "user", current.CreatedAt),
	}
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(func(context.Context, uuid.UUID) (dao.ClearinghouseUpload, error) {
		return current, nil
	})
	s.reg.ClearinghouseUpload.On("Submit", mock.Anything, id, "user").
		Return(func(_ context.Context, _ uuid.UUID, username string) (dao.ClearinghouseUpload, error) {
			if current.Status != api.ClearinghouseSubmissionStatusPending {
				return dao.ClearinghouseUpload{}, pgx.ErrNoRows
			}
			now := time.Now().UTC()
			current.Status = api.ClearinghouseSubmissionStatusSubmitted
			current.SubmittedAt = &now
			current.SubmittedBy = &username
			return current, nil
		})
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, id).Return(files, nil).Once()

	code, body := s.serve(clearinghouseRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+id.String()+"/submit"))
	s.Require().Equal(http.StatusOK, code, string(body))
	var fetched api.ClearinghouseSubmission
	s.Require().NoError(json.Unmarshal(body, &fetched))
	s.Equal(api.ClearinghouseSubmissionStatusSubmitted, fetched.Status)
	s.Require().NotNil(fetched.SubmittedBy)
	s.Equal("user", *fetched.SubmittedBy)
	s.Require().NotNil(fetched.SubmittedAt)
	s.False(fetched.SubmittedAt.IsZero())
	s.Len(fetched.Files, 3)

	code, body = s.serve(clearinghouseRequestAs(s.T(), http.MethodPost, "/clearinghouse/submissions/"+id.String()+"/submit", clearinghouseOtherUser()))
	s.Equal(http.StatusForbidden, code, string(body))

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+id.String()+"/submit"))
	s.Equal(http.StatusConflict, code, string(body))
	s.Contains(string(body), "can no longer be changed")

	code, body = s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+id.String()+"/files", "files", map[string]string{
		"late.txt": "nope",
	}))
	s.Equal(http.StatusConflict, code, string(body))
	s.Contains(string(body), "can no longer be changed")

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodDelete, "/clearinghouse/submissions/"+id.String()+"/files/extra.txt"))
	s.Equal(http.StatusConflict, code, string(body))
	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodDelete, "/clearinghouse/submissions/"+id.String()))
	s.Equal(http.StatusConflict, code, string(body))
}
func (s *ClearinghouseSuite) TestDownloadAccess() {
	s.enableClearinghouse()
	id := uuid.New()
	current := clearinghouseUpload(id, api.ClearinghouseSubmissionStatusPending)
	file := clearinghouseFile(id, "vulnerabilities.json", "user", current.CreatedAt)
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(func(context.Context, uuid.UUID) (dao.ClearinghouseUpload, error) {
		return current, nil
	})
	s.reg.ClearinghouseUpload.On("GetFileByName", mock.Anything, id, "vulnerabilities.json").Return(file, nil)
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, id).Return([]dao.ClearinghouseUploadFile{file}, nil)
	s.reg.ClearinghouseUpload.On("Submit", mock.Anything, id, "user").
		Return(func(_ context.Context, _ uuid.UUID, username string) (dao.ClearinghouseUpload, error) {
			now := time.Now().UTC()
			current.Status = api.ClearinghouseSubmissionStatusSubmitted
			current.SubmittedAt = &now
			current.SubmittedBy = &username
			return current, nil
		}).Once()
	s.reg.ClearinghouseUpload.On("Process", mock.Anything, id, "user").
		Return(func(_ context.Context, _ uuid.UUID, username string) (dao.ClearinghouseUpload, error) {
			now := time.Now().UTC()
			current.Status = api.ClearinghouseSubmissionStatusProcessed
			current.ProcessedAt = &now
			current.ProcessedBy = &username
			return current, nil
		}).Once()
	s.s3Client.On("Get", mock.Anything, file.Filepath).Return(func(context.Context, string) io.ReadCloser {
		return io.NopCloser(strings.NewReader("payload"))
	}, nil)

	downloadPath := "/clearinghouse/submissions/" + id.String() + "/files/vulnerabilities.json"
	other := clearinghouseOtherUser()
	code, body := s.serve(clearinghouseRequest(s.T(), http.MethodGet, downloadPath))
	s.Require().Equal(http.StatusOK, code, string(body))
	s.Equal("payload", string(body))
	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions/"+id.String()+"/submission"))
	s.Require().Equal(http.StatusOK, code, string(body))
	s.Equal("payload", string(body))

	code, body = s.serve(clearinghouseRequestAs(s.T(), http.MethodGet, downloadPath, other))
	s.Equal(http.StatusForbidden, code, string(body))
	s.Contains(string(body), "Only the user who created the submission can download it")

	foreignID := uuid.New()
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, foreignID).Return(dao.ClearinghouseUpload{
		UUID:     foreignID,
		OrgID:    "other-org",
		Username: "someone",
		Filename: "vulnerabilities.json",
		Status:   api.ClearinghouseSubmissionStatusPending,
	}, nil).Once()
	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions/"+foreignID.String()+"/files/vulnerabilities.json"))
	s.Equal(http.StatusNotFound, code, string(body))

	s.enableRead()
	code, body = s.serve(clearinghouseRequestAs(s.T(), http.MethodGet, downloadPath, other))
	s.Equal(http.StatusConflict, code, string(body))
	s.Contains(string(body), "must be submitted")

	s.disableRead()
	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+id.String()+"/submit"))
	s.Require().Equal(http.StatusOK, code, string(body))

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodGet, downloadPath))
	s.Require().Equal(http.StatusOK, code, string(body))

	s.enableRead()
	code, body = s.serve(clearinghouseRequestAs(s.T(), http.MethodGet, downloadPath, other))
	s.Require().Equal(http.StatusOK, code, string(body))
	s.Equal("payload", string(body))

	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+id.String()+"/process"))
	s.Require().Equal(http.StatusOK, code, string(body))
	var fetched api.ClearinghouseSubmission
	s.Require().NoError(json.Unmarshal(body, &fetched))
	s.Equal(api.ClearinghouseSubmissionStatusProcessed, fetched.Status)
	s.Require().NotNil(fetched.ProcessedBy)
	s.Equal("user", *fetched.ProcessedBy)
	s.Require().NotNil(fetched.ProcessedAt)
	s.False(fetched.ProcessedAt.IsZero())

	code, body = s.serve(clearinghouseRequestAs(s.T(), http.MethodGet, downloadPath, other))
	s.Require().Equal(http.StatusOK, code, string(body))
	code, body = s.serve(clearinghouseRequestAs(s.T(), http.MethodGet, "/clearinghouse/submissions/"+id.String()+"/archive", other))
	s.Require().Equal(http.StatusOK, code, string(body))
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	s.Require().NoError(err)
	s.Require().Len(archive.File, 1)

	s.disableRead()
	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodGet, downloadPath))
	s.Equal(http.StatusConflict, code, string(body))
	s.Contains(string(body), "cannot be downloaded")
}
func (s *ClearinghouseSuite) TestArchive() {
	s.enableClearinghouse()
	id := uuid.New()
	upload := clearinghouseUpload(id, api.ClearinghouseSubmissionStatusPending)
	now := upload.CreatedAt
	files := []dao.ClearinghouseUploadFile{
		clearinghouseFile(id, "reproducer.py", "user", now),
		clearinghouseFile(id, "vulnerabilities.json", "user", now),
		clearinghouseFile(id, "extra.txt", "user", now),
	}
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(upload, nil).Once()
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, id).Return(files, nil).Once()
	contents := map[string]string{
		"vulnerabilities.json": `{"vulnerabilities":[]}`,
		"extra.txt":            "new",
		"reproducer.py":        "print('hi')",
	}
	for _, file := range files {
		content := contents[file.Filename]
		s.s3Client.On("Get", mock.Anything, file.Filepath).Return(func(context.Context, string) io.ReadCloser {
			return io.NopCloser(strings.NewReader(content))
		}, nil).Once()
	}

	code, body := s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions/"+id.String()+"/archive"))
	s.Require().Equal(http.StatusOK, code, string(body))
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	s.Require().NoError(err)
	s.Require().Len(archive.File, 3)
	s.Equal([]string{"vulnerabilities.json", "extra.txt", "reproducer.py"}, []string{archive.File[0].Name, archive.File[1].Name, archive.File[2].Name})
	s.Equal(`{"vulnerabilities":[]}`, readZipEntry(s.T(), archive.File[0]))
	s.Equal("new", readZipEntry(s.T(), archive.File[1]))
	s.Equal("print('hi')", readZipEntry(s.T(), archive.File[2]))
}
func (s *ClearinghouseSuite) TestArchiveSecondFileFailure() {
	s.enableClearinghouse()
	id := uuid.New()
	upload := clearinghouseUpload(id, api.ClearinghouseSubmissionStatusPending)
	files := []dao.ClearinghouseUploadFile{
		clearinghouseFile(id, "vulnerabilities.json", "user", upload.CreatedAt),
		clearinghouseFile(id, "reproducer.py", "user", upload.CreatedAt),
	}
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(upload, nil).Once()
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, id).Return(files, nil).Once()
	s.s3Client.On("Get", mock.Anything, files[0].Filepath).Return(func(context.Context, string) io.ReadCloser {
		return io.NopCloser(strings.NewReader(`{"vulnerabilities":[]}`))
	}, nil).Once()
	s.s3Client.On("Get", mock.Anything, files[1].Filepath).Return(nil, errors.New("missing object")).Once()

	code, body := s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions/"+id.String()+"/archive"))
	s.Equal(http.StatusInternalServerError, code, string(body))
	s.Contains(string(body), "missing object")
}
func (s *ClearinghouseSuite) TestArchiveReadFailure() {
	s.enableClearinghouse()
	id := uuid.New()
	upload := clearinghouseUpload(id, api.ClearinghouseSubmissionStatusPending)
	files := []dao.ClearinghouseUploadFile{
		clearinghouseFile(id, "vulnerabilities.json", "user", upload.CreatedAt),
		clearinghouseFile(id, "reproducer.py", "user", upload.CreatedAt),
	}
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(upload, nil).Once()
	s.reg.ClearinghouseUpload.On("ListFiles", mock.Anything, id).Return(files, nil).Once()
	s.s3Client.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(func(context.Context, string) io.ReadCloser {
		return failingReadCloser{}
	}, nil).Times(2)

	code, body := s.serve(clearinghouseRequest(s.T(), http.MethodGet, "/clearinghouse/submissions/"+id.String()+"/archive"))
	s.Equal(http.StatusOK, code, string(body))
	s.NotEmpty(body)
	_, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	s.Error(err)
}
func (s *ClearinghouseSuite) TestProcessConflicts() {
	s.enableClearinghouse()
	id := uuid.New()
	current := clearinghouseUpload(id, api.ClearinghouseSubmissionStatusPending)
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, id).Return(func(context.Context, uuid.UUID) (dao.ClearinghouseUpload, error) {
		return current, nil
	})
	s.reg.ClearinghouseUpload.On("Submit", mock.Anything, id, "user").Return(dao.ClearinghouseUpload{}, pgx.ErrNoRows).Once()

	code, body := s.serve(clearinghouseRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+id.String()+"/process"))
	s.Equal(http.StatusBadRequest, code, string(body))

	s.enableRead()
	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+id.String()+"/process"))
	s.Equal(http.StatusConflict, code, string(body))
	s.Contains(string(body), "must be submitted")

	stillUploading := uuid.New()
	s.reg.ClearinghouseUpload.On("Get", mock.Anything, stillUploading).Return(dao.ClearinghouseUpload{
		UUID:     stillUploading,
		OrgID:    "other-org",
		Username: "someone",
		Status:   api.ClearinghouseSubmissionStatusPending,
	}, nil).Once()
	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+stillUploading.String()+"/process"))
	s.Equal(http.StatusConflict, code, string(body))
	s.Contains(string(body), "must be submitted")

	current.Status = api.ClearinghouseSubmissionStatusProcessed
	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+id.String()+"/process"))
	s.Equal(http.StatusConflict, code, string(body))
	s.Contains(string(body), "already been processed")

	code, body = s.serve(clearinghouseUploadRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+id.String()+"/files", "files", map[string]string{
		"late.txt": "nope",
	}))
	s.Equal(http.StatusConflict, code, string(body))
	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodDelete, "/clearinghouse/submissions/"+id.String()+"/files/extra.txt"))
	s.Equal(http.StatusConflict, code, string(body))
	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodDelete, "/clearinghouse/submissions/"+id.String()))
	s.Equal(http.StatusConflict, code, string(body))
	code, body = s.serve(clearinghouseRequest(s.T(), http.MethodPost, "/clearinghouse/submissions/"+id.String()+"/submit"))
	s.Equal(http.StatusConflict, code, string(body))
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
			Submission: param.Submission,
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

// validVulnerability is a ReportedVulnerability with every spec-required field present.
const validVulnerability = `{"vulnerability_id":"CVE-1","packageurl":"pkg:golang/example","component_name":"example","component_version":"1.0.0","title":"t","description":"d","cvss_severity":"HIGH","cvss_score":7.5}`

// clearinghouseSubmissionJSON builds an intake submission body with the given name.
// If no vulnerabilities are supplied, a single valid one is used.
func clearinghouseSubmissionJSON(name string, vulns ...string) string {
	if len(vulns) == 0 {
		vulns = []string{validVulnerability}
	}
	return `{"name":"` + name + `","vulnerabilities":[` + strings.Join(vulns, ",") + `]}`
}

func clearinghouseJSONRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, LightwellAPIPath+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
