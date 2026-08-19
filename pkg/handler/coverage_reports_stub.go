package handler

import (
	"embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/google/uuid"
)

//go:embed test_files/coverage_reports/*.json
var coverageReportStubFS embed.FS

func stubCreateCoverageReport() (api.CoverageReportResponse, error) {
	report, err := stubPendingReport()
	if err != nil {
		return api.CoverageReportResponse{}, err
	}

	report.UUID = uuid.New().String()
	report.CreatedAt = time.Now().UTC().Truncate(time.Second)
	return report, nil
}

func stubPendingReport() (api.CoverageReportResponse, error) {
	return loadCoverageReportStub("test_files/coverage_reports/report_pending.json")
}

func loadCoverageReportStub(path string) (api.CoverageReportResponse, error) {
	raw, err := coverageReportStubFS.ReadFile(path)
	if err != nil {
		return api.CoverageReportResponse{}, fmt.Errorf("failed to read coverage report stub %s: %w", path, err)
	}

	var report api.CoverageReportResponse
	if err := json.Unmarshal(raw, &report); err != nil {
		return api.CoverageReportResponse{}, fmt.Errorf("failed to unmarshal coverage report stub %s: %w", path, err)
	}

	return report, nil
}
