package seeds

import (
	"errors"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"gorm.io/gorm"
)

const (
	stubCompletedReportUUID    = "550e8400-e29b-41d4-a716-446655440000"
	stubHighCoverageReportUUID = "880e8400-e29b-41d4-a716-446655440003"
)

type CoverageReportSeedOptions struct {
	OrgID string
}

func SeedCoverageReport(db *gorm.DB, options CoverageReportSeedOptions) (*models.CoverageReport, error) {
	if options.OrgID == "" {
		return nil, fmt.Errorf("org ID is required")
	}

	report := models.CoverageReport{}
	result := db.Where("uuid = ?", stubCompletedReportUUID).First(&report)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("could not query coverage report: %w", result.Error)
	}
	if result.Error == nil {
		if err := seedHighCoverageReport(db, options.OrgID); err != nil {
			return nil, err
		}
		return &report, nil
	}

	createdAt := time.Date(2026, 8, 13, 14, 30, 0, 0, time.UTC)
	completedAt := time.Date(2026, 8, 13, 14, 30, 42, 0, time.UTC)
	inputFormat := "cyclonedx"
	total, exact, partial, unmatched := 15, 8, 3, 4
	summary := models.EcosystemCoverageSummary{
		{Ecosystem: "Java", Total: 9, ExactMatches: 4, PartialMatches: 2, Unmatched: 3},
		{Ecosystem: "Python", Total: 6, ExactMatches: 4, PartialMatches: 1, Unmatched: 1},
	}

	report = models.CoverageReport{
		UUID:                     stubCompletedReportUUID,
		CreatedAt:                createdAt,
		OrgID:                    options.OrgID,
		Status:                   config.TaskStatusCompleted,
		InputFormat:              &inputFormat,
		Total:                    &total,
		ExactMatches:             &exact,
		PartialMatches:           &partial,
		Unmatched:                &unmatched,
		EcosystemCoverageSummary: &summary,
		CatalogSnapshotAt:        &completedAt,
		CompletedAt:              &completedAt,
	}
	if err := db.Create(&report).Error; err != nil {
		return nil, fmt.Errorf("could not seed coverage report: %w", err)
	}

	packages := []models.CoverageReportPackage{
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Java", Name: "spring-core", Version: "6.1.0", Namespace: utils.Ptr("org.springframework"), MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Java", Name: "spring-boot-starter-web", Version: "3.2.0", Namespace: utils.Ptr("org.springframework.boot"), MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Java", Name: "jackson-databind", Version: "2.15.0", Namespace: utils.Ptr("com.fasterxml.jackson.core"), MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Java", Name: "guava", Version: "32.1.0", Namespace: utils.Ptr("com.google.guava"), MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Java", Name: "legacy-commons", Version: "1.0.0", Namespace: utils.Ptr("com.example"), MatchStatus: models.CoverageMatchStatusNone},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Java", Name: "internal-util", Version: "2.3.1", Namespace: utils.Ptr("com.internal"), MatchStatus: models.CoverageMatchStatusNone},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Java", Name: "slf4j-api", Version: "2.0.9", Namespace: utils.Ptr("org.slf4j"), MatchStatus: models.CoverageMatchStatusPartial},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Java", Name: "hibernate-core", Version: "6.2.0", Namespace: utils.Ptr("org.hibernate.orm"), MatchStatus: models.CoverageMatchStatusPartial},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Java", Name: "proprietary-lib", Version: "0.9.0", Namespace: utils.Ptr("com.vendor"), MatchStatus: models.CoverageMatchStatusNone},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Python", Name: "numpy", Version: "1.26.0", MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Python", Name: "pandas", Version: "2.1.0", MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Python", Name: "requests", Version: "2.31.0", MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Python", Name: "custom-ml-lib", Version: "0.1.0", MatchStatus: models.CoverageMatchStatusNone},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Python", Name: "flask", Version: "3.0.0", MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubCompletedReportUUID, Ecosystem: "Python", Name: "json5", Version: "0.9.14", MatchStatus: models.CoverageMatchStatusPartial},
	}
	if err := db.Create(&packages).Error; err != nil {
		return nil, fmt.Errorf("could not seed coverage report packages: %w", err)
	}

	demandSignals := make([]models.CoverageDemandSignal, 0, 7)
	for _, pkg := range packages {
		if pkg.MatchStatus != models.CoverageMatchStatusExact {
			demandSignals = append(demandSignals, models.CoverageDemandSignal{
				Ecosystem:   pkg.Ecosystem,
				Name:        pkg.Name,
				Version:     pkg.Version,
				Namespace:   pkg.Namespace,
				MatchStatus: pkg.MatchStatus,
				Source:      models.CoverageDemandSourceProspectDriven,
			})
		}
	}
	if err := db.Create(&demandSignals).Error; err != nil {
		return nil, fmt.Errorf("could not seed coverage demand signals: %w", err)
	}

	if err := seedHighCoverageReport(db, options.OrgID); err != nil {
		return nil, err
	}

	return &report, nil
}

