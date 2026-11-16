package lightwell

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/labstack/echo/v4"
)

// utf8BOM is the byte order mark spreadsheet tools such as Excel write at the
// start of UTF-8 CSV exports.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// csvVulnerability is one CSV row converted to an intake submission vulnerability.
// It carries the same JSON field names as api.ReportedVulnerability, but omits
// every optional field whose cell was empty so an empty cell reads as unset
// rather than as a zero value (for example "cvss_score": 0).
type csvVulnerability struct {
	VulnerabilityID    string   `json:"vulnerability_id"`
	Packageurl         string   `json:"packageurl"`
	ComponentName      string   `json:"component_name,omitempty"`
	ComponentVersion   string   `json:"component_version,omitempty"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	CvssSeverity       string   `json:"cvss_severity,omitempty"`
	CvssScore          *float64 `json:"cvss_score,omitempty"`
	Cwe                *string  `json:"cwe,omitempty"`
	CvssVector         *string  `json:"cvss_vector,omitempty"`
	ExploitTested      *bool    `json:"exploit_tested,omitempty"`
	ReproducerFilename *string  `json:"reproducer_filename,omitempty"`
	CustomerPriority   *string  `json:"customer_priority,omitempty"`
}

// csvIntakeSubmission is the intake submission JSON built from a CSV upload.
type csvIntakeSubmission struct {
	Name            string             `json:"name"`
	Vulnerabilities []csvVulnerability `json:"vulnerabilities"`
}

// csvRequiredColumns are the columns a CSV submission must have, in the order
// they are reported when missing.
var csvRequiredColumns = []string{"vulnerability_id", "packageurl", "title", "description"}

// csvColumnSetters parse a trimmed, non-empty cell into the matching field.
var csvColumnSetters = map[string]func(v *csvVulnerability, value string) error{
	"vulnerability_id":  func(v *csvVulnerability, value string) error { v.VulnerabilityID = value; return nil },
	"packageurl":        func(v *csvVulnerability, value string) error { v.Packageurl = value; return nil },
	"component_name":    func(v *csvVulnerability, value string) error { v.ComponentName = value; return nil },
	"component_version": func(v *csvVulnerability, value string) error { v.ComponentVersion = value; return nil },
	"title":             func(v *csvVulnerability, value string) error { v.Title = value; return nil },
	"description":       func(v *csvVulnerability, value string) error { v.Description = value; return nil },
	"cvss_severity":     func(v *csvVulnerability, value string) error { v.CvssSeverity = value; return nil },
	"cvss_score": func(v *csvVulnerability, value string) error {
		score, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(score) || math.IsInf(score, 0) {
			return errors.New("must be a number")
		}
		v.CvssScore = &score
		return nil
	},
	"cwe":         func(v *csvVulnerability, value string) error { v.Cwe = &value; return nil },
	"cvss_vector": func(v *csvVulnerability, value string) error { v.CvssVector = &value; return nil },
	"exploit_tested": func(v *csvVulnerability, value string) error {
		tested, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New("must be true or false")
		}
		v.ExploitTested = &tested
		return nil
	},
	"reproducer_filename": func(v *csvVulnerability, value string) error { v.ReproducerFilename = &value; return nil },
	"customer_priority":   func(v *csvVulnerability, value string) error { v.CustomerPriority = &value; return nil },
}

// createCSVSubmission godoc
// @Summary      Create a clearinghouse submission from a CSV file
// @ID           createClearinghouseSubmissionCSV
// @Description  Create a new clearinghouse submission from a CSV file of up to 50 MiB. The first row is a header naming the columns, using the intake submission field names: vulnerability_id, packageurl, title and description are required; component_name, component_version, cvss_severity, cvss_score, cwe, cvss_vector, exploit_tested, reproducer_filename and customer_priority are optional. Columns may appear in any order, unknown columns are rejected, and each further row is one vulnerability. Empty optional cells are treated as unset. cvss_score must be a number and exploit_tested must be true or false. The CSV must be UTF-8. It is converted to an intake submission JSON, which is stored as submission.json; the CSV itself is not kept. Requires the Lightwell clearinghouse feature.
// @Tags         lightwell
// @Accept       multipart/form-data
// @Produce      json
// @Param        file formData file   true "CSV file with a header row and one vulnerability per row."
// @Param        name formData string true "User-facing label for the submission, up to 255 characters."
// @Success      201 {object} api.ClearinghouseSubmission
// @Failure      400 {object} ce.ErrorResponse
// @Failure      409 {object} ce.ErrorResponse
// @Failure      413 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /clearinghouse/submissions/create/csv [post]
func (h *ClearinghouseHandler) createCSVSubmission(c echo.Context) error {
	_, orgID := handler.GetAccountIdOrgId(c)
	if orgID == "" {
		return ce.NewErrorResponse(http.StatusBadRequest, "Cannot upload a clearinghouse submission", "Org ID is required")
	}
	username, err := clearinghouseUsername(c)
	if err != nil {
		return err
	}
	body, name, vulns, err := readCSVIntakeSubmission(c)
	if err != nil {
		return err
	}
	return h.createIntakeSubmission(c, orgID, username, name, body, vulns)
}

// readCSVIntakeSubmission reads the multipart CSV upload and converts it to an
// intake submission. It returns the intake submission JSON to store, the
// submission name, and the vulnerabilities it contains.
func readCSVIntakeSubmission(c echo.Context) ([]byte, string, []intakeVulnerability, error) {
	form, err := c.MultipartForm()
	if err != nil {
		return nil, "", nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading submission", "A multipart form with a file and a name is required")
	}
	files := form.File["file"]
	if len(files) != 1 {
		return nil, "", nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading submission", "Exactly one CSV file is required")
	}
	name, err := validateIntakeName(strings.Join(form.Value["name"], ""))
	if err != nil {
		return nil, "", nil, err
	}
	if files[0].Size > maxClearinghouseFileBytes {
		return nil, "", nil, ce.NewErrorResponse(http.StatusRequestEntityTooLarge, "Error reading submission", "Submission exceeds maximum size")
	}
	file, err := files[0].Open()
	if err != nil {
		return nil, "", nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading submission", err.Error())
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxClearinghouseFileBytes+1))
	if err != nil {
		return nil, "", nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading submission", err.Error())
	}
	if int64(len(data)) > maxClearinghouseFileBytes {
		return nil, "", nil, ce.NewErrorResponse(http.StatusRequestEntityTooLarge, "Error reading submission", "Submission exceeds maximum size")
	}

	rows, err := parseClearinghouseCSV(data)
	if err != nil {
		return nil, "", nil, ce.NewErrorResponse(http.StatusBadRequest, "Error reading submission", err.Error())
	}
	body, err := json.Marshal(csvIntakeSubmission{Name: name, Vulnerabilities: rows})
	if err != nil {
		return nil, "", nil, ce.NewErrorResponse(http.StatusInternalServerError, "Error reading submission", err.Error())
	}
	vulns := make([]intakeVulnerability, len(rows))
	for i, row := range rows {
		vulns[i] = intakeVulnerability{
			VulnerabilityID: row.VulnerabilityID,
			Packageurl:      row.Packageurl,
			Title:           row.Title,
			Description:     row.Description,
		}
	}
	return body, name, vulns, nil
}

// parseClearinghouseCSV parses a CSV submission into vulnerabilities. Rows are
// numbered as a spreadsheet shows them: the header is row 1. Rows whose cells
// are all empty are skipped.
func parseClearinghouseCSV(data []byte) ([]csvVulnerability, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("CSV must be UTF-8 encoded")
	}
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, utf8BOM)))
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, errors.New("CSV is empty")
	}
	if err != nil {
		return nil, fmt.Errorf("invalid CSV: %w", err)
	}
	setters, err := csvHeaderSetters(header)
	if err != nil {
		return nil, err
	}

	var vulns []csvVulnerability
	for row := 2; ; row++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid CSV: %w", err)
		}
		vuln, empty, err := csvRecordVulnerability(record, header, setters)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", row, err)
		}
		if !empty {
			vulns = append(vulns, vuln)
		}
	}
	if len(vulns) == 0 {
		return nil, errors.New("at least one vulnerability is required")
	}
	return vulns, nil
}

// csvHeaderSetters validates the header row and returns the setter for each column.
func csvHeaderSetters(header []string) ([]func(*csvVulnerability, string) error, error) {
	setters := make([]func(*csvVulnerability, string) error, len(header))
	seen := map[string]bool{}
	for i, column := range header {
		column = strings.TrimSpace(column)
		header[i] = column
		setter, ok := csvColumnSetters[column]
		if !ok {
			return nil, fmt.Errorf("unknown column %q", column)
		}
		if seen[column] {
			return nil, fmt.Errorf("duplicate column %q", column)
		}
		seen[column] = true
		setters[i] = setter
	}
	for _, column := range csvRequiredColumns {
		if !seen[column] {
			return nil, fmt.Errorf("missing required column %q", column)
		}
	}
	return setters, nil
}

// csvRecordVulnerability converts one CSV record into a vulnerability. empty is
// true when every cell in the record is blank.
func csvRecordVulnerability(record, header []string, setters []func(*csvVulnerability, string) error) (vuln csvVulnerability, empty bool, err error) {
	empty = true
	for i, cell := range record {
		value := strings.TrimSpace(cell)
		if value == "" {
			continue
		}
		empty = false
		if err := setters[i](&vuln, value); err != nil {
			return csvVulnerability{}, false, fmt.Errorf("%s %w", header[i], err)
		}
	}
	if empty {
		return csvVulnerability{}, true, nil
	}
	missing := intakeVulnerability{
		VulnerabilityID: vuln.VulnerabilityID,
		Packageurl:      vuln.Packageurl,
		Title:           vuln.Title,
		Description:     vuln.Description,
	}.missingRequiredField()
	if missing != "" {
		return csvVulnerability{}, false, fmt.Errorf("missing required field %q", missing)
	}
	return vuln, false, nil
}
