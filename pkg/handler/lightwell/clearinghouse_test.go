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
