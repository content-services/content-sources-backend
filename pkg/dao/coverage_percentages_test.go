package dao

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCoveragePercentValuesZeroTotal(t *testing.T) {
	got := coveragePercentValues(coveragePercentCounts{exact: 1}, 0)
	assert.Equal(t, 0.0, got.Exact)
	assert.Equal(t, 0.0, got.NoCoverage)
}

func TestCoveragePercentValuesAllUnmatched(t *testing.T) {
	got := coveragePercentValues(coveragePercentCounts{noCoverage: 1}, 1)
	assert.Equal(t, 0.0, got.Exact)
	assert.Equal(t, 100.0, got.NoCoverage)
}

func TestCoveragePercentValuesSumsToOneHundred(t *testing.T) {
	got := coveragePercentValues(coveragePercentCounts{
		exact: 2, major: 1, minor: 1, patch: 1, none: 1, noCoverage: 1,
	}, 7)
	assert.InDelta(t, 28.5, got.Exact, 0.001)
	assert.InDelta(t, 14.3, got.Major, 0.001)
	assert.InDelta(t, 14.3, got.Minor, 0.001)
	assert.InDelta(t, 14.3, got.Patch, 0.001)
	assert.InDelta(t, 14.3, got.None, 0.001)
	assert.InDelta(t, 14.3, got.NoCoverage, 0.001)
	assert.InDelta(t, 100.0, got.Exact+got.Major+got.Minor+got.Patch+got.None+got.NoCoverage, 0.001)
}

func TestCoveragePercentValuesTieBreaksTowardEarlierBucket(t *testing.T) {
	got := coveragePercentValues(coveragePercentCounts{exact: 1, major: 1, minor: 1}, 3)
	assert.InDelta(t, 33.4, got.Exact, 0.001)
	assert.InDelta(t, 33.3, got.Major, 0.001)
	assert.InDelta(t, 33.3, got.Minor, 0.001)
	assert.Equal(t, 0.0, got.NoCoverage)
}

func TestCoveragePercentCountsExactIgnoresSegment(t *testing.T) {
	var counts coveragePercentCounts
	counts.add("exact", "none", 2)
	counts.add("partial", "major", 1)
	counts.add("partial", "none", 1)
	counts.add("partial", "", 1)
	counts.add("none", "none", 1)
	assert.Equal(t, coveragePercentCounts{exact: 2, major: 1, none: 2, noCoverage: 1}, counts)
}
