package rhlw

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		ok      bool
		want    Rank
		comment string
	}{
		{"1.2.3", false, Rank{}, "maven upstream"},
		{"1.2.4", false, Rank{}, "newer upstream"},
		{"1.2.3.rhlw.00003", true, Rank{Baseline: 3}, "maven baseline"},
		{"1.2.3.rhlw.00003.n00001", true, Rank{Baseline: 3, Novel: 1}, "maven novel"},
		{"1.2.3.rhlw.00003.n00001.hf00001", true, Rank{Baseline: 3, Novel: 1, Hotfix: 1}, "maven hotfix"},
		{"1.2.3.rhlw.00004", true, Rank{Baseline: 4}, "maven next baseline"},
		{"1.2.3+rhlw.1", true, Rank{Baseline: 1}, "python baseline"},
		{"1.2.3+rhlw.1.n1", true, Rank{Baseline: 1, Novel: 1}, "python novel"},
		{"1.2.3+rhlw.2", true, Rank{Baseline: 2}, "python next baseline"},
		{"1.2.3.rhlw-00003", true, Rank{Baseline: 3}, "hyphen maven"},
		{"1.2.3-rhlw-00001", true, Rank{Baseline: 1}, "hyphen python-style"},
		{"2.11.0.rhlw-00000-n-00008", true, Rank{Novel: 8}, "hyphen novel"},
		{"x_RHLW-CVE-2015-6748-1.7.2", false, Rank{}, "advisory id is not a rebuild"},
		{"x_RHLW-LW-2026-4255-2.11.0", false, Rank{}, "LW advisory id without rebuild"},
		{"x_RHLW-LW-2026-4255-2.11.0.rhlw-00000-n-00008", true, Rank{Novel: 8}, "rebuild token in advisory id"},
		{"1.7.2.RHLW.00003.N00001.HF00001", true, Rank{Baseline: 3, Novel: 1, Hotfix: 1}, "case insensitive"},
	}

	for _, tc := range cases {
		t.Run(tc.comment, func(t *testing.T) {
			t.Parallel()
			got, ok := Parse(tc.in)
			assert.Equal(t, tc.ok, ok, tc.in)
			assert.Equal(t, tc.want, got, tc.in)
		})
	}
}

func TestReleasesDedupesRankAndUsesAdvisoryIDFallback(t *testing.T) {
	t.Parallel()

	got := Releases(
		[]string{"1.2.3.rhlw.00003", "1.2.3.rhlw-00003", "1.2.3"},
		"x_RHLW-CVE-2015-6748-1.7.2",
	)
	require.Len(t, got, 1)
	assert.Equal(t, "1.2.3.rhlw.00003", got[0].Version)
	assert.Equal(t, Rank{Baseline: 3}, got[0].Rank)

	got = Releases([]string{"1.2.3"}, "x_RHLW-LW-2026-4255-2.11.0.rhlw-00000-n-00008")
	require.Len(t, got, 1)
	assert.Equal(t, Rank{Novel: 8}, got[0].Rank)
	assert.Equal(t, "x_RHLW-LW-2026-4255-2.11.0.rhlw-00000-n-00008", got[0].Version)

	got = Releases(nil, "FAKE-001")
	assert.Empty(t, got)
}
