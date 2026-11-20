package dao

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
)

const coveragePercentScale = 10 // one decimal place: 100.0 is 1000 tenths

type coveragePercentCounts struct {
	exact, major, minor, patch, none, noCoverage int
}

func (c coveragePercentCounts) total() int {
	return c.exact + c.major + c.minor + c.patch + c.none + c.noCoverage
}

func (c *coveragePercentCounts) add(status, segment string, count int) {
	switch status {
	case models.CoverageMatchStatusExact:
		c.exact += count
	case models.CoverageMatchStatusNone:
		c.noCoverage += count
	case models.CoverageMatchStatusPartial:
		switch segment {
		case models.CoverageMatchSegmentMajor:
			c.major += count
		case models.CoverageMatchSegmentMinor:
			c.minor += count
		case models.CoverageMatchSegmentPatch:
			c.patch += count
		default:
			c.none += count
		}
	}
}

// coveragePercentValues converts package counts into percentages of total.
// An empty total is all zeros. When the counts cover total, rounding keeps the six values at one decimal and a sum of 100.
func coveragePercentValues(counts coveragePercentCounts, total int) api.EcosystemCoveragePercentages {
	tenths := coveragePercentTenths(
		[6]int{counts.exact, counts.major, counts.minor, counts.patch, counts.none, counts.noCoverage},
		total,
		counts.total() == total,
	)
	return api.EcosystemCoveragePercentages{
		Exact:      float64(tenths[0]) / coveragePercentScale,
		Major:      float64(tenths[1]) / coveragePercentScale,
		Minor:      float64(tenths[2]) / coveragePercentScale,
		Patch:      float64(tenths[3]) / coveragePercentScale,
		None:       float64(tenths[4]) / coveragePercentScale,
		NoCoverage: float64(tenths[5]) / coveragePercentScale,
	}
}

// coveragePercentTenths returns each bucket in tenths of a percent.
// When distribute is set, leftover tenths go to the largest remainder so the result sums to 100.0.
// Equal remainders break toward the earlier bucket.
func coveragePercentTenths(counts [6]int, total int, distribute bool) [6]int {
	var tenths [6]int
	if total <= 0 {
		return tenths
	}
	const target = 100 * coveragePercentScale
	var remainders [6]int
	sum := 0
	for i, count := range counts {
		if count < 0 {
			count = 0
		}
		scaled := count * target
		tenths[i] = scaled / total
		remainders[i] = scaled % total
		sum += tenths[i]
	}
	if !distribute {
		return tenths
	}
	left := target - sum
	for left > 0 {
		best := 0
		for i := 1; i < len(remainders); i++ {
			if remainders[i] > remainders[best] {
				best = i
			}
		}
		tenths[best]++
		remainders[best] = -1
		left--
	}
	return tenths
}

type coveragePackageCount struct {
	Ecosystem    string
	MatchStatus  string
	MatchSegment string
	Count        int
}

func (d coverageReportDaoImpl) ecosystemCoveragePercentages(ctx context.Context, reportUUID string, summary models.EcosystemCoverageSummary) ([]api.EcosystemCoveragePercentages, error) {
	var rows []coveragePackageCount
	err := d.db.WithContext(ctx).
		Model(&models.CoverageReportPackage{}).
		Select("ecosystem, match_status, match_segment, COUNT(*) AS count").
		Where("coverage_report_uuid = ?", reportUUID).
		Group("ecosystem, match_status, match_segment").
		Scan(&rows).Error
	if err != nil {
		return nil, d.toApiError(err)
	}

	byEcosystem := map[string]coveragePercentCounts{}
	for _, row := range rows {
		counts := byEcosystem[row.Ecosystem]
		counts.add(row.MatchStatus, row.MatchSegment, row.Count)
		byEcosystem[row.Ecosystem] = counts
	}

	percentages := make([]api.EcosystemCoveragePercentages, len(summary))
	for i, entry := range summary {
		percentages[i] = coveragePercentValues(byEcosystem[entry.Ecosystem], entry.Total)
		percentages[i].Ecosystem = entry.Ecosystem
	}
	return percentages, nil
}