func seedHighCoverageReport(db *gorm.DB, orgID string) error {
	var existing models.CoverageReport
	result := db.Where("uuid = ?", stubHighCoverageReportUUID).First(&existing)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("could not query high coverage report: %w", result.Error)
	}
	if result.Error == nil {
		return nil
	}

	createdAt := time.Date(2026, 8, 14, 10, 15, 0, 0, time.UTC)
	completedAt := time.Date(2026, 8, 14, 10, 15, 28, 0, time.UTC)
	inputFormat := "cyclonedx"
	total, exact, partial, unmatched := 10, 8, 1, 1
	summary := models.EcosystemCoverageSummary{
		{Ecosystem: "Java", Total: 6, ExactMatches: 4, PartialMatches: 1, Unmatched: 1},
		{Ecosystem: "Python", Total: 4, ExactMatches: 4, PartialMatches: 0, Unmatched: 0},
	}

	report := models.CoverageReport{
		UUID:                     stubHighCoverageReportUUID,
		CreatedAt:                createdAt,
		OrgID:                    orgID,
		Status:                   config.TaskStatusCompleted,
		InputFormat:              &inputFormat,
		Total:                    &total,
		ExactMatches:             &exact,
		PartialMatches:           &partial,
		Unmatched:                &unmatched,
		EcosystemCoverageSummary: &summary,
		CatalogSnapshotAt:        &completedAt,
		CompletedAt:              &completedAt,
	}
	if err := db.Create(&report).Error; err != nil {
		return fmt.Errorf("could not seed high coverage report: %w", err)
	}

	packages := []models.CoverageReportPackage{
		{CoverageReportUUID: stubHighCoverageReportUUID, Ecosystem: "Java", Name: "spring-core", Version: "6.1.0", Namespace: utils.Ptr("org.springframework"), MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubHighCoverageReportUUID, Ecosystem: "Java", Name: "lombok", Version: "1.18.30", Namespace: utils.Ptr("org.projectlombok"), MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubHighCoverageReportUUID, Ecosystem: "Java", Name: "commons-io", Version: "2.15.0", Namespace: utils.Ptr("commons-io"), MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubHighCoverageReportUUID, Ecosystem: "Java", Name: "junit-jupiter", Version: "5.10.0", Namespace: utils.Ptr("org.junit.jupiter"), MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubHighCoverageReportUUID, Ecosystem: "Java", Name: "log4j-api", Version: "2.20.0", Namespace: utils.Ptr("org.apache.logging.log4j"), MatchStatus: models.CoverageMatchStatusPartial},
		{CoverageReportUUID: stubHighCoverageReportUUID, Ecosystem: "Java", Name: "internal-sdk", Version: "1.0.0", Namespace: utils.Ptr("com.internal"), MatchStatus: models.CoverageMatchStatusNone},
		{CoverageReportUUID: stubHighCoverageReportUUID, Ecosystem: "Python", Name: "django", Version: "4.2.0", MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubHighCoverageReportUUID, Ecosystem: "Python", Name: "celery", Version: "5.3.0", MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubHighCoverageReportUUID, Ecosystem: "Python", Name: "redis", Version: "5.0.0", MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: stubHighCoverageReportUUID, Ecosystem: "Python", Name: "boto3", Version: "1.34.0", MatchStatus: models.CoverageMatchStatusExact},
	}
	if err := db.Create(&packages).Error; err != nil {
		return fmt.Errorf("could not seed high coverage report packages: %w", err)
	}

	demandSignals := make([]models.CoverageDemandSignal, 0, 2)
	for _, pkg := range packages {
		if pkg.MatchStatus != models.CoverageMatchStatusExact {
			demandSignals = append(demandSignals, models.CoverageDemandSignal{
				Ecosystem:   pkg.Ecosystem,
				Name:        pkg.Name,
				Version:     pkg.Version,
				Namespace:   pkg.Namespace,
				MatchStatus: pkg.MatchStatus,
				Source:      models.CoverageDemandSourceProspectDriven,
			})
		}
	}
	if err := db.Create(&demandSignals).Error; err != nil {
		return fmt.Errorf("could not seed high coverage demand signals: %w", err)
	}

	return nil
}
