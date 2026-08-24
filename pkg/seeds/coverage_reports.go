package seeds

import (
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"gorm.io/gorm"
)

type CoverageReportSeedOptions struct {
	UUID string
}

func SeedCoverageReport(db *gorm.DB, options CoverageReportSeedOptions) (*models.CoverageReport, error) {
	if options.UUID == "" {
		return nil, fmt.Errorf("report UUID is required")
	}

	var report models.CoverageReport
	result := db.Where("uuid = ?", options.UUID).First(&report)
	if result.Error != nil {
		return nil, fmt.Errorf("could not seed coverage report: %w", result.Error)
	}

	now := time.Now().UTC()
	inputFormat := "csv"
	total, exact, partial, unmatched := 9, 5, 2, 2
	summary := models.EcosystemCoverageSummary{
		{Ecosystem: "Java", Total: 5, ExactMatches: 3, PartialMatches: 1, Unmatched: 1},
		{Ecosystem: "Python", Total: 4, ExactMatches: 2, PartialMatches: 1, Unmatched: 1},
	}

	report.Status = config.TaskStatusCompleted
	report.InputFormat = &inputFormat
	report.Total = &total
	report.ExactMatches = &exact
	report.PartialMatches = &partial
	report.Unmatched = &unmatched
	report.EcosystemCoverageSummary = &summary
	report.CatalogSnapshotAt = &now
	report.CompletedAt = &now
	if err := db.Save(&report).Error; err != nil {
		return nil, fmt.Errorf("could not seed coverage report: %w", err)
	}

	packages := []models.CoverageReportPackage{
		{CoverageReportUUID: options.UUID, Ecosystem: "Java", Name: "spring-web", Version: "6.1.5", Namespace: utils.Ptr("org.springframework"), MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: options.UUID, Ecosystem: "Java", Name: "spring-core", Version: "6.1.5", Namespace: utils.Ptr("org.springframework"), MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: options.UUID, Ecosystem: "Java", Name: "spring-boot-starter-web", Version: "3.2.4", Namespace: utils.Ptr("org.springframework.boot"), MatchStatus: models.CoverageMatchStatusPartial},
		{CoverageReportUUID: options.UUID, Ecosystem: "Java", Name: "jackson-databind", Version: "2.17.0", Namespace: utils.Ptr("com.fasterxml.jackson.core"), MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: options.UUID, Ecosystem: "Java", Name: "netty-codec-http", Version: "4.1.108.Final", Namespace: utils.Ptr("io.netty"), MatchStatus: models.CoverageMatchStatusNone},
		{CoverageReportUUID: options.UUID, Ecosystem: "Python", Name: "requests", Version: "2.31.0", MatchStatus: models.CoverageMatchStatusExact},
		{CoverageReportUUID: options.UUID, Ecosystem: "Python", Name: "urllib3", Version: "2.0.7", MatchStatus: models.CoverageMatchStatusNone},
		{CoverageReportUUID: options.UUID, Ecosystem: "Python", Name: "idna", Version: "3.7", MatchStatus: models.CoverageMatchStatusPartial},
		{CoverageReportUUID: options.UUID, Ecosystem: "Python", Name: "django", Version: "4.2.11", MatchStatus: models.CoverageMatchStatusExact},
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

	return &report, nil
}
