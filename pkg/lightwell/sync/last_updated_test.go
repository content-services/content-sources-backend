package sync

import (
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/stretchr/testify/assert"
)

func TestApplyLastUpdatedFreezesShippedAndUnremediated(t *testing.T) {
	jiraUpdated := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	published := time.Date(2026, 3, 2, 15, 0, 0, 0, time.UTC)
	frozen := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	t.Run("network uses the package publish time", func(t *testing.T) {
		v := Vulnerability{Stage: stageLightwellNetwork, LastUpdated: jiraUpdated}
		applyLastUpdated(&v, storedVulnerability{}, published, true)
		assert.Equal(t, published, v.LastUpdated)
	})

	t.Run("network without a publish time keeps the stored timestamp", func(t *testing.T) {
		v := Vulnerability{Stage: stageLightwellNetwork, LastUpdated: jiraUpdated}
		applyLastUpdated(&v, storedVulnerability{Found: true, Stage: stageLightwellNetwork, LastUpdated: frozen}, time.Time{}, false)
		assert.Equal(t, frozen, v.LastUpdated)
	})

	t.Run("network without a publish time or a stored row keeps jira updated", func(t *testing.T) {
		v := Vulnerability{Stage: stageLightwellNetwork, LastUpdated: jiraUpdated}
		applyLastUpdated(&v, storedVulnerability{}, time.Time{}, false)
		assert.Equal(t, jiraUpdated, v.LastUpdated)
	})

	t.Run("unremediated snapshots jira updated on the transition", func(t *testing.T) {
		v := Vulnerability{Stage: stageUnremediated, LastUpdated: jiraUpdated}
		applyLastUpdated(&v, storedVulnerability{Found: true, Stage: "Validation", LastUpdated: frozen}, time.Time{}, false)
		assert.Equal(t, jiraUpdated, v.LastUpdated)
	})

	t.Run("unremediated keeps the stored timestamp while the stage stays", func(t *testing.T) {
		v := Vulnerability{Stage: stageUnremediated, LastUpdated: jiraUpdated}
		applyLastUpdated(&v, storedVulnerability{Found: true, Stage: stageUnremediated, LastUpdated: frozen}, time.Time{}, false)
		assert.Equal(t, frozen, v.LastUpdated)
	})

	t.Run("open stages follow jira updated", func(t *testing.T) {
		v := Vulnerability{Stage: "Validation", LastUpdated: jiraUpdated}
		applyLastUpdated(&v, storedVulnerability{Found: true, Stage: stageLightwellNetwork, LastUpdated: frozen}, published, true)
		assert.Equal(t, jiraUpdated, v.LastUpdated)
	})
}

func TestLatestPackagePublishedAt(t *testing.T) {
	rows := []dao.PackagePublishTime{
		{Group: "com.example", Name: "demo-lib", Version: "1.2.3.build-00001", PublishedAt: "2026-01-15T12:00:00Z"},
		{Group: "com.example", Name: "demo-lib", Version: "1.2.3.build-00002", PublishedAt: "2026-04-01T08:30:00Z"},
		{Group: "com.example", Name: "other-lib", Version: "1.2.3.build-00002", PublishedAt: "2026-09-01T00:00:00Z"},
		{Group: "com.example", Name: "demo-lib", Version: "1.2.3.build-00003", PublishedAt: "not-a-time"},
	}

	got, ok := latestPackagePublishedAt("com.example:demo-lib", rows)
	assert.True(t, ok)
	assert.Equal(t, time.Date(2026, 4, 1, 8, 30, 0, 0, time.UTC), got)

	_, ok = latestPackagePublishedAt("missing-lib", rows)
	assert.False(t, ok)

	_, ok = latestPackagePublishedAt("demo-lib", []dao.PackagePublishTime{
		{Name: "demo-lib", PublishedAt: ""},
	})
	assert.False(t, ok)
}
