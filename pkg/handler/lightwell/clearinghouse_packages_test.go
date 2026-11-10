package lightwell

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/stretchr/testify/assert"
)

func TestClearinghousePackagesDeduplicatesLikeLens(t *testing.T) {
	vulns := []intakeVulnerability{
		{Packageurl: "pkg:maven/org.apache.logging.log4j/log4j-core@2.14.1"},
		{Packageurl: "pkg:maven/org.apache.logging.log4j/log4j-core@2.14.1"},
		{Packageurl: "pkg:pypi/flask@2.3.0"},
		{Packageurl: "pkg:maven/org.apache.logging.log4j/log4j-core@2.14.1"},
		{Packageurl: "pkg:maven/org.apache.logging.log4j/log4j-core@2.15.0"},
	}

	packages, summary := clearinghousePackages(vulns)

	assert.Equal(t, []dao.CreateClearinghouseUploadPackageParams{
		{Packageurl: "pkg:maven/org.apache.logging.log4j/log4j-core@2.14.1", Ecosystem: "Java", Namespace: "org.apache.logging.log4j", Name: "log4j-core", Version: "2.14.1", ValidPurl: true},
		{Packageurl: "pkg:pypi/flask@2.3.0", Ecosystem: "Python", Name: "flask", Version: "2.3.0", ValidPurl: true},
		{Packageurl: "pkg:maven/org.apache.logging.log4j/log4j-core@2.15.0", Ecosystem: "Java", Namespace: "org.apache.logging.log4j", Name: "log4j-core", Version: "2.15.0", ValidPurl: true},
	}, packages)
	assert.Equal(t, api.ClearinghousePackageSummary{
		Total: 3,
		Ecosystems: []api.ClearinghousePackageEcosystemSummary{
			{Ecosystem: "Java", Total: 2},
			{Ecosystem: "Python", Total: 1},
		},
	}, summary)
}

func TestClearinghousePackagesKeepsInvalidPurlsAsUnknown(t *testing.T) {
	vulns := []intakeVulnerability{
		{Packageurl: "log4j-core 2.14"},
		{Packageurl: " log4j-core 2.14 "},
		{Packageurl: "pkg:"},
		{Packageurl: "pkg:npm/react@18.2.0"},
	}

	packages, summary := clearinghousePackages(vulns)

	assert.Equal(t, []dao.CreateClearinghouseUploadPackageParams{
		{Packageurl: "log4j-core 2.14", Ecosystem: api.ClearinghouseEcosystemUnknown,
			InvalidReason: api.ClearinghouseInvalidReasonNotAPurl, InvalidDetail: `purl scheme is not "pkg": "log4j-core 2.14"`},
		{Packageurl: "pkg:", Ecosystem: api.ClearinghouseEcosystemUnknown,
			InvalidReason: api.ClearinghouseInvalidReasonNotAPurl, InvalidDetail: "purl is missing type or name"},
		{Packageurl: "pkg:npm/react@18.2.0", Ecosystem: "JavaScript", Name: "react", Version: "18.2.0", ValidPurl: true},
	}, packages)
	assert.Equal(t, api.ClearinghousePackageSummary{
		Total:        3,
		InvalidPurls: 2,
		Ecosystems: []api.ClearinghousePackageEcosystemSummary{
			{Ecosystem: api.ClearinghouseEcosystemUnknown, Total: 2},
			{Ecosystem: "JavaScript", Total: 1},
		},
	}, summary)
}

func TestClearinghousePackagesMarksPurlsWithoutVersionInvalid(t *testing.T) {
	vulns := []intakeVulnerability{
		{Packageurl: "pkg:maven/org.example/lib"},
		{Packageurl: "pkg:maven/org.example/lib"},
		{Packageurl: "pkg:maven/org.example/lib@1.0"},
	}

	packages, summary := clearinghousePackages(vulns)

	assert.Equal(t, []dao.CreateClearinghouseUploadPackageParams{
		{Packageurl: "pkg:maven/org.example/lib", Ecosystem: "Java", Namespace: "org.example", Name: "lib",
			InvalidReason: api.ClearinghouseInvalidReasonMissingVersion, InvalidDetail: "packageurl has no version"},
		{Packageurl: "pkg:maven/org.example/lib@1.0", Ecosystem: "Java", Namespace: "org.example", Name: "lib", Version: "1.0", ValidPurl: true},
	}, packages)
	assert.Equal(t, api.ClearinghousePackageSummary{
		Total:        2,
		InvalidPurls: 1,
		Ecosystems:   []api.ClearinghousePackageEcosystemSummary{{Ecosystem: "Java", Total: 2}},
	}, summary)
}
