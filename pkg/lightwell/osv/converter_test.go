package osv

import (
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/stretchr/testify/assert"
)

func advisory(id, pkg string, fixed []string, updated time.Time) models.LightwellAdvisory {
	a := models.LightwellAdvisory{
		AdvisoryID:    id,
		PackageName:   pkg,
		FixedVersions: fixed,
	}
	a.CreatedAt = updated
	a.UpdatedAt = updated
	return a
}

func TestBuildRecordsGroupsByAdvisoryID(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	advisories := []models.LightwellAdvisory{
		advisory("LW-1", "left-pad", []string{"1.3.0"}, t1),
		advisory("LW-1", "right-pad", []string{"2.0.0"}, t2),
		advisory("LW-2", "log4j", []string{"2.17.0"}, t1),
	}

	recs := BuildRecords(advisories)
	assert.Len(t, recs, 2)

	assert.Equal(t, "LW-1", recs[0].ID)
	assert.Len(t, recs[0].Affected, 2)
	// modified is the max UpdatedAt across the group.
	assert.Equal(t, t2.Format(time.RFC3339), recs[0].Modified)
	assert.Equal(t, t1.Format(time.RFC3339), recs[0].Published)

	assert.Equal(t, "LW-2", recs[1].ID)
	assert.Len(t, recs[1].Affected, 1)
}

func TestBuildRecordsMapsAffectedAndReferences(t *testing.T) {
	a := advisory("LW-1", "left-pad", []string{"1.3.0", "1.2.5"}, time.Now())
	a.ReferenceURLs = []string{"https://example.com/a", "https://example.com/a", "https://example.com/b"}
	a.Details = "A bad bug"

	recs := BuildRecords([]models.LightwellAdvisory{a})
	assert.Len(t, recs, 1)
	rec := recs[0]

	assert.Equal(t, "A bad bug", rec.Details)
	assert.Equal(t, DefaultEcosystem, rec.Affected[0].Package.Ecosystem)
	assert.Equal(t, "left-pad", rec.Affected[0].Package.Name)

	events := rec.Affected[0].Ranges[0].Events
	assert.Equal(t, "0", events[0].Introduced)
	assert.Equal(t, "1.3.0", events[1].Fixed)
	assert.Equal(t, "1.2.5", events[2].Fixed)

	// References are de-duplicated.
	assert.Len(t, rec.References, 2)
	assert.Equal(t, "ADVISORY", rec.References[0].Type)
}

func TestBuildRecordsSeverityOnlyForVectors(t *testing.T) {
	qualitative := advisory("LW-1", "p", nil, time.Now())
	qualitative.Severity = "Critical"
	vector := advisory("LW-2", "p", nil, time.Now())
	vector.Severity = "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"

	recs := BuildRecords([]models.LightwellAdvisory{qualitative, vector})
	assert.Empty(t, recs[0].Severity)
	assert.Len(t, recs[1].Severity, 1)
	assert.Equal(t, "CVSS_V3", recs[1].Severity[0].Type)
}

func makeRecord(pkg string, fixed []string) api.OsvVulnerability {
	return BuildRecords([]models.LightwellAdvisory{advisory("LW-1", pkg, fixed, time.Now())})[0]
}

func TestMatches(t *testing.T) {
	rec := makeRecord("left-pad", []string{"1.3.0"})

	tests := []struct {
		name  string
		query api.OsvQuery
		want  bool
	}{
		{
			name:  "no version matches package",
			query: api.OsvQuery{Package: api.OsvPackage{Name: "left-pad"}},
			want:  true,
		},
		{
			name:  "version below fix is affected",
			query: api.OsvQuery{Version: "1.2.0", Package: api.OsvPackage{Name: "left-pad"}},
			want:  true,
		},
		{
			name:  "version at fix is not affected",
			query: api.OsvQuery{Version: "1.3.0", Package: api.OsvPackage{Name: "left-pad"}},
			want:  false,
		},
		{
			name:  "version above fix is not affected",
			query: api.OsvQuery{Version: "1.4.0", Package: api.OsvPackage{Name: "left-pad"}},
			want:  false,
		},
		{
			name:  "ecosystem mismatch",
			query: api.OsvQuery{Version: "1.2.0", Package: api.OsvPackage{Name: "left-pad", Ecosystem: "npm"}},
			want:  false,
		},
		{
			name:  "matching ecosystem",
			query: api.OsvQuery{Version: "1.2.0", Package: api.OsvPackage{Name: "left-pad", Ecosystem: DefaultEcosystem}},
			want:  true,
		},
		{
			name:  "different package",
			query: api.OsvQuery{Version: "1.2.0", Package: api.OsvPackage{Name: "right-pad"}},
			want:  false,
		},
		{
			name:  "commit queries never match",
			query: api.OsvQuery{Commit: "deadbeef"},
			want:  false,
		},
		{
			name:  "purl fallback for name",
			query: api.OsvQuery{Version: "1.2.0", Package: api.OsvPackage{Purl: "pkg:npm/left-pad@1.2.0"}},
			want:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Matches(rec, tc.query))
		})
	}
}

func TestMatchesNoFixedVersionAlwaysAffected(t *testing.T) {
	rec := makeRecord("left-pad", nil)
	assert.True(t, Matches(rec, api.OsvQuery{Version: "9.9.9", Package: api.OsvPackage{Name: "left-pad"}}))
}

func TestCompareVersions(t *testing.T) {
	assert.Equal(t, -1, compareVersions("1.2.0", "1.3.0"))
	assert.Equal(t, 1, compareVersions("1.10.0", "1.9.0"))
	assert.Equal(t, 0, compareVersions("1.2.0", "1.2.0"))
	assert.Equal(t, -1, compareVersions("1.2", "1.2.1"))
}
