package handler

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/google/uuid"
)

const (
	stubPendingReportUUID      = "660e8400-e29b-41d4-a716-446655440001"
	stubFailedReportUUID       = "770e8400-e29b-41d4-a716-446655440002"
	stubCompletedReportUUID    = "550e8400-e29b-41d4-a716-446655440000"
	stubHighCoverageReportUUID = "880e8400-e29b-41d4-a716-446655440003"
)

var errStubCoverageReportNotFound = errors.New("coverage report not found")

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

func stubGetCoverageReport(reportUUID string) (api.CoverageReportResponse, error) {
	switch reportUUID {
	case stubPendingReportUUID:
		return stubPendingReport()
	case stubFailedReportUUID:
		return stubFailedReport()
	case stubCompletedReportUUID:
		return stubCompletedReport()
	case stubHighCoverageReportUUID:
		return stubHighCoverageReport()
	default:
		return api.CoverageReportResponse{}, errStubCoverageReportNotFound
	}
}

func stubListCoverageReportPackages(reportUUID string, req api.ListCoverageReportPackagesRequest, pageData api.PaginationData) (api.CoverageReportPackagesResponse, error) {
	var allPackages []api.CoverageReportPackageItem
	var err error

	switch reportUUID {
	case stubCompletedReportUUID:
		allPackages, err = stubCoverageReportPackages()
	case stubHighCoverageReportUUID:
		allPackages, err = stubHighCoverageReportPackages()
	default:
		return api.CoverageReportPackagesResponse{}, errStubCoverageReportNotFound
	}
	if err != nil {
		return api.CoverageReportPackagesResponse{}, err
	}

	filtered := filterCoverageReportPackages(allPackages, req)

	response := api.CoverageReportPackagesResponse{
		Total:  len(filtered),
		Limit:  pageData.Limit,
		Offset: pageData.Offset,
	}

	start := pageData.Offset
	if start > len(filtered) {
		start = len(filtered)
	}

	end := start + pageData.Limit
	if end > len(filtered) {
		end = len(filtered)
	}

	response.Results = filtered[start:end]
	if response.Results == nil {
		response.Results = []api.CoverageReportPackageItem{}
	}

	return response, nil
}

func stubCompletedReport() (api.CoverageReportResponse, error) {
	return loadCoverageReportStub("test_files/coverage_reports/report_completed.json")
}

func stubHighCoverageReport() (api.CoverageReportResponse, error) {
	return loadCoverageReportStub("test_files/coverage_reports/report_completed_high.json")
}

func stubPendingReport() (api.CoverageReportResponse, error) {
	return loadCoverageReportStub("test_files/coverage_reports/report_pending.json")
}

func stubFailedReport() (api.CoverageReportResponse, error) {
	return loadCoverageReportStub("test_files/coverage_reports/report_failed.json")
}

func stubCoverageReportPackages() ([]api.CoverageReportPackageItem, error) {
	return loadCoverageReportPackagesStub("test_files/coverage_reports/report_packages.json")
}

func stubHighCoverageReportPackages() ([]api.CoverageReportPackageItem, error) {
	return loadCoverageReportPackagesStub("test_files/coverage_reports/report_packages_high.json")
}

func loadCoverageReportPackagesStub(path string) ([]api.CoverageReportPackageItem, error) {
	raw, err := coverageReportStubFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read coverage report packages stub: %w", err)
	}

	var packages []api.CoverageReportPackageItem
	if err := json.Unmarshal(raw, &packages); err != nil {
		return nil, fmt.Errorf("failed to unmarshal coverage report packages stub: %w", err)
	}

	return packages, nil
}

func filterCoverageReportPackages(packages []api.CoverageReportPackageItem, req api.ListCoverageReportPackagesRequest) []api.CoverageReportPackageItem {
	filtered := make([]api.CoverageReportPackageItem, 0, len(packages))
	for _, pkg := range packages {
		if req.Search != "" && pkg.Name != req.Search {
			continue
		}
		if req.Ecosystem != "" && pkg.Ecosystem != req.Ecosystem {
			continue
		}
		if req.Status != "" && pkg.Status != req.Status {
			continue
		}
		filtered = append(filtered, pkg)
	}

	return filtered
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
