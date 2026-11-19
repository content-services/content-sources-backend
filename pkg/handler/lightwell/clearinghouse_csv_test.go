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

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const csvCreatePath = "/clearinghouse/submissions/create/csv"

// validCSV is a CSV submission with one valid vulnerability.
const validCSV = "vulnerability_id,packageurl,title,description\nCVE-1,pkg:golang/example@1.0.0,t,d\n"

func TestParseClearinghouseCSV(t *testing.T) {
	score := 7.5
	tested := true
	cwe := "CWE-79"

	cases := []struct {
		name    string
		csv     string
		want    []csvVulnerability
		wantErr string
	}{
		{
			name: "required columns only",
			csv:  validCSV,
			want: []csvVulnerability{{VulnerabilityID: "CVE-1", Packageurl: "pkg:golang/example@1.0.0", Title: "t", Description: "d"}},
		},
		{
			name: "all columns in any order with optional values",
			csv: "description,cvss_score,title,exploit_tested,packageurl,cwe,vulnerability_id,component_name,component_version,cvss_severity,cvss_vector,reproducer_filename,customer_priority\n" +
				"d,7.5,t,true,pkg:npm/a@1,CWE-79,CVE-1,a,1,HIGH,AV:N,repro.sh,P1\n",
			want: []csvVulnerability{{
				VulnerabilityID: "CVE-1", Packageurl: "pkg:npm/a@1", ComponentName: "a", ComponentVersion: "1",
				Title: "t", Description: "d", CvssSeverity: "HIGH", CvssScore: &score, Cwe: &cwe,
				CvssVector: strPtr("AV:N"), ExploitTested: &tested, ReproducerFilename: strPtr("repro.sh"), CustomerPriority: strPtr("P1"),
			}},
		},
		{
			name: "empty optional cells are unset",
			csv:  "vulnerability_id,packageurl,title,description,cvss_score,exploit_tested,cwe\nCVE-1,p,t,d,,,\n",
			want: []csvVulnerability{{VulnerabilityID: "CVE-1", Packageurl: "p", Title: "t", Description: "d"}},
		},
		{
			name: "BOM, CRLF, padded headers and cells, quoted multi-line description",
			csv:  "\xEF\xBB\xBF vulnerability_id , packageurl ,title,description\r\n CVE-1 ,p,t,\"line 1\r\nline, 2\"\r\n",
			want: []csvVulnerability{{VulnerabilityID: "CVE-1", Packageurl: "p", Title: "t", Description: "line 1\nline, 2"}},
		},
		{
			name: "all-empty rows are skipped",
			csv:  "vulnerability_id,packageurl,title,description\n,,,\nCVE-1,p,t,d\n , , , \n",
			want: []csvVulnerability{{VulnerabilityID: "CVE-1", Packageurl: "p", Title: "t", Description: "d"}},
		},
		{name: "empty file", csv: "", wantErr: "CSV is empty"},
		{name: "header only", csv: "vulnerability_id,packageurl,title,description\n", wantErr: "at least one vulnerability is required"},
		{name: "unknown column", csv: "vulnerability_id,packageurl,title,description,severity\nCVE-1,p,t,d,HIGH\n", wantErr: `unknown column "severity"`},
		{name: "duplicate column", csv: "vulnerability_id,packageurl,title,description,title\nCVE-1,p,t,d,t\n", wantErr: `duplicate column "title"`},
		{name: "missing required column", csv: "vulnerability_id,packageurl,title\nCVE-1,p,t\n", wantErr: `missing required column "description"`},
		{name: "missing required field reports spreadsheet row", csv: validCSV + "CVE-2,p,,d\n", wantErr: `row 3: missing required field "title"`},
		{name: "bad cvss_score", csv: "vulnerability_id,packageurl,title,description,cvss_score\nCVE-1,p,t,d,high\n", wantErr: "row 2: cvss_score must be a number"},
		{name: "NaN cvss_score", csv: "vulnerability_id,packageurl,title,description,cvss_score\nCVE-1,p,t,d,NaN\n", wantErr: "row 2: cvss_score must be a number"},
		{name: "bad exploit_tested", csv: "vulnerability_id,packageurl,title,description,exploit_tested\nCVE-1,p,t,d,maybe\n", wantErr: "row 2: exploit_tested must be true or false"},
		{name: "wrong number of fields", csv: validCSV + "CVE-2,p,t\n", wantErr: "invalid CSV"},
		{name: "not UTF-8", csv: "vulnerability_id,packageurl,title,description\nCVE-1,p,t,caf\xe9\n", wantErr: "CSV must be UTF-8 encoded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseClearinghouseCSV([]byte(tc.csv))
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func (s *ClearinghouseSuite) TestCreateCSV() {
	s.enableClearinghouse()
	var stored []byte
	s.s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).
		Return(func(_ context.Context, _ string, body io.Reader, _ int64) error {
			stored, _ = io.ReadAll(body)
			return nil
		}).Once()
	var params dao.CreateClearinghouseUploadParams
	s.reg.ClearinghouseUpload.On("CreateWithFiles", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(func(_ context.Context, upload dao.CreateClearinghouseUploadParams, files []dao.CreateClearinghouseUploadFileParams, before func([]dao.ClearinghouseUploadFile) error) (dao.ClearinghouseUpload, []dao.ClearinghouseUploadFile, error) {
			params = upload
			rows := clearinghouseRows(files)
			if err := before(rows); err != nil {
				return dao.ClearinghouseUpload{}, nil, err
			}
			return dao.ClearinghouseUpload{UUID: upload.UUID, OrgID: upload.OrgID, Username: upload.Username, Name: upload.Name, Status: upload.Status, VulnerabilityCount: upload.VulnerabilityCount, PackageSummary: upload.PackageSummary}, rows, nil
		}).Once()

	csv := "vulnerability_id,packageurl,title,description,cvss_score,exploit_tested\n" +
		"CVE-1,pkg:maven/org.apache.logging.log4j/log4j-core@2.14.1,t1,d1,9.8,true\n" +
		"CVE-2,log4j-core 2.14,t2,d2,,\n"
	code, body := s.serve(clearinghouseCSVRequest(s.T(), map[string]string{"name": "  scan.csv  "}, "scan.csv", csv))
	s.Require().Equal(http.StatusCreated, code, string(body))

	var created api.ClearinghouseSubmission
	s.Require().NoError(json.Unmarshal(body, &created))
	s.Equal("scan.csv", created.Name)
	s.Equal(test_handler.MockOrgId, created.OrgID)
	s.Equal(api.ClearinghouseSubmissionStatusPending, created.Status)
	s.Require().NotNil(created.VulnerabilityCount)
	s.Equal(2, *created.VulnerabilityCount)
	s.Require().Len(created.Files, 1)
	s.Equal("submission.json", created.Files[0].Filename)
	s.True(created.Files[0].Submission)

	// The CSV is stored as intake submission JSON, with empty optional cells omitted.
	s.JSONEq(`{"name":"scan.csv","vulnerabilities":[
		{"vulnerability_id":"CVE-1","packageurl":"pkg:maven/org.apache.logging.log4j/log4j-core@2.14.1","title":"t1","description":"d1","cvss_score":9.8,"exploit_tested":true},
		{"vulnerability_id":"CVE-2","packageurl":"log4j-core 2.14","title":"t2","description":"d2"}
	]}`, string(stored))

	// Packages are parsed from the CSV like from a JSON submission.
	s.Require().NotNil(params.PackageSummary)
	s.Equal(2, params.PackageSummary.Total)
	s.Equal(1, params.PackageSummary.InvalidPurls)
	s.Len(params.Packages, 2)
}

func (s *ClearinghouseSuite) TestCreateCSVCountsFindings() {
	s.enableClearinghouse()
	s.allowPut()
	var params dao.CreateClearinghouseUploadParams
	s.reg.ClearinghouseUpload.On("CreateWithFiles", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(func(_ context.Context, upload dao.CreateClearinghouseUploadParams, files []dao.CreateClearinghouseUploadFileParams, before func([]dao.ClearinghouseUploadFile) error) (dao.ClearinghouseUpload, []dao.ClearinghouseUploadFile, error) {
			params = upload
			rows := clearinghouseRows(files)
			if err := before(rows); err != nil {
				return dao.ClearinghouseUpload{}, nil, err
			}
			return dao.ClearinghouseUpload{UUID: upload.UUID, OrgID: upload.OrgID, Status: upload.Status, FindingsSummary: upload.FindingsSummary}, rows, nil
		}).Once()

	csv := "vulnerability_id,packageurl,title,description\n" +
		"CVE-1,pkg:maven/org.x/y@1.0,t,Reported by jsmith@corp.com on 10.1.2.3\n"
	code, body := s.serve(clearinghouseCSVRequest(s.T(), map[string]string{"name": "scan.csv"}, "scan.csv", csv))
	s.Require().Equal(http.StatusCreated, code, string(body))

	want := &api.ClearinghouseFindingsSummary{Total: 2, Emails: 1, IPAddresses: 1}
	s.Equal(want, params.FindingsSummary)
	var created api.ClearinghouseSubmission
	s.Require().NoError(json.Unmarshal(body, &created))
	s.Equal(want, created.FindingsSummary)
}

func (s *ClearinghouseSuite) TestCreateCSVValidation() {
	// Feature not enabled.
	code, body := s.serve(clearinghouseCSVRequest(s.T(), map[string]string{"name": "a"}, "a.csv", validCSV))
	s.Equal(http.StatusBadRequest, code, string(body))

	s.enableClearinghouse()

	// Not a multipart form.
	code, body = s.serve(clearinghouseJSONRequest(s.T(), http.MethodPost, csvCreatePath, clearinghouseSubmissionJSON("a")))
	s.Equal(http.StatusBadRequest, code, string(body))

	// Missing file.
	code, body = s.serve(clearinghouseCSVRequest(s.T(), map[string]string{"name": "a"}, "", ""))
	s.Equal(http.StatusBadRequest, code, string(body))
	s.Contains(string(body), "Exactly one CSV file is required")

	// Missing and blank name.
	code, body = s.serve(clearinghouseCSVRequest(s.T(), nil, "a.csv", validCSV))
	s.Equal(http.StatusBadRequest, code, string(body))
	s.Contains(string(body), "A name is required")
	code, body = s.serve(clearinghouseCSVRequest(s.T(), map[string]string{"name": "  "}, "a.csv", validCSV))
	s.Equal(http.StatusBadRequest, code, string(body))

	// Name too long.
	code, body = s.serve(clearinghouseCSVRequest(s.T(), map[string]string{"name": strings.Repeat("a", 256)}, "a.csv", validCSV))
	s.Equal(http.StatusBadRequest, code, string(body))
	s.Contains(string(body), "Name is too long")

	// Invalid CSV content is reported with its row.
	code, body = s.serve(clearinghouseCSVRequest(s.T(), map[string]string{"name": "a"}, "a.csv", validCSV+"CVE-2,p,t,\n"))
	s.Equal(http.StatusBadRequest, code, string(body))
	s.Contains(string(body), `row 3: missing required field \"description\"`)

	// Too large.
	large := validCSV + strings.Repeat(" ", maxClearinghouseFileBytes)
	code, body = s.serve(clearinghouseCSVRequest(s.T(), map[string]string{"name": "a"}, "a.csv", large))
	s.Equal(http.StatusRequestEntityTooLarge, code, string(body))
}

func (s *ClearinghouseSuite) TestCreateCSVPutFailure() {
	s.enableClearinghouse()
	s.expectCreate()
	s.s3Client.On("Put", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(errors.New("put failed")).Once()

	code, body := s.serve(clearinghouseCSVRequest(s.T(), map[string]string{"name": "a"}, "a.csv", validCSV))
	s.Equal(http.StatusInternalServerError, code, string(body))
	s.Contains(string(body), "put failed")
}

// clearinghouseCSVRequest builds a multipart CSV create request with the given
// form fields and, when filename is set, a "file" part holding content.
func clearinghouseCSVRequest(t *testing.T, fields map[string]string, filename, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}
	if filename != "" {
		part, err := writer.CreateFormFile("file", filename)
		require.NoError(t, err)
		_, err = part.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, LightwellAPIPath+csvCreatePath, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	return req
}

func strPtr(s string) *string {
	return &s
}
